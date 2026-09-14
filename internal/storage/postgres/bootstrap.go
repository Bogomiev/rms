package postgres

import (
	"context"
	"fmt"
	"rms/internal/lib/password"
)

// Bootstrap initializes missing records atomically; existing passwords are preserved.
func (s *Storage) Bootstrap(ctx context.Context, userToken, name, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(714206914)"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(user_token,name,password,is_admin) VALUES($1,$2,$3,TRUE) ON CONFLICT(user_token) DO NOTHING", userToken, name, passwordHash); err != nil {
		return err
	}
	var admin bool
	if err = tx.QueryRowContext(ctx, "SELECT is_admin FROM users WHERE user_token=$1", userToken).Scan(&admin); err != nil {
		return err
	}
	if !admin {
		return fmt.Errorf("bootstrap user_token belongs to a non-admin user; no changes applied")
	}
	return tx.Commit()
}

// ensureFirstAdmin creates the initial account only when there are no users.
// The table lock also serializes this check with ordinary user inserts.
func (s *Storage) ensureFirstAdmin(ctx context.Context, firstAdminPwd string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	if len(firstAdminPwd) < 5 || len(firstAdminPwd) > 72 {
		return fmt.Errorf("first_admin_pwd must contain 8 to 72 bytes when users is empty")
	}
	hash, err := password.HashPassword(firstAdminPwd)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO users (user_token,name,password,is_admin) VALUES ('admin','admin',$1,TRUE)", hash); err != nil {
		return err
	}
	return tx.Commit()
}
