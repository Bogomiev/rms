package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"rms/internal/services"
	"rms/internal/storage"

	"rms/internal/domain/models"
	sl "rms/internal/lib/logger/slog"
	lib "rms/internal/lib/password"
)

type User struct {
	log        *slog.Logger
	usrStorage UserStorage
}

type UserStorage interface {
	AddUser(ctx context.Context, userToken string, name string, password string, isAdmin bool) (uid int64, err error)
	User(ctx context.Context, userToken string) (models.User, error)
	Users(ctx context.Context, page models.UserPage) ([]*models.User, error)
	IsAdmin(ctx context.Context, userID int64) (bool, error)
}

func New(
	log *slog.Logger,
	userStorage UserStorage,
) *User {
	return &User{
		usrStorage: userStorage,
		log:        log,
	}
}

// RegisterNewUser registers new user in the system and returns user ID.
// If user with given userToken already exists, returns error.
func (s *User) RegisterNewUser(ctx context.Context, userToken string, name string, pass string, isAdmin bool) (int64, error) {
	const op = "User.RegisterNewUser"

	log := s.log.With(
		slog.String("op", op),
	)

	log.Info("registering user")

	passHash, err := lib.HashPassword(pass)
	if err != nil {
		log.Error("failed to generate password hash", sl.Err(err))

		return 0, fmt.Errorf("%s: %w", op, err)
	}

	id, err := s.usrStorage.AddUser(ctx, userToken, name, passHash, isAdmin)
	if err != nil {
		log.Error("failed to save user", sl.Err(err))
		if errors.Is(err, storage.ErrUserExists) {
			return 0, fmt.Errorf("%s: %w", op, services.ErrUserExists)
		}

		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

// Returns a list of users.
func (s *User) UserList(ctx context.Context, page models.UserPage) ([]*models.User, error) {
	const op = "User.UserList"

	log := s.log.With(
		slog.String("op", op),
	)

	log.Info("return user list")

	users, err := s.usrStorage.Users(ctx, page)
	if err != nil {
		log.Error("failed to while generating user list", sl.Err(err))

		return []*models.User{}, fmt.Errorf("%s: %w", op, err)
	}

	return users, nil
}

// IsAdmin checks if user is admin.
func (s *User) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	const op = "User.IsAdmin"

	log := s.log.With(
		slog.String("op", op),
		slog.Int64("user_id", userID),
	)

	log.Info("checking if user is admin")

	isAdmin, err := s.usrStorage.IsAdmin(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	log.Info("checked if user is admin", slog.Bool("is_admin", isAdmin))

	return isAdmin, nil
}

func (s *User) UserTokenValid(ctx context.Context, userToken string) (bool, error) {
	if len(userToken) == 0 || len(userToken) > 255 {
		return false, nil
	}
	_, err := s.usrStorage.User(ctx, userToken)
	if errors.Is(err, storage.ErrUserNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("User.UserTokenValid: %w", err)
	}
	return true, nil
}
