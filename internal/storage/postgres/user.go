package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

// SaveUser saves user to db.
func (s *Storage) AddUser(ctx context.Context, userToken string, name string, password string, isAdmin bool) (int64, error) {
	const op = "storage.postgres.AddUser"

	stmt, err := s.db.PrepareContext(ctx, "INSERT INTO users(user_token, name, password, is_admin) VALUES($1, $2, $3, $4) RETURNING id")
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	var id int64
	err = stmt.QueryRowContext(ctx, userToken, name, password, isAdmin).Scan(&id)

	if err != nil {

		if isErrorPg(err, ERR_UniqueViolation) {
			return 0, fmt.Errorf("%s: %w", op, storage.ErrUserExists)
		}

		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

// User returns user by userToken.
func (s *Storage) User(ctx context.Context, userToken string) (models.User, error) {
	const op = "storage.postgres.User"

	stmt, err := s.db.PrepareContext(ctx, "SELECT id, name, user_token, password, is_admin FROM users WHERE user_token = $1")
	if err != nil {
		return models.User{}, fmt.Errorf("%s: prepare statement: %w", op, err)
	}
	defer stmt.Close()

	row := stmt.QueryRowContext(ctx, userToken)

	var user models.User
	err = row.Scan(&user.ID, &user.Name, &user.UserToken, &user.Password, &user.IsAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.User{}, storage.ErrUserNotFound
		}

		return models.User{}, fmt.Errorf("%s: execute statement: %w", op, err)
	}

	return user, nil
}

// User returns user by userToken.
func (s *Storage) Users(ctx context.Context, page models.UserPage) ([]*models.User, error) {
	const op = "storage.postgres.Users"
	if !page.Valid() {
		return nil, fmt.Errorf("%s: invalid pagination", op)
	}

	stmt, err := s.db.PrepareContext(ctx, "SELECT id, name, user_token, is_admin FROM users ORDER BY id LIMIT $1 OFFSET $2")
	if err != nil {
		return []*models.User{}, fmt.Errorf("%s: prepare statement: %w", op, err)
	}
	defer stmt.Close()

	var users []*models.User

	rows, err := stmt.QueryContext(ctx, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("%s: query users: %w", op, err)
	}
	defer rows.Close()

	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Name, &user.UserToken, &user.IsAdmin); err != nil {
			return []*models.User{}, fmt.Errorf("error listing users: %w", err)
		}
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: read users: %w", op, err)
	}
	return users, nil
}

func (s *Storage) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	const op = "storage.postgres.IsAdmin"

	stmt, err := s.db.PrepareContext(ctx, "SELECT is_admin FROM users WHERE id = $1")
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	row := stmt.QueryRowContext(ctx, userID)

	var isAdmin bool

	err = row.Scan(&isAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("%s: %w", op, storage.ErrUserNotFound)
		}

		return false, fmt.Errorf("%s: %w", op, err)
	}

	return isAdmin, nil
}
