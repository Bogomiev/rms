package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

func (s *Storage) CreateSession(ctx context.Context, ss *models.Session) (*models.Session, error) {
	const op = "storage.postgres.CreateSession"

	stmt, err := s.db.PrepareContext(ctx, "INSERT INTO sessions (id, user_id, refresh_token, is_revoked, expires_at) VALUES ($1, $2, $3, $4, $5)")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, ss.ID, ss.UserID, ss.RefreshToken, ss.IsRevoked, ss.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if err != nil {
		return nil, fmt.Errorf("error inserting session: %s: %w", op, err)
	}

	return ss, nil
}

func (s *Storage) GetSession(ctx context.Context, id string) (*models.Session, error) {
	const op = "storage.postgres.GetSession"

	stmt, err := s.db.PrepareContext(ctx, "SELECT id, user_id, refresh_token, is_revoked, expires_at FROM sessions WHERE id = $1")
	if err != nil {
		return nil, fmt.Errorf("%s: prepare statement: %w", op, err)
	}
	defer stmt.Close()

	var ss models.Session

	row := stmt.QueryRowContext(ctx, id)
	err = row.Scan(&ss.ID, &ss.UserID, &ss.RefreshToken, &ss.IsRevoked, &ss.ExpiresAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, storage.ErrSessionNotFound)
		}

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &ss, nil
}

func (s *Storage) RevokeSession(ctx context.Context, id string) error {
	const op = "storage.postgres.RevokeSession"

	result, err := s.db.ExecContext(ctx, "UPDATE sessions SET is_revoked=TRUE WHERE id=$1", id)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if count == 0 {
		return fmt.Errorf("%s: %w", op, storage.ErrSessionNotFound)
	}
	return nil
}

func (s *Storage) DeleteSession(ctx context.Context, id string) error {
	const op = "storage.postgres.DeleteSession"

	stmt, err := s.db.PrepareContext(ctx, "DELETE FROM sessions WHERE id = $1")
	if err != nil {
		return fmt.Errorf("%s: prepare statement: %w", op, err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, id)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// DeleteExpiredSessions removes at most limit rows per transaction.
func (s *Storage) DeleteExpiredSessions(ctx context.Context, limit int) (int64, error) {
	result, err := s.db.ExecContext(ctx, `WITH expired AS MATERIALIZED (
 SELECT id FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP
 ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED)
 DELETE FROM sessions USING expired WHERE sessions.id = expired.id`, limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return result.RowsAffected()
}

// RotateSession consumes a refresh token once and inserts its replacement atomically.
func (s *Storage) RotateSession(ctx context.Context, id, refresh string, next *models.Session) (*models.Session, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id=$1 AND refresh_token=$2 AND NOT is_revoked AND expires_at > CURRENT_TIMESTAMP`, id, refresh)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, storage.ErrSessionNotFound
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions (id,user_id,refresh_token,is_revoked,expires_at) VALUES ($1,$2,$3,$4,$5)`, next.ID, next.UserID, next.RefreshToken, next.IsRevoked, next.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return next, nil
}
