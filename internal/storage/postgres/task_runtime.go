package postgres

import (
	"context"
	"encoding/json"
	"rms/internal/storage"
	"time"
)

// Wall-clock timestamps share the configured scheduler timezone with generation.
func taskDay(now time.Time) (string, string, string) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return day.Format("2006-01-02"), day.AddDate(0, 0, 1).Format("2006-01-02"), now.Format("2006-01-02 15:04:05")
}
func (s *Storage) TaskCurrent(ctx context.Context, store string, now time.Time) ([]json.RawMessage, error) {
	day, next, clock := taskDay(now)
	rows, e := s.db.QueryContext(ctx, `SELECT to_jsonb(t)||jsonb_build_object('name',tt.name,'short_name',tt.short_name,'service',tt.service,'color',tt.color,'color_dark',tt.color_dark,'icon',replace(encode(tt.icon,'base64'),chr(10),'')) FROM tasks t JOIN task_types tt ON tt.id=t.type_id WHERE t.store_id=$1::uuid AND t.start >= $2::timestamp AND t.start < $3::timestamp AND t.start <= $4::timestamp AND t.status IN ('new','in_progress') ORDER BY tt.sort_order,t.start,t.id`, store, day, next, clock)
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
 AND sc.days[(($1::date-sc.period_from) % (sc.frequency*7))+1]=1`, day, typeID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM task_status_history h USING tasks t WHERE h.task_id=t.id AND t.start >= $1::timestamp AND t.start < $2::timestamp AND t.status='new' AND ($3::bigint=0 OR t.type_id=$3) AND NOT EXISTS(SELECT 1 FROM desired_tasks d WHERE d.type_id=t.type_id AND d.store_id=t.store_id AND d.start=t.start AND d.custom_fields=t.custom_fields)`, day, next, typeID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM tasks t WHERE t.start >= $1::timestamp AND t.start < $2::timestamp AND t.status='new' AND ($3::bigint=0 OR t.type_id=$3) AND NOT EXISTS(SELECT 1 FROM desired_tasks d WHERE d.type_id=t.type_id AND d.store_id=t.store_id AND d.start=t.start AND d.custom_fields=t.custom_fields)`, day, next, typeID); e != nil {
		return e
	}
	var admin int64
	if e = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE user_token='admin'").Scan(&admin); e != nil {
		return personnelError(e)
	}
	_, e = tx.ExecContext(ctx, `WITH inserted AS (
 INSERT INTO tasks(type_id,store_id,start,status,custom_fields)
 SELECT d.type_id,d.store_id,d.start,'new',d.custom_fields FROM desired_tasks d
 WHERE NOT EXISTS(SELECT 1 FROM tasks t WHERE t.type_id=d.type_id AND t.store_id=d.store_id AND t.start=d.start AND t.custom_fields=d.custom_fields)
 RETURNING id)
 INSERT INTO task_status_history(period,task_id,user_id,status) SELECT clock_timestamp(),id,$1,'new' FROM inserted`, admin)
	if e != nil {
		return personnelError(e)
	}
	return tx.Commit()
}
