package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"rms/internal/domain/models"
	"rms/internal/lib/password"
	"rms/internal/services"
	"rms/internal/services/auth"
	"rms/internal/storage"
	"rms/internal/token"
)

func authTestStorage(t *testing.T) *Storage {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("auth_test_%d", time.Now().UnixNano())
	if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	// Every pooled connection uses the isolated test schema, including concurrent logins.
	u, err := url.Parse(dsn)
	if err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
	} else {
		dsn += " search_path=" + schema
	}
	testDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Close(); db.Exec("DROP SCHEMA " + schema + " CASCADE"); db.Close() })
	testDB.SetMaxOpenConns(10)
	if err = migrate(context.Background(), testDB); err != nil {
		t.Fatal(err)
	}
	return &Storage{db: testDB}
}

func TestConcurrentLoginLockoutPersists(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	hash, err := password.HashPassword("12345")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddUser(ctx, "invite", "User", hash, false); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	newAuth := func() *auth.Auth {
		return auth.New(auth.Config{TokenTTL: time.Minute, RefreshTokenTTL: time.Hour, MaxLoginAttempts: 5, LoginBlockDuration: 15 * time.Minute}, auth.Dependencies{Logger: log, Users: s, Sessions: s, Tokens: token.NewJWTMaker("test-secret")})
	}
	results := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, _, _, err := newAuth().Login(ctx, "invite", "00000"); results <- err }()
	}
	wg.Wait()
	close(results)
	invalid, blocked := 0, 0
	for err := range results {
		var lock *services.LoginBlockedError
		switch {
		case errors.As(err, &lock):
			blocked++
		case errors.Is(err, services.ErrInvalidCredentials):
			invalid++
		default:
			t.Fatal(err)
		}
	}
	if invalid != 4 || blocked != 8 {
		t.Fatalf("invalid=%d blocked=%d", invalid, blocked)
	}
	var failures int
	var until time.Time
	if err = s.db.QueryRow("SELECT login_failures,login_blocked_until FROM users WHERE user_token='invite'").Scan(&failures, &until); err != nil {
		t.Fatal(err)
	}
	if failures != 5 {
		t.Fatal("counter bypassed by concurrency", failures)
	}
	// A fresh service instance and correct PIN cannot bypass the persisted block.
	_, _, _, _, err = newAuth().Login(ctx, "invite", "12345")
	var lock *services.LoginBlockedError
	if !errors.As(err, &lock) || !lock.Until.Equal(until) {
		t.Fatal("block not persisted", err)
	}
	if _, err = s.db.Exec("UPDATE users SET login_blocked_until=CURRENT_TIMESTAMP-INTERVAL '1 second'"); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err = newAuth().Login(ctx, "invite", "12345")
	if err != nil {
		t.Fatal(err)
	}
	var blockedUntil sql.NullTime
	if err = s.db.QueryRow("SELECT login_failures,login_blocked_until FROM users WHERE user_token='invite'").Scan(&failures, &blockedUntil); err != nil {
		t.Fatal(err)
	}
	if failures != 0 || blockedUntil.Valid {
		t.Fatal("successful login did not reset state")
	}
}

func TestRefreshConsumedOnceAndRollback(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	id, err := s.AddUser(ctx, "invite", "User", "hash", false)
	if err != nil {
		t.Fatal(err)
	}
	old := &models.Session{ID: "old", UserID: id, RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if _, err = s.CreateSession(ctx, old); err != nil {
		t.Fatal(err)
	}
	// Failed replacement must preserve the old session.
	if _, err = s.RotateSession(ctx, old.ID, old.RefreshToken, &models.Session{ID: "bad", UserID: id + 999, RefreshToken: "bad", ExpiresAt: old.ExpiresAt}); err == nil {
		t.Fatal("invalid owner accepted")
	}
	if _, err = s.GetSession(ctx, old.ID); err != nil {
		t.Fatal("rotation lost session on failure", err)
	}
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.RotateSession(ctx, old.ID, old.RefreshToken, &models.Session{ID: fmt.Sprint("new", i), UserID: id, RefreshToken: fmt.Sprint("token", i), ExpiresAt: old.ExpiresAt})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, storage.ErrSessionNotFound) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("refresh replay succeeded", success)
	}
	if _, err = s.GetSession(ctx, old.ID); !errors.Is(err, storage.ErrSessionNotFound) {
		t.Fatal("consumed session survived", err)
	}
}
