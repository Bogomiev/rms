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
	"strings"
)

const productColumns = "id,code,name,parent_id,marked,marking_type,is_weight,is_thermal_mode,created_at,updated_at"

// Aggregate child rows in the same statement to read a consistent product snapshot.
const productReadColumns = productColumns + `,COALESCE((SELECT jsonb_agg(
 jsonb_build_object('barcode',b.barcode,'unit',b.unit,'ratio',b.ratio,'isBase',b.is_base)
 ORDER BY b.position) FROM product_barcodes b WHERE b.product_id=products.id),'[]'::jsonb)`

func scanProduct(row interface{ Scan(...any) error }) (*models.Product, error) {
	var v models.Product
	var barcodes []byte
	if err := row.Scan(&v.ID, &v.Code, &v.Name, &v.ParentID, &v.Marked, &v.MarkingType, &v.IsWeight, &v.IsThermalMode, &v.CreatedAt, &v.UpdatedAt, &barcodes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(barcodes, &v.Barcodes); err != nil {
		return nil, err
	}
	if v.Barcodes == nil {
		v.Barcodes = []models.BarcodeInfo{}
	}
	return &v, nil
}
func (s *Storage) AddProduct(ctx context.Context, v *models.Product) (*models.Product, error) {
	return s.writeProduct(ctx, v, false)
}
func (s *Storage) UpdateProduct(ctx context.Context, v *models.Product) (*models.Product, error) {
	return s.writeProduct(ctx, v, true)
}

func (s *Storage) writeProduct(ctx context.Context, v *models.Product, update bool) (*models.Product, error) {
	if v == nil {
		return nil, fmt.Errorf("product is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	query := `INSERT INTO products (id,code,name,parent_id,marked,marking_type,is_weight,is_thermal_mode) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	if update {
		query = `UPDATE products SET code=$2,name=$3,parent_id=$4,marked=$5,marking_type=$6,is_weight=$7,is_thermal_mode=$8,updated_at=NOW() WHERE id=$1`
	}
	result, err := tx.ExecContext(ctx, query, v.ID, v.Code, v.Name, v.ParentID, v.Marked, v.MarkingType, v.IsWeight, v.IsThermalMode)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, storage.ErrNotFound
	}
	if err = replaceProductBarcodes(ctx, tx, []models.Product{*v}); err != nil {
		return nil, err
	}
	saved, err := scanProduct(tx.QueryRowContext(ctx, "SELECT "+productReadColumns+" FROM products WHERE id=$1", v.ID))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return saved, nil
}

func (s *Storage) Products(ctx context.Context) ([]models.Product, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+productReadColumns+" FROM products ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []models.Product{}
	for rows.Next() {
		v, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *v)
	}
	return result, rows.Err()
}
func (s *Storage) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return scanProduct(s.db.QueryRowContext(ctx, "SELECT "+productReadColumns+" FROM products WHERE id=$1", id))
}
func (s *Storage) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM products WHERE id=$1", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

// UpsertProducts writes the entire batch and its barcodes in one transaction.
// Fields absent from GetGoods (parent_id and marked) are preserved on updates.
func (s *Storage) UpsertProducts(ctx context.Context, products []models.Product) error {
	if len(products) == 0 {
		return nil
	}
	values := make([]string, 0, len(products))
	args := make([]any, 0, len(products)*7)
	for _, v := range products {
		n := len(args)
		values = append(values, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7))
		args = append(args, v.ID, v.Code, v.Name, uuid.Nil, v.MarkingType, v.IsWeight, v.IsThermalMode)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO products
 (id,code,name,parent_id,marking_type,is_weight,is_thermal_mode)
 VALUES `+strings.Join(values, ",")+`
 ON CONFLICT (id) DO UPDATE SET
 code=EXCLUDED.code,name=EXCLUDED.name,marking_type=EXCLUDED.marking_type,
 is_weight=EXCLUDED.is_weight,is_thermal_mode=EXCLUDED.is_thermal_mode,
 updated_at=NOW()`, args...)
	if err != nil {
		return err
	}
	if err = replaceProductBarcodes(ctx, tx, products); err != nil {
		return err
	}
	return tx.Commit()
}

// Parent rows are locked by the preceding INSERT/UPDATE for the transaction duration.
func replaceProductBarcodes(ctx context.Context, tx *sql.Tx, products []models.Product) error {
	ids := make([]string, len(products))
	for i, v := range products {
		ids[i] = v.ID.String()
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM product_barcodes WHERE product_id = ANY($1::uuid[])", pq.Array(ids)); err != nil {
		return err
	}
	values := []string{}
	args := []any{}
	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO product_barcodes (product_id,position,barcode,unit,ratio,is_base) VALUES `+strings.Join(values, ","), args...)
		values = nil
		args = nil
		return err
	}
	for _, v := range products {
		for position, b := range v.Barcodes {
			n := len(args)
			values = append(values, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d)", n+1, n+2, n+3, n+4, n+5, n+6))
			args = append(args, v.ID, position, b.Barcode, b.Unit, b.Ratio, b.IsBase)
			if len(values) == 1000 {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	return flush()
}
