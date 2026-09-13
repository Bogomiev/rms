package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"rms/internal/domain/models"
)

func TestCanceledRequestsDoNotReachDatabase(t *testing.T) {
	db, err := sql.Open("postgres", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Storage{db: db}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	operations := map[string]func() error{
		"users":          func() error { _, err := s.Users(ctx, models.UserPage{Limit: 50}); return err },
		"user":           func() error { _, err := s.User(ctx, "user_token"); return err },
		"create user":    func() error { _, err := s.AddUser(ctx, "user_token", "name", "hash", false); return err },
		"create session": func() error { _, err := s.CreateSession(ctx, &models.Session{}); return err },
		"get session":    func() error { _, err := s.GetSession(ctx, "id"); return err },
		"delete session": func() error { return s.DeleteSession(ctx, "id") },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}
