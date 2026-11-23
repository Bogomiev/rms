package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

// promoPositions lists products of 1C promotion documents in the promotional price types of stores.
// A position starts with its promotional price and lasts until the day before the next price
// record of the same type; it has no end while any of its price types has no next record.
func promoPositions(filter string) string {
	return `promo_types AS (
 SELECT DISTINCT price_type_promo AS id FROM stores WHERE price_type_promo<>'00000000-0000-0000-0000-000000000000'
 ),
 promo AS (
 SELECT p.doc_id,p.product_id,min(p.period)::date AS start,array_agg(DISTINCT p.price_type) AS types,
 CASE WHEN bool_and(n.period IS NOT NULL) THEN GREATEST(min(p.period)::date,max(n.period)::date-1) END AS finish
 FROM prices p JOIN promo_types t ON t.id=p.price_type
 LEFT JOIN LATERAL (
 SELECT z.period FROM prices z WHERE z.price_type=p.price_type AND z.product_id=p.product_id AND z.period>p.period
 ORDER BY z.period LIMIT 1
 ) n ON true
 WHERE p.price>0` + filter + `
 GROUP BY p.doc_id,p.product_id
 )`
}

func (s *Storage) Stoppers(ctx context.Context) ([]models.Stopper, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.name,s.color,s.days,count(p.id) FROM stoppers s
 LEFT JOIN stopper_positions p ON p.stopper_id=s.id GROUP BY s.id ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Stopper{}
	for rows.Next() {
		var v models.Stopper
		if err = rows.Scan(&v.ID, &v.Name, &v.Color, &v.Days, &v.Positions); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Storage) AddStopper(ctx context.Context, in models.StopperInput) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, "INSERT INTO stoppers(name,color,days) VALUES($1,$2,$3) RETURNING id", in.Name, in.Color, in.Days).Scan(&id)
	return id, personnelError(err)
}
func (s *Storage) SaveStopper(ctx context.Context, id int64, in models.StopperInput) error {
	return affected(s.db.ExecContext(ctx, "UPDATE stoppers SET name=$2,color=$3,days=$4 WHERE id=$1", id, in.Name, in.Color, in.Days))
}

// DeleteStopper keeps the positions; they become positions without a stopper.
func (s *Storage) DeleteStopper(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, "DELETE FROM stoppers WHERE id=$1", id))
}

func (s *Storage) StopperPositions(ctx context.Context, f models.StopperPositionFilter, timezone string) ([]models.StopperPosition, error) {
	stores := make([]string, len(f.Stores))
	for i, id := range f.Stores {
		stores[i] = id.String()
	}
	rows, err := s.db.QueryContext(ctx, `WITH `+promoPositions("")+`,
 positions AS (
 SELECT COALESCE(sp.doc_id,pr.doc_id) AS doc_id,COALESCE(sp.product_id,pr.product_id) AS product_id,sp.stopper_id,
 COALESCE(sp.period_from,pr.start) AS period_from,
 CASE WHEN sp.id IS NULL THEN pr.finish ELSE sp.period_to END AS period_to,
 COALESCE(pr.start,sp.created_at::date) AS received,
 pr.doc_id IS NULL AND sp.doc_id IS NULL AS manual,pr.types
 FROM promo pr FULL JOIN stopper_positions sp ON sp.doc_id=pr.doc_id AND sp.product_id=pr.product_id
 )
 SELECT ps.doc_id,ps.product_id,COALESCE(p.code,''),COALESCE(p.name,''),COALESCE(p.is_thermal_mode,false),ps.stopper_id,
 to_char(ps.period_from,'YYYY-MM-DD'),to_char(ps.period_to,'YYYY-MM-DD'),to_char(ps.received,'YYYY-MM-DD'),ps.manual,
 (SELECT count(*) FROM stores WHERE price_type_promo=ANY(ps.types)),
 (SELECT COALESCE(json_agg(json_build_object('store_id',st.id,'price',(r.price::numeric(30,2))::text,'promo_price',(pp.price::numeric(30,2))::text) ORDER BY o.n),'[]')
 FROM unnest($3::uuid[]) WITH ORDINALITY o(id,n) JOIN stores st ON st.id=o.id
 LEFT JOIN LATERAL (
 SELECT price FROM prices WHERE price_type=st.price_type AND product_id=ps.product_id AND period<=(CURRENT_TIMESTAMP AT TIME ZONE $4)
 ORDER BY period DESC,doc_id DESC LIMIT 1
 ) r ON true
 LEFT JOIN LATERAL (
 SELECT price FROM prices WHERE price_type=st.price_type_promo AND product_id=ps.product_id AND doc_id=ps.doc_id AND price>0
 ORDER BY period LIMIT 1
 ) pp ON true)
 FROM positions ps
 LEFT JOIN products p ON p.id=ps.product_id
 WHERE ps.period_from<=$2::date AND (ps.period_to IS NULL OR ps.period_to>=$1::date)
 ORDER BY p.name,ps.product_id,ps.period_from,ps.doc_id`, f.From, f.To, pq.Array(stores), timezone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.StopperPosition{}
	for rows.Next() {
		var v models.StopperPosition
		var doc uuid.NullUUID
		var stopper sql.NullInt64
		var prices []byte
		if err = rows.Scan(&doc, &v.ProductID, &v.Code, &v.Name, &v.ThermalMode, &stopper, &v.From, &v.To, &v.Received, &v.Manual, &v.Stores, &prices); err != nil {
			return nil, err
		}
		if doc.Valid {
			v.DocID = &doc.UUID
		}
		if stopper.Valid {
			v.StopperID = &stopper.Int64
		}
		if err = json.Unmarshal(prices, &v.Prices); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// positionID returns the stored row of a position. A promotional position gets a row
// with its 1C period on first change; a manual position must already exist.
func positionID(ctx context.Context, tx *sql.Tx, key models.StopperPositionKey) (int64, error) {
	var id int64
	if key.DocID == nil {
		err := tx.QueryRowContext(ctx, "SELECT id FROM stopper_positions WHERE doc_id IS NULL AND product_id=$1", key.ProductID).Scan(&id)
		return id, personnelError(err)
	}
	err := tx.QueryRowContext(ctx, `WITH `+promoPositions(" AND p.doc_id=$1 AND p.product_id=$2")+`
 INSERT INTO stopper_positions(product_id,doc_id,period_from,period_to)
 SELECT product_id,doc_id,start,finish FROM promo
 ON CONFLICT (doc_id,product_id) WHERE doc_id IS NOT NULL DO NOTHING RETURNING id`, *key.DocID, key.ProductID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, "SELECT id FROM stopper_positions WHERE doc_id=$1 AND product_id=$2", *key.DocID, key.ProductID).Scan(&id)
	}
	return id, personnelError(err)
}
func (s *Storage) updatePositions(ctx context.Context, keys []models.StopperPositionKey, query string, args ...any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, key := range keys {
		id, err := positionID(ctx, tx, key)
		if err != nil {
			return err
		}
		if err = affected(tx.ExecContext(ctx, query, append([]any{id}, args...)...)); err != nil {
			return err
		}
	}
	return personnelError(tx.Commit())
}

// SetPositionsStopper sets or, with a nil stopper, removes the stopper of positions. A newly set stopper
// with a duration ends the position on the start date plus its days, unless the position ends earlier
// (LEAST skips a missing end); setting the same stopper again or removing it keeps the period.
func (s *Storage) SetPositionsStopper(ctx context.Context, keys []models.StopperPositionKey, stopperID *int64) error {
	return s.updatePositions(ctx, keys, `UPDATE stopper_positions p SET stopper_id=$2,
 period_to=CASE WHEN d.days IS NULL OR p.stopper_id IS NOT DISTINCT FROM $2 THEN p.period_to ELSE LEAST(p.period_to,p.period_from+d.days) END
 FROM (SELECT (SELECT days FROM stoppers WHERE id=$2) AS days) d WHERE p.id=$1`, stopperID)
}

// SetPositionsPeriod sets the period of positions; a nil end means a period without an end.
func (s *Storage) SetPositionsPeriod(ctx context.Context, keys []models.StopperPositionKey, from string, to *string) error {
	return s.updatePositions(ctx, keys, "UPDATE stopper_positions SET period_from=$2::date,period_to=$3::date WHERE id=$1", from, to)
}

// AddManualPosition adds a product by hand; a stopper with a duration gives it an end date.
func (s *Storage) AddManualPosition(ctx context.Context, in models.StopperManualInput) error {
	return affected(s.db.ExecContext(ctx, `INSERT INTO stopper_positions(product_id,stopper_id,period_from,period_to)
 SELECT $1,s.id,$3::date,$3::date+s.days FROM stoppers s WHERE s.id=$2`, in.ProductID, in.StopperID, in.From))
}

// StopperProducts searches the catalog by code or name for manual addition.
func (s *Storage) StopperProducts(ctx context.Context, query string, limit int) ([]models.StopperProduct, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.code,p.name,
 EXISTS(SELECT 1 FROM stopper_positions sp WHERE sp.doc_id IS NULL AND sp.product_id=p.id)
 FROM products p WHERE p.code ILIKE $1 OR p.name ILIKE $1 ORDER BY p.name,p.id LIMIT $2`, "%"+escapeLike(query)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.StopperProduct{}
	for rows.Next() {
		var v models.StopperProduct
		if err = rows.Scan(&v.ID, &v.Code, &v.Name, &v.Added); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// stopperTaskRemoveDays is how long an ended stopper stays in store tasks for removal.
const stopperTaskRemoveDays = 14

// stopperTaskPosition filters stopper_positions sp for the store row s on the day: a stopper is set,
// it has started, it is running or ended within stopperTaskRemoveDays, and a promotion position
// has a promotional price in the store price type.
func stopperTaskPosition(day string) string {
	return fmt.Sprintf(`sp.stopper_id IS NOT NULL AND sp.period_from<=%[1]s
 AND (sp.period_to IS NULL OR sp.period_to>=%[1]s-%[2]d)
 AND (sp.doc_id IS NULL OR EXISTS(SELECT 1 FROM prices pp WHERE pp.doc_id=sp.doc_id AND pp.product_id=sp.product_id AND pp.price_type=s.price_type_promo AND pp.price>0))`, day, stopperTaskRemoveDays)
}

// stopperTaskWork is true when a «Стопперы» task of the store would list anything on the day.
func stopperTaskWork(store, day string) string {
	return fmt.Sprintf(`EXISTS(SELECT 1 FROM stopper_positions sp JOIN stores s ON s.id=%s WHERE %s)`, store, stopperTaskPosition(day))
}

// StopperTask returns positions with a stopper for the task store on the given day: active ones
// to check and ones ended within two weeks to remove, unless the product has an active stopper.
func (s *Storage) StopperTask(ctx context.Context, taskID int64, today string, timezone string) (models.StopperTask, error) {
	v := models.StopperTask{TaskID: taskID, Items: []models.StopperTaskItem{}}
	var service string
	err := s.db.QueryRowContext(ctx, `SELECT t.status,to_char(t.start,'YYYY-MM-DD"T"HH24:MI:SS'),tt.service,st.id,st.code,st.name,st.address
 FROM tasks t JOIN task_types tt ON tt.id=t.type_id JOIN stores st ON st.id=t.store_id WHERE t.id=$1`, taskID).
		Scan(&v.Status, &v.Start, &service, &v.StoreID, &v.StoreCode, &v.StoreName, &v.StoreAddress)
	if err != nil {
		return v, personnelError(err)
	}
	if service != models.TaskServiceStoppers {
		return v, storage.ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `WITH store AS (SELECT id,price_type,price_type_promo FROM stores WHERE id=$1),
 pos AS (
 SELECT sp.*,sp.period_to IS NOT NULL AND sp.period_to<$2::date AS remove
 FROM stopper_positions sp CROSS JOIN store s
 WHERE `+stopperTaskPosition("$2::date")+`
 )
 SELECT ps.doc_id,ps.product_id,COALESCE(p.code,''),COALESCE(p.name,''),COALESCE(p.is_thermal_mode,false),
 CASE WHEN p.is_weight THEN 'кг' ELSE 'шт' END,sr.name,sr.color,
 to_char(ps.period_from,'YYYY-MM-DD'),to_char(ps.period_to,'YYYY-MM-DD'),ps.remove,
 (SELECT stock::text FROM stocks WHERE store_id=s.id AND product_id=ps.product_id),
 (SELECT (price::numeric(30,2))::text FROM prices WHERE price_type=s.price_type AND product_id=ps.product_id AND period<=(CURRENT_TIMESTAMP AT TIME ZONE $4) ORDER BY period DESC,doc_id DESC LIMIT 1),
 (SELECT (price::numeric(30,2))::text FROM prices WHERE price_type=s.price_type_promo AND product_id=ps.product_id AND doc_id=ps.doc_id AND price>0 ORDER BY period LIMIT 1),
 COALESCE((SELECT barcode FROM product_barcodes WHERE product_id=ps.product_id ORDER BY is_base DESC,position LIMIT 1),''),
 COALESCE((SELECT result FROM stopper_task_checks c WHERE c.task_id=$3 AND c.product_id=ps.product_id AND c.doc_id IS NOT DISTINCT FROM ps.doc_id),'')
 FROM pos ps CROSS JOIN store s
 JOIN stoppers sr ON sr.id=ps.stopper_id
 LEFT JOIN products p ON p.id=ps.product_id
 WHERE NOT ps.remove OR NOT EXISTS(SELECT 1 FROM pos a WHERE a.product_id=ps.product_id AND NOT a.remove)
 ORDER BY ps.remove DESC,p.name,ps.product_id,ps.doc_id`, v.StoreID, today, taskID, timezone)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var item models.StopperTaskItem
		var doc uuid.NullUUID
		if err = rows.Scan(&doc, &item.ProductID, &item.Code, &item.Name, &item.ThermalMode, &item.Unit, &item.Stopper, &item.StopperColor,
			&item.From, &item.To, &item.Remove, &item.Stock, &item.Price, &item.PromoPrice, &item.Barcode, &item.Result); err != nil {
			return v, err
		}
		if doc.Valid {
			item.DocID = &doc.UUID
		}
		v.Items = append(v.Items, item)
	}
	return v, rows.Err()
}

// SetStopperTaskCheck saves or clears a store result for an open «Стопперы» task.
// The first result starts a new task on behalf of the user.
func (s *Storage) SetStopperTaskCheck(ctx context.Context, taskID int64, in models.StopperTaskCheckInput, user int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var status, service string
	if err = tx.QueryRowContext(ctx, "SELECT t.status,tt.service FROM tasks t JOIN task_types tt ON tt.id=t.type_id WHERE t.id=$1 FOR UPDATE OF t", taskID).Scan(&status, &service); err != nil {
		return "", personnelError(err)
	}
	if service != models.TaskServiceStoppers {
		return "", storage.ErrNotFound
	}
	if status != "new" && status != "in_progress" {
		return "", storage.ErrInvalid
	}
	if in.Result == "" {
		_, err = tx.ExecContext(ctx, "DELETE FROM stopper_task_checks WHERE task_id=$1 AND product_id=$2 AND doc_id IS NOT DISTINCT FROM $3::uuid", taskID, in.ProductID, in.DocID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO stopper_task_checks(task_id,product_id,doc_id,result,user_id) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT (task_id,product_id,COALESCE(doc_id,'00000000-0000-0000-0000-000000000000'))
 DO UPDATE SET result=EXCLUDED.result,user_id=EXCLUDED.user_id,checked_at=CURRENT_TIMESTAMP`, taskID, in.ProductID, in.DocID, in.Result, user)
	}
	if err != nil {
		return "", personnelError(err)
	}
	if status == "new" && in.Result != "" {
		status = "in_progress"
		if _, err = tx.ExecContext(ctx, "UPDATE tasks SET status='in_progress' WHERE id=$1", taskID); err != nil {
			return "", personnelError(err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO task_status_history(period,task_id,user_id,status) VALUES(clock_timestamp(),$1,$2,'in_progress')", taskID, user); err != nil {
			return "", personnelError(err)
		}
	}
	return status, personnelError(tx.Commit())
}

// StopperManager reports whether a user may manage stoppers: an admin or a user whose position has the manager role.
func (s *Storage) StopperManager(ctx context.Context, userID int64) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT u.is_admin OR EXISTS(SELECT 1 FROM position_roles pr JOIN roles r ON r.id=pr.role_id WHERE pr.position_id=u.position AND r.name='manager')
 FROM users u WHERE u.id=$1`, userID).Scan(&ok)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return ok, err
}
