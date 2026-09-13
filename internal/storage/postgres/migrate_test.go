package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"rms/internal/domain/models"
)

func TestSessionMigrationAndCleanup(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is required for PostgreSQL integration tests")
	}
	for _, legacy := range []bool{false} {
		t.Run(fmt.Sprintf("legacy=%v", legacy), func(t *testing.T) {
			db, err := sql.Open("postgres", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			ctx := context.Background()
			schema := fmt.Sprintf("session_test_%d", time.Now().UnixNano())
			if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
				t.Fatal(err)
			}
			defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
			if _, err = db.Exec("SET search_path TO " + schema); err != nil {
				t.Fatal(err)
			}
			if err = migrate(ctx, db); err != nil {
				t.Fatal(err)
			}
			if err = migrate(ctx, db); err != nil {
				t.Fatalf("repeat migration: %v", err)
			}
			store := &Storage{db: db}
			if _, err = db.Exec("INSERT INTO users(id,name,user_token,password) VALUES(42,'User','old-token','hash')"); err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec("UPDATE users SET user_token='new@example.com' WHERE id=42")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"expired1", "expired2", "active"} {
				expiry := time.Now().Add(-time.Hour)
				if id == "active" {
					expiry = time.Now().Add(time.Hour)
				}
				_, err = store.CreateSession(ctx, &models.Session{ID: id, UserID: 42, RefreshToken: "refresh", ExpiresAt: expiry})
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err = store.CreateSession(ctx, &models.Session{ID: "invalid", UserID: 999, RefreshToken: "refresh", ExpiresAt: time.Now()}); err == nil {
				t.Fatal("foreign key accepted missing user")
			}
			for i := 0; i < 2; i++ {
				n, err := store.DeleteExpiredSessions(ctx, 1)
				if err != nil || n != 1 {
					t.Fatalf("batch = %d, %v", n, err)
				}
			}
			n, err := store.DeleteExpiredSessions(ctx, 1000)
			if err != nil || n != 0 {
				t.Fatalf("unexpected cleanup: %d, %v", n, err)
			}
			if _, err = store.GetSession(ctx, "active"); err != nil {
				t.Fatal("active session removed", err)
			}
			if _, err = db.Exec("DELETE FROM users WHERE id=42"); err != nil {
				t.Fatal(err)
			}
			var count int
			if err = db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("sessions not deleted with user")
			}
		})
	}
}
