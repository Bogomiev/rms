package user

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/storage"
)

type failingStorage struct {
	UserStorage
	err error
	ctx context.Context
}

func (s *failingStorage) AddUser(ctx context.Context, _, _, _ string, _ bool) (int64, error) {
	s.ctx = ctx
	return 0, s.err
}

func TestRegisterClassifiesStorageErrors(t *testing.T) {
	internal := errors.New("database unavailable")
	for _, tc := range []struct {
		name        string
		input, want error
	}{
		{"duplicate user_token", storage.ErrUserExists, services.ErrUserExists},
		{"internal", internal, internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &failingStorage{err: fmt.Errorf("storage: %w", tc.input)}
			service := New(slog.New(slog.NewTextHandler(io.Discard, nil)), repo)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := service.RegisterNewUser(ctx, "user_token", "name", "password", false)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if repo.ctx != ctx {
				t.Fatal("request context replaced")
			}
		})
	}
}

type lookupStorage struct {
	UserStorage
	err   error
	token string
}

func (s *lookupStorage) User(_ context.Context, value string) (models.User, error) {
	s.token = value
	return models.User{}, s.err
}
func TestUserTokenValid(t *testing.T) {
	for _, tc := range []struct {
		value          string
		err            error
		valid, wantErr bool
	}{
		{"", nil, false, false}, {"known-token", nil, true, false}, {"unknown-token", storage.ErrUserNotFound, false, false}, {"token", errors.New("database unavailable"), false, true},
	} {
		repo := &lookupStorage{err: tc.err}
		service := New(slog.Default(), repo)
		valid, err := service.UserTokenValid(context.Background(), tc.value)
		if valid != tc.valid || (err != nil) != tc.wantErr {
			t.Fatal(valid, err)
		}
		if tc.value != "" && repo.token != tc.value {
			t.Fatal("incorrect lookup token")
		}
	}
}
