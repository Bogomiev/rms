package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"rms/internal/domain/models"
)

// The transaction holds a per-dataset advisory lock across COPY and rename.
// No live table is modified until the complete replacement has been loaded.
func (s *Storage) replaceSnapshot(ctx context.Context, table string, columns []string, count int, row func(int) []any) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	lock := int64(714206914)
	if table == "stocks" {
		lock++
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lock); err != nil {
		return err
	}
	stage := table + "_staging"
	if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+stage+" (LIKE "+table+" INCLUDING ALL); TRUNCATE TABLE "+stage); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(stage, columns...))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i := 0; i < count; i++ {
		if _, err = stmt.ExecContext(ctx, row(i)...); err != nil {
			return fmt.Errorf("copy %s row %d: %w", table, i, err)
		}
	}
	if _, err = stmt.ExecContext(ctx); err != nil {
		return err
	}
	if err = stmt.Close(); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "ALTER TABLE "+table+" RENAME TO "+table+"_swap; ALTER TABLE "+stage+" RENAME TO "+table+"; ALTER TABLE "+table+"_swap RENAME TO "+stage+"; TRUNCATE TABLE "+stage); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Storage) ReplacePrices(ctx context.Context, items []models.Price) error {
	return s.replaceSnapshot(ctx, "prices", []string{"period", "price_type", "product_id", "price"}, len(items), func(i int) []any { v := items[i]; return []any{v.Period, v.PriceType, v.ProductID, v.Price.String()} })
}
func (s *Storage) ReplaceStocks(ctx context.Context, items []models.Stock) error {
	return s.replaceSnapshot(ctx, "stocks", []string{"product_id", "store_id", "stock"}, len(items), func(i int) []any { v := items[i]; return []any{v.ProductID, v.StoreID, v.Stock.String()} })
}
func (s *Storage) Prices(ctx context.Context, storeID uuid.UUID, productID *uuid.UUID) (models.Page[models.Price], error) {
	args := []any{storeID}
	query := `SELECT to_char(p.period,'YYYY-MM-DD"T"HH24:MI:SS.US'),p.price_type,p.product_id,p.price FROM prices p JOIN stores s ON s.price_type=p.price_type WHERE s.id=$1`
	if productID != nil {
		query += " AND p.product_id=$2"
		args = append(args, *productID)
	}
	query += " ORDER BY p.period,p.price_type,p.product_id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return models.Page[models.Price]{}, err
	}
	defer rows.Close()
	items := []models.Price{}
	for rows.Next() {
		var v models.Price
		if err = rows.Scan(&v.Period, &v.PriceType, &v.ProductID, &v.Price); err != nil {
			return models.Page[models.Price]{}, err
		}
		items = append(items, v)
	}
	return models.SinglePage(items), rows.Err()
}
func (s *Storage) Stocks(ctx context.Context, storeID uuid.UUID, productIDs []uuid.UUID) (models.Page[models.Stock], error) {
	args := []any{storeID}
	query := "SELECT product_id,store_id,stock FROM stocks WHERE store_id=$1"
	if len(productIDs) > 0 {
		ids := make([]string, len(productIDs))
		for i, id := range productIDs {
			ids[i] = id.String()
		}
		query += " AND product_id=ANY($2::uuid[])"
		args = append(args, pq.Array(ids))
	}
	rows, err := s.db.QueryContext(ctx, query+" ORDER BY product_id", args...)
	if err != nil {
		return models.Page[models.Stock]{}, err
	}
	defer rows.Close()
	items := []models.Stock{}
	for rows.Next() {
		var v models.Stock
		if err = rows.Scan(&v.ProductID, &v.StoreID, &v.Stock); err != nil {
			return models.Page[models.Stock]{}, err
		}
		items = append(items, v)
	}
	return models.SinglePage(items), rows.Err()
}
