package postgres

import (
	"context"
	"database/sql"
	"errors"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

// WithLoginAttempt serializes password checks across instances. State is committed
// even when check returns an authentication error, so failed attempts persist.
func (s *Storage) WithLoginAttempt(ctx context.Context, userToken string, check func(*models.LoginAttempt) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempt models.LoginAttempt
	var blocked sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id, name, user_token, password, is_admin, login_failures, login_blocked_until
	 FROM users WHERE user_token=$1 FOR UPDATE`, userToken).Scan(
		&attempt.User.ID, &attempt.User.Name, &attempt.User.UserToken, &attempt.User.Password,
		&attempt.User.IsAdmin, &attempt.Failures, &blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ErrUserNotFound
	}
	if err != nil {
		return err
	}
	attempt.BlockedUntil = blocked.Time
	authErr := check(&attempt)
	blocked = sql.NullTime{Time: attempt.BlockedUntil, Valid: !attempt.BlockedUntil.IsZero()}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET login_failures=$2, login_blocked_until=$3 WHERE id=$1`, attempt.User.ID, attempt.Failures, blocked); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return authErr
}
