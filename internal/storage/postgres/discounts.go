package postgres

import (
	"context"
	"rms/internal/domain/models"
)

// Discounts selects the current price before filtering out cancelled promotions.
func (s *Storage) Discounts(ctx context.Context, timezone string) ([]models.Discount, error) {
	rows, err := s.db.QueryContext(ctx, `
 WITH current_prices AS (
 SELECT DISTINCT ON (price_type, product_id) price_type,product_id,price,period,doc_id
 FROM prices WHERE period <= (CURRENT_TIMESTAMP AT TIME ZONE $1)
 ORDER BY price_type,product_id,period DESC,doc_id DESC
 )
 SELECT s.id,p.product_id,(p.price::numeric(30,2))::text,
 to_char(p.period,'YYYY-MM-DD HH24:MI:SS'),
 COALESCE(to_char(e.period,'YYYY-MM-DD HH24:MI:SS'),''),
 p.doc_id
 FROM current_prices p JOIN stores s ON s.price_type_promo=p.price_type
 LEFT JOIN LATERAL (
 SELECT period FROM prices
 WHERE product_id=p.product_id AND price_type=p.price_type
 AND price=0 AND period>p.period ORDER BY period LIMIT 1
 ) e ON true
 WHERE p.price>0 ORDER BY s.id,p.product_id`, timezone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.Discount{}
	for rows.Next() {
		var v models.Discount
		if err := rows.Scan(&v.StoreID, &v.ProductID, &v.Price, &v.Start, &v.End, &v.DocID); err != nil {
			return nil, err
		}
		v.WriteOffBonus = true
		items = append(items, v)
	}
	return items, rows.Err()
}
