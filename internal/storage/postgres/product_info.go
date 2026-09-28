package postgres

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"rms/internal/domain/models"
)

// Read all requested products' stock and seven current prices in one statement.
func (s *Storage) ProductValues(ctx context.Context, store uuid.UUID, ids, types []uuid.UUID, timezone string) (map[uuid.UUID]models.ProductValues, error) {
	if len(types) != 6 {
		return nil, fmt.Errorf("six marketplace price types required")
	}
	products := make([]string, len(ids))
	for i, id := range ids {
		products[i] = id.String()
	}
	args := []any{pq.Array(products), store, timezone}
	for _, id := range types {
		args = append(args, id)
	}
	query := `SELECT wanted.id,COALESCE(st.stock,0)`
	for i := 0; i < 7; i++ {
		query += fmt.Sprintf(",COALESCE(p%d.price,0)", i)
	}
	query += ` FROM unnest($1::uuid[]) AS wanted(id) LEFT JOIN stocks st ON st.product_id=wanted.id AND st.store_id=$2 LEFT JOIN stores store ON store.id=$2`
	for i := 0; i < 7; i++ {
		typ := "store.price_type"
		if i > 0 {
			typ = fmt.Sprintf("$%d::uuid", i+3)
		}
		query += fmt.Sprintf(` LEFT JOIN LATERAL (SELECT price FROM prices WHERE product_id=wanted.id AND price_type=%s AND period <= (CURRENT_TIMESTAMP AT TIME ZONE $3) ORDER BY period DESC LIMIT 1) p%d ON true`, typ, i)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[uuid.UUID]models.ProductValues{}
	for rows.Next() {
		var id uuid.UUID
		var v models.ProductValues
		if err = rows.Scan(&id, &v.Stock, &v.Price, &v.PriceEshop, &v.PromoPriceEshop, &v.PriceOzon, &v.PromoPriceOzon, &v.PriceYandexEats, &v.PromoPriceYandexEats); err != nil {
			return nil, err
		}

		result[id] = v
	}
	return result, rows.Err()
}
