package postgres

import (
	"context"
	"encoding/json"
	"rms/internal/storage"
	"time"
)

// Wall-clock timestamps share the configured scheduler timezone with generation.
// oneActiveServices are services a store has at most one open («Новое» or «Выполняется») task of: an open
// task stays current on later days until it is completed or cancelled, and no new one is created meanwhile.
const oneActiveServices = "('stoppers','price_print')"

// serviceWork is true when a task of the service may be created for the store on the day by schedule or
// by hand: «Стопперы» needs stoppers to check or remove; «Печать ценников» is never created this way, RMS
// keeps it itself (see refreshPriceTags); other services always can.
func serviceWork(service, store, day string) string {
	return "(" + service + " NOT IN " + oneActiveServices + " OR (" + service + "='stoppers' AND " + stopperTaskWork(store, day) + "))"
}

// notPriceTagTask is true for a task (alias t) of another service than «Печать ценников».
const notPriceTagTask = "NOT EXISTS(SELECT 1 FROM task_types pt WHERE pt.id=t.type_id AND pt.service='price_print')"

func taskDay(now time.Time) (string, string, string) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return day.Format("2006-01-02"), day.AddDate(0, 0, 1).Format("2006-01-02"), now.Format("2006-01-02 15:04:05")
}

// TaskCurrent lists open tasks of the store that have started today; open tasks of oneActiveServices
// from earlier days are listed too, so a store finishes a started «Стопперы» or «Печать ценников» task.
// The standing «Печать ценников» task is brought up to date first; it is listed while it has tags to print
// or printed today, with price_tags_pending — the number of tags to print (active when above zero).
func (s *Storage) TaskCurrent(ctx context.Context, store string, now time.Time) ([]json.RawMessage, error) {
	if e := s.RefreshPriceTags(ctx, store, now); e != nil {
		return nil, e
	}
	day, next, clock := taskDay(now)
	rows, e := s.db.QueryContext(ctx, `SELECT to_jsonb(t)||jsonb_build_object('name',tt.name,'short_name',tt.short_name,'service',tt.service,'color',tt.color,'color_dark',tt.color_dark,'icon',replace(encode(tt.icon,'base64'),chr(10),''))||CASE WHEN tt.service='price_print' THEN jsonb_build_object('price_tags_pending',`+priceTagPendingTags+`) ELSE '{}'::jsonb END FROM tasks t JOIN task_types tt ON tt.id=t.type_id WHERE t.store_id=$1::uuid AND (t.start >= $2::timestamp OR tt.service IN `+oneActiveServices+`) AND (tt.service<>'price_print' OR `+priceTagShown("$2")+`) AND t.start < $3::timestamp AND t.start <= $4::timestamp AND t.status IN ('new','in_progress') ORDER BY tt.sort_order,t.start,t.id`, store, day, next, clock)
	if e != nil {
		return nil, personnelError(e)
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var v json.RawMessage
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Storage) TaskTransition(ctx context.Context, id int64, target string, user int64) (json.RawMessage, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var previous string
	if e = tx.QueryRowContext(ctx, "SELECT status FROM tasks WHERE id=$1 FOR UPDATE", id).Scan(&previous); e != nil {
		return nil, personnelError(e)
	}
	if previous != target {
		if (target == "in_progress" && previous != "new") || (target == "completed" && previous != "in_progress") || (target == "cancelled" && previous != "new" && previous != "in_progress") {
			return nil, storage.ErrInvalid
		}
		if _, e = tx.ExecContext(ctx, "UPDATE tasks SET status=$2 WHERE id=$1", id, target); e != nil {
			return nil, personnelError(e)
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO task_status_history(period,task_id,user_id,status) VALUES(clock_timestamp(),$1,$2,$3)", id, user, target); e != nil {
			return nil, personnelError(e)
		}
	}
	var out json.RawMessage
	if e = tx.QueryRowContext(ctx, "SELECT to_jsonb(t) FROM tasks t WHERE id=$1", id).Scan(&out); e != nil {
		return nil, e
	}
	return out, personnelError(tx.Commit())
}

func (s *Storage) TaskGenerate(ctx context.Context, now time.Time) error {
	return s.taskGenerate(ctx, now, 0)
}

func (s *Storage) TaskGenerateType(ctx context.Context, now time.Time, typeID int64) error {
	return s.taskGenerate(ctx, now, typeID)
}

func (s *Storage) taskGenerate(ctx context.Context, now time.Time, typeID int64) error {
	day, next, _ := taskDay(now)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Also serializes multiple RMS instances and manual writes during reconciliation.
	if _, e = tx.ExecContext(ctx, "LOCK TABLE task_types,task_scheduling IN SHARE MODE; LOCK TABLE tasks IN SHARE ROW EXCLUSIVE MODE"); e != nil {
		return e
	}
	if typeID > 0 {
		var id int64
		if e = tx.QueryRowContext(ctx, "SELECT id FROM task_types WHERE id=$1", typeID).Scan(&id); e != nil {
			return personnelError(e)
		}
	}
	if _, e = tx.ExecContext(ctx, `CREATE TEMP TABLE desired_tasks ON COMMIT DROP AS
 SELECT DISTINCT tt.id AS type_id, st.store_id, $1::date + COALESCE(sc.time,'00:00'::time) AS start, tt.custom_fields
 FROM task_scheduling sc JOIN task_types tt ON tt.id=sc.task_type_id CROSS JOIN LATERAL unnest(sc.stores) st(store_id)
 WHERE ($2::bigint=0 OR tt.id=$2) AND sc.is_active AND tt.is_active AND $1::date BETWEEN sc.period_from AND sc.period_to
 AND sc.days[(($1::date-sc.period_from) % (sc.frequency*7))+1]=1
 AND `+serviceWork("tt.service", "st.store_id", "$1::date"), day, typeID); e != nil {
		return e
	}
	// The standing «Печать ценников» task is not scheduled, so reconciliation leaves it alone.
	if _, e = tx.ExecContext(ctx, `DELETE FROM task_status_history h USING tasks t WHERE h.task_id=t.id AND t.start >= $1::timestamp AND t.start < $2::timestamp AND t.status='new' AND NOT t.manual AND `+notPriceTagTask+` AND ($3::bigint=0 OR t.type_id=$3) AND NOT EXISTS(SELECT 1 FROM desired_tasks d WHERE d.type_id=t.type_id AND d.store_id=t.store_id AND d.start=t.start AND d.custom_fields=t.custom_fields)`, day, next, typeID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM tasks t WHERE t.start >= $1::timestamp AND t.start < $2::timestamp AND t.status='new' AND NOT t.manual AND `+notPriceTagTask+` AND ($3::bigint=0 OR t.type_id=$3) AND NOT EXISTS(SELECT 1 FROM desired_tasks d WHERE d.type_id=t.type_id AND d.store_id=t.store_id AND d.start=t.start AND d.custom_fields=t.custom_fields)`, day, next, typeID); e != nil {
		return e
	}
	var admin int64
	if e = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE user_token='admin'").Scan(&admin); e != nil {
		return personnelError(e)
	}
	_, e = tx.ExecContext(ctx, `WITH inserted AS (
 INSERT INTO tasks(type_id,store_id,start,status,custom_fields)
 SELECT d.type_id,d.store_id,d.start,'new',d.custom_fields FROM desired_tasks d
 WHERE NOT EXISTS(SELECT 1 FROM tasks t WHERE t.type_id=d.type_id AND t.store_id=d.store_id AND t.start=d.start AND t.custom_fields=d.custom_fields AND NOT t.manual)
 AND NOT EXISTS(SELECT 1 FROM task_types dt JOIN tasks t ON t.store_id=d.store_id JOIN task_types tt ON tt.id=t.type_id
  WHERE dt.id=d.type_id AND dt.service IN `+oneActiveServices+` AND tt.service=dt.service AND t.start<$2::timestamp AND t.status IN ('new','in_progress'))
 RETURNING id)
 INSERT INTO task_status_history(period,task_id,user_id,status) SELECT clock_timestamp(),id,$1,'new' FROM inserted`, admin, next)
	if e != nil {
		return personnelError(e)
	}
	return tx.Commit()
}

// TaskTypesAvailable lists active task types a store user may create by hand.
// available is false for «Стопперы» when the store has nothing for it today; without a store every type
// is available. A store does not get «Печать ценников»: RMS keeps that task itself.
func (s *Storage) TaskTypesAvailable(ctx context.Context, store string, now time.Time) ([]json.RawMessage, error) {
	day, _, _ := taskDay(now)
	rows, e := s.db.QueryContext(ctx, `SELECT jsonb_build_object('id',id,'name',name,'short_name',short_name,'service',service,'color',color,'color_dark',color_dark,'icon',replace(encode(icon,'base64'),chr(10),''),
 'available',$1='' OR `+serviceWork("service", "NULLIF($1,'')::uuid", "$2::date")+`) FROM task_types WHERE is_active AND ($1='' OR service<>'price_print') ORDER BY sort_order,name,id`, store, day)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var v json.RawMessage
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// TaskCreateManual creates a task of an active type for a store starting now.
// Manual tasks are kept by schedule reconciliation.
func (s *Storage) TaskCreateManual(ctx context.Context, typeID int64, store string, now time.Time, user int64) (json.RawMessage, error) {
	day, next, clock := taskDay(now)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	// Locking the store serializes manual creation, so a store has one open task of each service in
	// oneActiveServices, including an open task left from an earlier day.
	var locked string
	if e = tx.QueryRowContext(ctx, "SELECT id FROM stores WHERE id=$1::uuid FOR UPDATE", store).Scan(&locked); e != nil {
		return nil, personnelError(e)
	}
	var busy bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM task_types nt JOIN tasks t ON t.store_id=$2::uuid JOIN task_types tt ON tt.id=t.type_id
 WHERE nt.id=$1 AND nt.service IN `+oneActiveServices+` AND tt.service=nt.service AND t.start<$3::timestamp AND t.status IN ('new','in_progress'))`, typeID, store, next).Scan(&busy); e != nil {
		return nil, e
	}
	if busy {
		return nil, storage.ErrConflict
	}
	// A «Стопперы» task with nothing to do today is not created, nor «Печать ценников» (RMS keeps it itself).
	var empty bool
	if e = tx.QueryRowContext(ctx, `SELECT NOT `+serviceWork("service", "$2::uuid", "$3::date")+` FROM task_types WHERE id=$1`, typeID, store, day).Scan(&empty); e != nil {
		return nil, personnelError(e)
	}
	if empty {
		return nil, storage.ErrConflict
	}
	var id int64
	if e = tx.QueryRowContext(ctx, `INSERT INTO tasks(type_id,store_id,start,status,custom_fields,manual)
 SELECT id,$2::uuid,$3::timestamp,'new',custom_fields,true FROM task_types WHERE id=$1 AND is_active RETURNING id`, typeID, store, clock).Scan(&id); e != nil {
		return nil, personnelError(e)
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO task_status_history(period,task_id,user_id,status) VALUES(clock_timestamp(),$1,$2,'new')", id, user); e != nil {
		return nil, personnelError(e)
	}
	var out json.RawMessage
	if e = tx.QueryRowContext(ctx, "SELECT to_jsonb(t) FROM tasks t WHERE id=$1", id).Scan(&out); e != nil {
		return nil, e
	}
	return out, personnelError(tx.Commit())
}
