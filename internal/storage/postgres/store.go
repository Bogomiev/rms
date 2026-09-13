package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

const storeColumns = "id,code,name,address,created_at,updated_at"

func scanStore(row interface{ Scan(...any) error }) (*models.Store, error) {
	var v models.Store
	if err := row.Scan(&v.ID, &v.Code, &v.Name, &v.Address, &v.CreatedAt, &v.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}
func (s *Storage) AddStore(ctx context.Context, v *models.Store) (*models.Store, error) {
	if v == nil {
		return nil, fmt.Errorf("store is required")
	}

	return scanStore(s.db.QueryRowContext(ctx, `INSERT INTO stores (id,code,name,address,created_at,updated_at) VALUES ($1,$2,$3,$4,NOW(),NOW()) RETURNING `+storeColumns, v.ID, v.Code, v.Name, v.Address))
}
func (s *Storage) UpdateStore(ctx context.Context, v *models.Store) (*models.Store, error) {
	if v == nil {
		return nil, fmt.Errorf("store is required")
	}

	return scanStore(s.db.QueryRowContext(ctx, `UPDATE stores SET code=$2,name=$3,address=$4,updated_at=NOW() WHERE id=$1 RETURNING `+storeColumns, v.ID, v.Code, v.Name, v.Address))
}
func (s *Storage) Stores(ctx context.Context) ([]models.Store, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+storeColumns+" FROM stores ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []models.Store{}
	for rows.Next() {
		v, err := scanStore(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *v)
	}
	return result, rows.Err()
}
func (s *Storage) GetStore(ctx context.Context, id uuid.UUID) (*models.Store, error) {
	return scanStore(s.db.QueryRowContext(ctx, "SELECT "+storeColumns+" FROM stores WHERE id=$1", id))
}
func (s *Storage) DeleteStore(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM stores WHERE id=$1", id)
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
