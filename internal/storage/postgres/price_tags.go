package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"rms/internal/domain/models"
	"rms/internal/storage"
)

// «Печать ценников» is a standing store task: a store has one task, created by RMS itself when a product
// first needs a tag, and it is never closed. A product needs a new price tag when something changed its tag
// (a reason, see priceTagEvents) after the last time the store printed a tag for it.
// A position is printed while printed_at >= actual_at, the time its latest reason appeared; a newer reason
// moves actual_at forward and the position is to be printed again. prev_printed_at keeps the print made
// before the position was put to print again, so a position always shows the print before actual_at.
// The task is shown to the store while it has positions to print or printed today (see priceTagShown).

// priceTagWindowDays bounds how far back reasons are looked for; reasons are counted only since the first
// «Печать ценников» task of the store, so a new store or a fresh install starts from today's changes.
const priceTagWindowDays = 60

// priceTagPending is true for a position (alias i) that is still to be printed.
func priceTagPending(i string) string {
	return fmt.Sprintf("(%[1]s.printed_at IS NULL OR %[1]s.printed_at<%[1]s.actual_at)", i)
}

// stopperInStore is true for a stopper position (alias a) with a stopper whose promotion has a
// promotional price in the price type of the store row s; positions added by hand have no document.
func stopperInStore(a string) string {
	return fmt.Sprintf(`%[1]s.stopper_id IS NOT NULL AND (%[1]s.doc_id IS NULL OR EXISTS(SELECT 1 FROM prices pp WHERE pp.doc_id=%[1]s.doc_id AND pp.product_id=%[1]s.product_id AND pp.price_type=s.price_type_promo AND pp.price>0))`, a)
}

// priceTagEvents lists reasons for a new price tag of products in the store row s from the start of the
// window to the moment clock (the scheduler wall clock in the zone tz), each with the time it appeared and
// a priority for reasons of the same time:
// stopper_bad — marked «Не соответствует» in a «Стопперы» task, promo_start / promo_end — a stopper period
// starts / ended the day before (unless another stopper runs), revaluation — a price of the store changed
// today in 1C (today_price_changes, regular or promotional), at the time RMS received the change, not before
// the day and not after the clock: a change that came after a print puts the product to print again.
func priceTagEvents(day, clock, tz string) string {
	since := fmt.Sprintf(`GREATEST(COALESCE((SELECT min(ft.start)::date FROM tasks ft JOIN task_types ftt ON ftt.id=ft.type_id WHERE ft.store_id=s.id AND ftt.service='price_print'),%[1]s),%[1]s-%[2]d)`, day, priceTagWindowDays)
	return fmt.Sprintf(`SELECT c.product_id,'stopper_bad' AS source,c.doc_id,(c.checked_at AT TIME ZONE current_setting('TimeZone')) AT TIME ZONE %[3]s AS at,1 AS priority
 FROM stopper_task_checks c JOIN tasks ct ON ct.id=c.task_id
 WHERE ct.store_id=s.id AND c.result='bad' AND ct.start>=(%[4]s)::timestamp AND ct.start<=%[2]s
 UNION ALL
 SELECT sp.product_id,'promo_start',sp.doc_id,sp.period_from::timestamp,2 FROM stopper_positions sp
 WHERE sp.period_from BETWEEN %[4]s AND %[1]s AND %[5]s
 UNION ALL
 SELECT sp.product_id,'promo_end',sp.doc_id,(sp.period_to+1)::timestamp,3 FROM stopper_positions sp
 WHERE sp.period_to+1 BETWEEN %[4]s AND %[1]s AND %[5]s
  AND NOT EXISTS(SELECT 1 FROM stopper_positions a WHERE a.product_id=sp.product_id AND a.period_from<=sp.period_to+1 AND (a.period_to IS NULL OR a.period_to>=sp.period_to+1) AND %[6]s)
 UNION ALL
 SELECT pc.product_id,'revaluation',NULL::uuid,LEAST(GREATEST(%[1]s::timestamp,max(pc.arrived_at) AT TIME ZONE %[3]s),%[2]s),4
 FROM today_price_changes pc WHERE pc.store_id=s.id GROUP BY pc.product_id`,
		day, clock, tz, since, stopperInStore("sp"), stopperInStore("a"))
}

// priceTagShown is true while the task (alias t) has a position to print or one printed on the day:
// the store sees the task on its main page, active with something to print, inactive otherwise.
func priceTagShown(day string) string {
	return fmt.Sprintf("EXISTS(SELECT 1 FROM price_tag_task_items si WHERE si.task_id=t.id AND (%s OR si.printed_at>=%s::timestamp))", priceTagPending("si"), day)
}

// priceTagPendingTags is the number of tags to print in the task (alias t).
const priceTagPendingTags = "(SELECT COALESCE(sum(pi.quantity),0) FROM price_tag_task_items pi WHERE pi.task_id=t.id AND (pi.printed_at IS NULL OR pi.printed_at<pi.actual_at))"

// priceTagPrintedBefore is the last print of the product of a position (alias i, task alias t) made before the
// position was put to print (actual_at): in this position or in earlier tasks of the store.
func priceTagPrintedBefore(i string) string {
	return fmt.Sprintf(`(SELECT max(v.x) FROM (VALUES (CASE WHEN %[1]s.printed_at<%[1]s.actual_at THEN %[1]s.printed_at END),(%[1]s.prev_printed_at),
 ((SELECT max(bp.printed_at) FROM price_tag_task_items bp JOIN tasks bt ON bt.id=bp.task_id WHERE bt.store_id=t.store_id AND bp.product_id=%[1]s.product_id AND bp.id<>%[1]s.id AND bp.printed_at<=%[1]s.actual_at))) v(x))`, i)
}

// priceTagLastPrinted is the last time the store of the row s printed a tag for the product.
func priceTagLastPrinted(product string) string {
	return fmt.Sprintf(`(SELECT max(lp.printed_at) FROM price_tag_task_items lp JOIN tasks lt ON lt.id=lp.task_id WHERE lt.store_id=s.id AND lp.product_id=%s)`, product)
}

// priceTagNeeds lists products of the store row s that need a new tag: the latest reason (product_id,
// source, doc_id, at) appeared after the last print, and the product has a regular price to print.
func priceTagNeeds(day, clock, tz string) string {
	return fmt.Sprintf(`SELECT DISTINCT ON (x.product_id) x.product_id,x.source,x.doc_id,x.at FROM (%s) x
 WHERE COALESCE(%s,'-infinity')<x.at
 AND COALESCE(%s,(SELECT rp.price FROM prices rp WHERE rp.price_type=s.price_type AND rp.product_id=x.product_id AND rp.period<=%s ORDER BY rp.period DESC,rp.doc_id DESC LIMIT 1),0)>0
 ORDER BY x.product_id,x.at DESC,x.priority`, priceTagEvents(day, clock, tz), priceTagLastPrinted("x.product_id"), todayPrice("x.product_id", false), clock)
}

// promoActive is true while the store row s has a promotional price for the product on the day: today's 1C
// change, or else the latest record of the promotional price type, is above zero (zero ends a promotion).
func promoActive(product, day string) string {
	return fmt.Sprintf(`COALESCE(%s,(SELECT pa.price FROM prices pa WHERE pa.price_type=s.price_type_promo AND pa.product_id=%s AND pa.period<(%s+1)::timestamp ORDER BY pa.period DESC,pa.doc_id DESC LIMIT 1),0)>0`, todayPrice(product, true), product, day)
}

// todayPrice is the price of the product in the store row s from today's 1C price changes (today_price_changes):
// the promotional one (0 ends a promotion) or the regular one; null when 1C sent no such change today.
// The tag is printed with these prices: a change applies on the day it came.
func todayPrice(product string, promo bool) string {
	return fmt.Sprintf("(SELECT max(tc.price) FROM today_price_changes tc WHERE tc.store_id=s.id AND tc.product_id=%s AND tc.is_promo=%t)", product, promo)
}

// priceTagClock is the day, the wall clock and the zone name of the moment.
func priceTagClock(now time.Time) (string, string, string) {
	day, _, clock := taskDay(now)
	return day, clock, now.Location().String()
}

// lockTaskStore locks the store of a task: «Печать ценников» changes are serialized per store, as task creation is.
func lockTaskStore(ctx context.Context, tx *sql.Tx, taskID int64) (string, error) {
	var store string
	if err := tx.QueryRowContext(ctx, "SELECT st.id FROM tasks t JOIN stores st ON st.id=t.store_id WHERE t.id=$1 FOR UPDATE OF st", taskID).Scan(&store); err != nil {
		return "", personnelError(err)
	}
	return store, nil
}

// priceTagTaskOpen locks an open «Печать ценников» task and returns its status.
func priceTagTaskOpen(ctx context.Context, tx *sql.Tx, taskID int64) (string, error) {
	if _, err := lockTaskStore(ctx, tx, taskID); err != nil {
		return "", err
	}
	var status, service string
	if err := tx.QueryRowContext(ctx, "SELECT t.status,tt.service FROM tasks t JOIN task_types tt ON tt.id=t.type_id WHERE t.id=$1 FOR UPDATE OF t", taskID).Scan(&status, &service); err != nil {
		return "", personnelError(err)
	}
	if service != models.TaskServicePricePrint {
		return "", storage.ErrNotFound
	}
	if status != "new" && status != "in_progress" {
		return "", storage.ErrInvalid
	}
	return status, nil
}

// startTask moves a new task to work on behalf of the user.
func startTask(ctx context.Context, tx *sql.Tx, taskID, user int64, status string) (string, error) {
	if status != "new" {
		return status, nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET status='in_progress' WHERE id=$1", taskID); err != nil {
		return "", personnelError(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO task_status_history(period,task_id,user_id,status) VALUES(clock_timestamp(),$1,$2,'in_progress')", taskID, user); err != nil {
		return "", personnelError(err)
	}
	return "in_progress", nil
}

// systemUser is the user RMS acts for in automatic task changes, as in scheduled generation.
func systemUser(ctx context.Context, tx *sql.Tx) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE user_token='admin'").Scan(&id); err != nil {
		return 0, personnelError(err)
	}
	return id, nil
}

// priceTagDefaultUnit is the unit of measure of a new position: шт, or 1 кг for weighed goods.
func priceTagDefaultUnit(product string) string {
	return fmt.Sprintf("CASE WHEN COALESCE((SELECT dp.is_weight FROM products dp WHERE dp.id=%s),false) THEN 'kg' ELSE 'pc' END", product)
}

// syncPriceTagTask adds products that need a new tag to an open task; a position that got a newer reason
// takes it over and is to be printed again (the default tag type follows the promotion of the day).
func syncPriceTagTask(ctx context.Context, tx *sql.Tx, taskID int64, now time.Time) error {
	day, clock, tz := priceTagClock(now)
	_, err := tx.ExecContext(ctx, `INSERT INTO price_tag_task_items(task_id,product_id,source,doc_id,tag_type,actual_at,unit)
 SELECT t.id,n.product_id,n.source,n.doc_id,CASE WHEN `+promoActive("n.product_id", "$2::date")+` THEN 'promo' ELSE 'regular' END,n.at,`+priceTagDefaultUnit("n.product_id")+`
 FROM tasks t JOIN stores s ON s.id=t.store_id CROSS JOIN LATERAL (`+priceTagNeeds("$2::date", "$3::timestamp", "$4::text")+`) n
 WHERE t.id=$1
 ON CONFLICT (task_id,product_id) DO UPDATE SET source=EXCLUDED.source,doc_id=EXCLUDED.doc_id,tag_type=EXCLUDED.tag_type,actual_at=EXCLUDED.actual_at
 WHERE price_tag_task_items.actual_at<EXCLUDED.actual_at`, taskID, day, clock, tz)
	return personnelError(err)
}

// refreshPriceTags keeps the standing task of a locked store up to date: the task gets new products; a store
// without one gets it when a product needs a tag, or always when create is set. It returns the task, or 0.
func refreshPriceTags(ctx context.Context, tx *sql.Tx, store string, now time.Time, create bool) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT t.id FROM tasks t JOIN task_types tt ON tt.id=t.type_id
 WHERE t.store_id=$1::uuid AND tt.service='price_print' AND t.status IN ('new','in_progress') ORDER BY t.start,t.id LIMIT 1 FOR UPDATE OF t`, store).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return 0, personnelError(err)
	}
	if id > 0 {
		return id, syncPriceTagTask(ctx, tx, id, now)
	}
	day, clock, tz := priceTagClock(now)
	if !create {
		var work bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM stores s CROSS JOIN LATERAL (`+priceTagNeeds("$2::date", "$3::timestamp", "$4::text")+`) n WHERE s.id=$1::uuid)`, store, day, clock, tz).Scan(&work); err != nil || !work {
			return 0, personnelError(err)
		}
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO tasks(type_id,store_id,start,status,custom_fields)
 SELECT id,$1::uuid,$2::timestamp,'new',custom_fields FROM task_types WHERE service='price_print' AND is_active ORDER BY sort_order,id LIMIT 1 RETURNING id`, store, clock).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, personnelError(err)
	}
	user, err := systemUser(ctx, tx)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO task_status_history(period,task_id,user_id,status) VALUES(clock_timestamp(),$1,$2,'new')", id, user); err != nil {
		return 0, personnelError(err)
	}
	return id, syncPriceTagTask(ctx, tx, id, now)
}

func lockStore(ctx context.Context, tx *sql.Tx, store string) error {
	var locked string
	return personnelError(tx.QueryRowContext(ctx, "SELECT id FROM stores WHERE id=$1::uuid FOR UPDATE", store).Scan(&locked))
}

// RefreshPriceTags brings the standing «Печать ценников» task of the store up to date (see refreshPriceTags).
func (s *Storage) RefreshPriceTags(ctx context.Context, store string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockStore(ctx, tx, store); err != nil {
		return err
	}
	if _, err = refreshPriceTags(ctx, tx, store, now, false); err != nil {
		return err
	}
	return personnelError(tx.Commit())
}

// PriceTagTask returns the store screen of a «Печать ценников» task: positions to print and positions printed
// on the day. An open task first gets products that need a new tag. Prices are the store prices of the day:
// today's 1C price changes (today_price_changes) first, then the price records.
func (s *Storage) PriceTagTask(ctx context.Context, taskID int64, now time.Time) (models.PriceTagTask, error) {
	v := models.PriceTagTask{TaskID: taskID, Items: []models.PriceTagItem{}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	if _, err = lockTaskStore(ctx, tx, taskID); err != nil {
		return v, err
	}
	var service string
	if err = tx.QueryRowContext(ctx, `SELECT t.status,to_char(t.start,'YYYY-MM-DD"T"HH24:MI:SS'),tt.service,st.id,st.code,st.name,st.address
 FROM tasks t JOIN task_types tt ON tt.id=t.type_id JOIN stores st ON st.id=t.store_id WHERE t.id=$1`, taskID).
		Scan(&v.Status, &v.Start, &service, &v.StoreID, &v.StoreCode, &v.StoreName, &v.StoreAddress); err != nil {
		return v, personnelError(err)
	}
	if service != models.TaskServicePricePrint {
		return v, storage.ErrNotFound
	}
	if v.Status == "new" || v.Status == "in_progress" {
		if err = syncPriceTagTask(ctx, tx, taskID, now); err != nil {
			return v, err
		}
	}
	day, _, _ := priceTagClock(now)
	rows, err := tx.QueryContext(ctx, `SELECT i.id,i.product_id,i.source,i.doc_id,i.tag_type,i.quantity,`+priceTagPending("i")+`,
 to_char(i.actual_at,'YYYY-MM-DD"T"HH24:MI:SS'),to_char(i.printed_at,'YYYY-MM-DD"T"HH24:MI:SS'),to_char(`+priceTagPrintedBefore("i")+`,'YYYY-MM-DD"T"HH24:MI:SS'),i.unit,
 COALESCE(p.code,''),COALESCE(p.name,''),COALESCE(p.is_weight,false),
 COALESCE((SELECT barcode FROM product_barcodes WHERE product_id=i.product_id ORDER BY is_base DESC,position LIMIT 1),''),
 (COALESCE(tr.price,reg.price)::numeric(30,2))::text,CASE WHEN tr.price IS NOT NULL THEN to_char($2::date,'YYYY-MM-DD') ELSE to_char(reg.period,'YYYY-MM-DD') END,
 CASE WHEN tp.price IS NOT NULL THEN CASE WHEN tp.price>0 THEN (tp.price::numeric(30,2))::text END
  WHEN pr.price>0 THEN (pr.price::numeric(30,2))::text END,
 CASE WHEN tp.price IS NOT NULL THEN CASE WHEN tp.price>0 THEN to_char($2::date,'YYYY-MM-DD') END
  WHEN pr.price>0 THEN to_char(pr.period,'YYYY-MM-DD') END,
 CASE WHEN tp.price IS NOT NULL THEN CASE WHEN tp.price>0 THEN to_char((SELECT sp.period_to FROM stopper_positions sp
   WHERE sp.product_id=i.product_id AND sp.period_from<=$2::date AND (sp.period_to IS NULL OR sp.period_to>=$2::date) AND `+stopperInStore("sp")+`
   ORDER BY sp.period_from DESC,sp.id DESC LIMIT 1),'YYYY-MM-DD') END
  WHEN pr.price>0 THEN to_char(COALESCE(
  (SELECT sp.period_to FROM stopper_positions sp WHERE sp.doc_id=pr.doc_id AND sp.product_id=i.product_id),
  (SELECT min(n.period)::date-1 FROM prices n WHERE n.price_type=s.price_type_promo AND n.product_id=i.product_id AND n.period>pr.period)),'YYYY-MM-DD') END
 FROM price_tag_task_items i JOIN tasks t ON t.id=i.task_id JOIN stores s ON s.id=t.store_id
 LEFT JOIN products p ON p.id=i.product_id
 LEFT JOIN LATERAL (SELECT price,period FROM prices WHERE price_type=s.price_type AND product_id=i.product_id AND period<($2::date+1)::timestamp ORDER BY period DESC,doc_id DESC LIMIT 1) reg ON true
 LEFT JOIN LATERAL (SELECT price,period,doc_id FROM prices WHERE price_type=s.price_type_promo AND product_id=i.product_id AND period<($2::date+1)::timestamp ORDER BY period DESC,doc_id DESC LIMIT 1) pr ON true
 LEFT JOIN LATERAL (SELECT `+todayPrice("i.product_id", false)+` AS price) tr ON true
 LEFT JOIN LATERAL (SELECT `+todayPrice("i.product_id", true)+` AS price) tp ON true
 WHERE i.task_id=$1 AND (`+priceTagPending("i")+` OR i.printed_at>=$2::timestamp) ORDER BY p.name,i.id`, taskID, day)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var item models.PriceTagItem
		var doc uuid.NullUUID
		if err = rows.Scan(&item.ID, &item.ProductID, &item.Source, &doc, &item.TagType, &item.Quantity, &item.Pending, &item.ActualAt, &item.PrintedAt, &item.LastPrintedAt, &item.Unit,
			&item.Code, &item.Name, &item.IsWeight, &item.Barcode, &item.Price, &item.PriceFrom, &item.PromoPrice, &item.PromoFrom, &item.PromoTo); err != nil {
			return v, err
		}
		if doc.Valid {
			item.DocID = &doc.UUID
		}
		v.Items = append(v.Items, item)
	}
	if err = rows.Err(); err != nil {
		return v, err
	}
	rows.Close()
	return v, personnelError(tx.Commit())
}

// addPriceTag adds a product to an open task by hand. A position still to be printed gets one more tag;
// a printed one is to be printed again as added by hand.
func addPriceTag(ctx context.Context, tx *sql.Tx, taskID int64, product uuid.UUID, now time.Time) error {
	day, clock, _ := priceTagClock(now)
	pending := priceTagPending("price_tag_task_items")
	res, err := tx.ExecContext(ctx, `INSERT INTO price_tag_task_items(task_id,product_id,source,tag_type,actual_at,unit)
 SELECT t.id,p.id,'manual',CASE WHEN `+promoActive("p.id", "$3::date")+` THEN 'promo' ELSE 'regular' END,$4::timestamp,CASE WHEN p.is_weight THEN 'kg' ELSE 'pc' END
 FROM tasks t JOIN stores s ON s.id=t.store_id JOIN products p ON p.id=$2 WHERE t.id=$1
 ON CONFLICT (task_id,product_id) DO UPDATE SET
  quantity=CASE WHEN `+pending+` THEN LEAST(price_tag_task_items.quantity+1,99) ELSE price_tag_task_items.quantity END,
  source=CASE WHEN `+pending+` THEN price_tag_task_items.source ELSE 'manual' END,
  doc_id=CASE WHEN `+pending+` THEN price_tag_task_items.doc_id END,
  tag_type=CASE WHEN `+pending+` THEN price_tag_task_items.tag_type ELSE EXCLUDED.tag_type END,
  actual_at=CASE WHEN `+pending+` THEN price_tag_task_items.actual_at ELSE EXCLUDED.actual_at END`, taskID, product, day, clock)
	if err != nil {
		return personnelError(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

// AddPriceTag adds a product to an open task by hand (see addPriceTag).
func (s *Storage) AddPriceTag(ctx context.Context, taskID int64, product uuid.UUID, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = priceTagTaskOpen(ctx, tx, taskID); err != nil {
		return err
	}
	if err = addPriceTag(ctx, tx, taskID, product, now); err != nil {
		return err
	}
	return personnelError(tx.Commit())
}

// SetPriceTag changes the tag type, the number of tags and the unit of measure of a position of an open task:
// шт for goods sold by the piece, 100 г or 1 кг for weighed goods.
func (s *Storage) SetPriceTag(ctx context.Context, taskID, itemID int64, in models.PriceTagItemInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = priceTagTaskOpen(ctx, tx, taskID); err != nil {
		return err
	}
	var weight bool
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(p.is_weight,false) FROM price_tag_task_items i LEFT JOIN products p ON p.id=i.product_id WHERE i.task_id=$1 AND i.id=$2", taskID, itemID).Scan(&weight); err != nil {
		return personnelError(err)
	}
	if weight != (in.Unit != models.PriceTagUnitPiece) {
		return storage.ErrInvalid
	}
	if _, err = tx.ExecContext(ctx, "UPDATE price_tag_task_items SET tag_type=$3,quantity=$4,unit=$5 WHERE task_id=$1 AND id=$2", taskID, itemID, in.TagType, in.Quantity, in.Unit); err != nil {
		return personnelError(err)
	}
	return personnelError(tx.Commit())
}

// DeletePriceTag removes a position added by hand and never printed; positions with a price reason and
// printed ones (they keep the last print time) stay. It returns the task status.
func (s *Storage) DeletePriceTag(ctx context.Context, taskID, itemID, user int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	status, err := priceTagTaskOpen(ctx, tx, taskID)
	if err != nil {
		return "", err
	}
	var source string
	var printed bool
	if err = tx.QueryRowContext(ctx, "SELECT source,printed_at IS NOT NULL FROM price_tag_task_items WHERE task_id=$1 AND id=$2", taskID, itemID).Scan(&source, &printed); err != nil {
		return "", personnelError(err)
	}
	if source != models.PriceTagManual || printed {
		return "", storage.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM price_tag_task_items WHERE task_id=$1 AND id=$2", taskID, itemID); err != nil {
		return "", personnelError(err)
	}
	return status, personnelError(tx.Commit())
}

// SetPriceTagsPrinted marks positions as printed by the user at the moment now; the first print starts a
// new task. It returns the task status.
func (s *Storage) SetPriceTagsPrinted(ctx context.Context, taskID int64, ids []int64, user int64, now time.Time) (string, error) {
	_, clock, _ := priceTagClock(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	status, err := priceTagTaskOpen(ctx, tx, taskID)
	if err != nil {
		return "", err
	}
	res, err := tx.ExecContext(ctx, "UPDATE price_tag_task_items SET prev_printed_at=CASE WHEN printed_at<actual_at THEN printed_at ELSE prev_printed_at END,printed_at=GREATEST($4::timestamp,actual_at),printed_by=$3 WHERE task_id=$1 AND id=ANY($2)", taskID, pq.Array(ids), user, clock)
	if err != nil {
		return "", personnelError(err)
	}
	if n, _ := res.RowsAffected(); n != int64(len(ids)) {
		return "", storage.ErrNotFound
	}
	if status, err = startTask(ctx, tx, taskID, user, status); err != nil {
		return "", err
	}
	if err = syncPriceTagTask(ctx, tx, taskID, now); err != nil {
		return "", err
	}
	return status, personnelError(tx.Commit())
}

// PriceTagProducts finds catalogue products by name, code or barcode for adding to the task.
func (s *Storage) PriceTagProducts(ctx context.Context, taskID int64, query string, limit int) ([]models.PriceTagProduct, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.code,p.name,
 COALESCE((SELECT barcode FROM product_barcodes WHERE product_id=p.id ORDER BY is_base DESC,position LIMIT 1),''),
 EXISTS(SELECT 1 FROM price_tag_task_items i WHERE i.task_id=$3 AND i.product_id=p.id AND `+priceTagPending("i")+`)
 FROM products p WHERE p.code ILIKE $1 OR p.name ILIKE $1 OR EXISTS(SELECT 1 FROM product_barcodes b WHERE b.product_id=p.id AND b.barcode=$4)
 ORDER BY p.name,p.id LIMIT $2`, "%"+escapeLike(query)+"%", limit, taskID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PriceTagProduct{}
	for rows.Next() {
		var v models.PriceTagProduct
		if err = rows.Scan(&v.ID, &v.Code, &v.Name, &v.Barcode, &v.Added); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// priceTagProduct tells the product card about the price tag of a product in a store: the open task,
// the position of the product in it, the last time the store printed a tag for the product and the stopper
// of the product on the day (the latest stopper period of the store that covers the day).
func priceTagProduct(ctx context.Context, tx *sql.Tx, store string, product uuid.UUID, taskID int64, now time.Time) (models.PriceTagProductState, error) {
	v := models.PriceTagProductState{}
	if taskID > 0 {
		v.TaskID = &taskID
	}
	var item sql.NullInt64
	var quantity sql.NullInt64
	var pending sql.NullBool
	err := tx.QueryRowContext(ctx, `SELECT i.id,i.source,i.quantity,`+priceTagPending("i")+`,to_char(i.actual_at,'YYYY-MM-DD"T"HH24:MI:SS'),
 (SELECT to_char(`+priceTagLastPrinted("$2")+`,'YYYY-MM-DD"T"HH24:MI:SS') FROM stores s WHERE s.id=$1::uuid)
 FROM (SELECT 1) one LEFT JOIN price_tag_task_items i ON i.task_id=$3 AND i.product_id=$2`, store, product, taskID).
		Scan(&item, &v.Source, &quantity, &pending, &v.ActualAt, &v.LastPrintedAt)
	if err != nil {
		return v, personnelError(err)
	}
	if item.Valid {
		v.ItemID = &item.Int64
		v.Quantity = int(quantity.Int64)
		v.Pending = pending.Bool
	}
	day, _, _ := priceTagClock(now)
	var stopper models.PriceTagStopper
	err = tx.QueryRowContext(ctx, `SELECT st.name,st.color FROM stores s JOIN stopper_positions sp ON sp.product_id=$2 JOIN stoppers st ON st.id=sp.stopper_id
 WHERE s.id=$1::uuid AND `+stopperInStore("sp")+` AND sp.period_from<=$3::date AND (sp.period_to IS NULL OR sp.period_to>=$3::date)
 ORDER BY sp.period_from DESC,sp.id DESC LIMIT 1`, store, product, day).Scan(&stopper.Name, &stopper.Color)
	if err == nil {
		v.Stopper = &stopper
	} else if err != sql.ErrNoRows {
		return v, personnelError(err)
	}
	return v, nil
}

// PriceTagProduct returns the price tag state of a product in a store for the product card.
func (s *Storage) PriceTagProduct(ctx context.Context, store string, product uuid.UUID, now time.Time) (models.PriceTagProductState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.PriceTagProductState{}, err
	}
	defer tx.Rollback()
	if err = lockStore(ctx, tx, store); err != nil {
		return models.PriceTagProductState{}, err
	}
	taskID, err := refreshPriceTags(ctx, tx, store, now, false)
	if err != nil {
		return models.PriceTagProductState{}, err
	}
	v, err := priceTagProduct(ctx, tx, store, product, taskID, now)
	if err != nil {
		return v, err
	}
	return v, personnelError(tx.Commit())
}

// AddPriceTagProduct puts a product into the standing «Печать ценников» task of the store, creating the
// task when the store has none yet; the tag is printed from the task.
func (s *Storage) AddPriceTagProduct(ctx context.Context, store string, product uuid.UUID, now time.Time) (models.PriceTagProductState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.PriceTagProductState{}, err
	}
	defer tx.Rollback()
	if err = lockStore(ctx, tx, store); err != nil {
		return models.PriceTagProductState{}, err
	}
	taskID, err := refreshPriceTags(ctx, tx, store, now, true)
	if err != nil {
		return models.PriceTagProductState{}, err
	}
	if taskID == 0 {
		return models.PriceTagProductState{}, storage.ErrNotFound
	}
	if err = addPriceTag(ctx, tx, taskID, product, now); err != nil {
		return models.PriceTagProductState{}, err
	}
	v, err := priceTagProduct(ctx, tx, store, product, taskID, now)
	if err != nil {
		return v, err
	}
	return v, personnelError(tx.Commit())
}
