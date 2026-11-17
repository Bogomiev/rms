package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/lib/pq"
	"rms/internal/domain/models"
	"rms/internal/services/files"
)

type fileStore struct{ tx *sql.Tx }

// A transaction-scoped lock serializes tree validation and S3 mutations across instances.
func (s *Storage) Transaction(ctx context.Context, fn func(files.Store) error) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(714206914)"); e != nil {
		return e
	}
	if e = fn(fileStore{tx}); e != nil {
		return fileError(e)
	}
	return fileError(tx.Commit())
}
func fileError(e error) error {
	var p *pq.Error
	if errors.As(e, &p) {
		switch p.Code {
		case "23505", "23503":
			return files.ErrConflict
		case "23514", "22P02":
			return files.ErrInvalid
		}
	}
	return e
}

// View keeps path traversal on one snapshot without serializing readers and writers.
func (s *Storage) View(ctx context.Context, fn func(files.Store) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(fileStore{tx}); err != nil {
		return fileError(err)
	}
	return fileError(tx.Commit())
}

const fileColumns = "id,name,priority,parent_id,s3_key,is_file,icon,created_at,updated_at,deleted_at"

func scanFile(row interface{ Scan(...any) error }) (models.File, error) {
	var f models.File
	err := row.Scan(&f.ID, &f.Name, &f.Priority, &f.ParentID, &f.S3Key, &f.IsFile, &f.Icon, &f.CreatedAt, &f.UpdatedAt, &f.DeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = files.ErrNotFound
	}
	return f, err
}
func (s fileStore) Get(ctx context.Context, id string) (models.File, error) {
	return scanFile(s.tx.QueryRowContext(ctx, "SELECT "+fileColumns+" FROM files WHERE id=$1", id))
}
func (s fileStore) List(ctx context.Context, parent *string, deleted bool) ([]models.File, error) {
	query := "SELECT " + fileColumns + " FROM files WHERE parent_id IS NULL"
	args := []any{}
	if parent != nil {
		query = "SELECT " + fileColumns + " FROM files WHERE parent_id=$1"
		args = append(args, *parent)
	}
	if !deleted {
		query += " AND deleted_at IS NULL"
	}
	rows, err := s.tx.QueryContext(ctx, query+" ORDER BY priority,name,id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s fileStore) Save(ctx context.Context, f models.File) error {
	_, e := s.tx.ExecContext(ctx, `INSERT INTO files(id,name,priority,parent_id,s3_key,is_file,icon,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,priority=EXCLUDED.priority,parent_id=EXCLUDED.parent_id,s3_key=EXCLUDED.s3_key,icon=EXCLUDED.icon,updated_at=EXCLUDED.updated_at,deleted_at=EXCLUDED.deleted_at`, f.ID, f.Name, f.Priority, f.ParentID, f.S3Key, f.IsFile, f.Icon, f.CreatedAt, f.UpdatedAt, f.DeletedAt)
	return e
}
func (s fileStore) Delete(ctx context.Context, id string) error {
	_, e := s.tx.ExecContext(ctx, "DELETE FROM files WHERE id=$1", id)
	return e
}

func (s fileStore) Find(ctx context.Context, parent *string, name string, deleted bool) (models.File, error) {
	query := "SELECT " + fileColumns + " FROM files WHERE parent_id IS NULL AND name=$1"
	args := []any{name}
	if parent != nil {
		query = "SELECT " + fileColumns + " FROM files WHERE parent_id=$2 AND name=$1"
		args = append(args, *parent)
	}
	if !deleted {
		query += " AND deleted_at IS NULL"
	}
	return scanFile(s.tx.QueryRowContext(ctx, query, args...))
}
func (s fileStore) HasChildren(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM files WHERE parent_id=$1)", id).Scan(&exists)
	return exists, err
}
