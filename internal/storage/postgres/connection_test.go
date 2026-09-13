package postgres

import (
	"github.com/lib/pq"
	"net/url"
	"rms/internal/config"
	"testing"
	"time"
)

func TestConnectionStringEscapesCredentials(t *testing.T) {
	cfg := config.DbSetting{Host: "localhost", Port: 5432, User: "user name", Password: "a b'\\?@#&", DbName: "database name", SSLMode: "verify-full", ConnectTimeout: 1500 * time.Millisecond, SSLRootCert: "/tmp/root cert.pem"}
	dsn := connectionString(cfg)
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if password != cfg.Password || parsed.User.Username() != cfg.User {
		t.Fatal("credentials not preserved")
	}
	if parsed.Query().Get("connect_timeout") != "2" || parsed.Query().Get("sslmode") != "verify-full" {
		t.Fatal("connection options lost")
	}
	// lib/pq must be able to parse the escaped values into a connector without connecting.
	if _, err = pq.NewConnector(dsn); err != nil {
		t.Fatal(err)
	}
}
