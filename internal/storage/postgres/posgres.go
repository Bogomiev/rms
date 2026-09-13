package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"rms/internal/config"
	"strconv"
	"time"
)

type Storage struct{ db *sql.DB }

func connectionString(cfg config.DbSetting) string {
	values := url.Values{}
	values.Set("host", cfg.Host)
	values.Set("port", strconv.Itoa(cfg.Port))
	values.Set("dbname", cfg.DbName)
	values.Set("sslmode", cfg.SSLMode)
	values.Set("connect_timeout", strconv.FormatInt(int64((cfg.ConnectTimeout+time.Second-1)/time.Second), 10))
	for key, value := range map[string]string{"sslrootcert": cfg.SSLRootCert, "sslcert": cfg.SSLCert, "sslkey": cfg.SSLKey} {
		if value != "" {
			values.Set(key, value)
		}
	}
	return (&url.URL{Scheme: "postgres", User: url.UserPassword(cfg.User, cfg.Password), RawQuery: values.Encode()}).String()
}

func New(parent context.Context, cfg config.DbSetting, firstAdminPwd string) (*Storage, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", connectionString(cfg))
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	if err = migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	store := &Storage{db: db}
	if err := store.ensureFirstAdmin(ctx, firstAdminPwd); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize first administrator: %w", err)
	}
	return store, nil
}
func (s *Storage) Stop() error { return s.db.Close() }
