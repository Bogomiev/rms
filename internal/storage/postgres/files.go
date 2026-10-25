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
func (s fileStore) List(ctx context.Context) ([]models.File, error) {
	rows, e := s.tx.QueryContext(ctx, `SELECT id,name,priority,parent_id,s3_key,is_file,icon,created_at,updated_at,deleted_at FROM files ORDER BY priority,name,id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.File{}
	for rows.Next() {
		var f models.File
		if e = rows.Scan(&f.ID, &f.Name, &f.Priority, &f.ParentID, &f.S3Key, &f.IsFile, &f.Icon, &f.CreatedAt, &f.UpdatedAt, &f.DeletedAt); e != nil {
			return nil, e
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
