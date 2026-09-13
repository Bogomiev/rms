package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

const productColumns = "id,code,name,parent_id,marked,marking_type,is_weight,is_thermal_mode,barcodes,created_at,updated_at"

func scanProduct(row interface{ Scan(...any) error }) (*models.Product, error) {
	var v models.Product
	var barcodes []byte
	if err := row.Scan(&v.ID, &v.Code, &v.Name, &v.ParentID, &v.Marked, &v.MarkingType, &v.IsWeight, &v.IsThermalMode, &barcodes, &v.CreatedAt, &v.UpdatedAt); err != nil {
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
	if v == nil {
		return nil, fmt.Errorf("product is required")
	}
	if v.Barcodes == nil {
		v.Barcodes = []models.BarcodeInfo{}
	}
	barcodes, err := json.Marshal(v.Barcodes)
	if err != nil {
		return nil, err
	}
	return scanProduct(s.db.QueryRowContext(ctx, `INSERT INTO products (id,code,name,parent_id,marked,marking_type,is_weight,is_thermal_mode,barcodes,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW()) RETURNING `+productColumns, v.ID, v.Code, v.Name, v.ParentID, v.Marked, v.MarkingType, v.IsWeight, v.IsThermalMode, string(barcodes)))
}
func (s *Storage) UpdateProduct(ctx context.Context, v *models.Product) (*models.Product, error) {
	if v == nil {
		return nil, fmt.Errorf("product is required")
	}
	if v.Barcodes == nil {
		v.Barcodes = []models.BarcodeInfo{}
	}
	barcodes, err := json.Marshal(v.Barcodes)
	if err != nil {
		return nil, err
	}
	return scanProduct(s.db.QueryRowContext(ctx, `UPDATE products SET code=$2,name=$3,parent_id=$4,marked=$5,marking_type=$6,is_weight=$7,is_thermal_mode=$8,barcodes=$9,updated_at=NOW() WHERE id=$1 RETURNING `+productColumns, v.ID, v.Code, v.Name, v.ParentID, v.Marked, v.MarkingType, v.IsWeight, v.IsThermalMode, string(barcodes)))
}
func (s *Storage) Products(ctx context.Context) ([]models.Product, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+productColumns+" FROM products ORDER BY id")
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
	return scanProduct(s.db.QueryRowContext(ctx, "SELECT "+productColumns+" FROM products WHERE id=$1", id))
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
