package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"rms/internal/lib/password"
)

func TestFirstAdminInitialization(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("first_admin_test_%d", time.Now().UnixNano())
	if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	if _, err = db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	s := &Storage{db: db}
	for _, invalid := range []string{"", "short", strings.Repeat("x", 73)} {
		if err = s.ensureFirstAdmin(ctx, invalid); err == nil {
			t.Fatal("invalid initial password accepted")
		}
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	const initialPassword = "initial-password"
	if err = s.ensureFirstAdmin(ctx, initialPassword); err != nil {
		t.Fatal(err)
	}
	admin, err := s.User(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if admin.Name != "admin" || !admin.IsAdmin || admin.Password == initialPassword {
		t.Fatal("invalid first admin")
	}
	if err = password.CheckPassword(initialPassword, admin.Password); err != nil {
		t.Fatal("password hash does not match", err)
	}
	for _, value := range []string{"another-password", ""} {
		if err = s.ensureFirstAdmin(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	unchanged, err := s.User(ctx, "admin")
	if err != nil || unchanged != admin {
		t.Fatal("existing user changed", err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err = db.Exec("DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddUser(ctx, "employee", "Employee", "existing-hash", false); err != nil {
		t.Fatal(err)
	}
	if err = s.ensureFirstAdmin(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	employee, err := s.User(ctx, "employee")
	if err != nil || employee.IsAdmin || employee.Password != "existing-hash" {
		t.Fatal("existing employee changed", err)
	}
	if _, err = s.User(ctx, "admin"); err == nil {
		t.Fatal("admin created in nonempty users table")
	}
}
