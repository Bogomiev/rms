package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	httpapp "rms/internal/app/http"
	"rms/internal/config"
	"rms/internal/scheduler"
	"rms/internal/services/auth"
	"rms/internal/services/onec"
	"rms/internal/services/product"
	"rms/internal/services/store"
	"rms/internal/services/user"
	"rms/internal/storage/postgres"
	"rms/internal/token"
)

type App struct {
	HTTPServer *httpapp.App
	storage    storage
	scheduler  *scheduler.Scheduler
}

type storage interface{ Stop() error }

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	initCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	db, err := postgres.New(initCtx, cfg.Db, cfg.FirstAdminPwd)
	if err != nil {
		return nil, fmt.Errorf("initialize storage: %w", err)
	}
	tokens := token.NewJWTMaker(cfg.SigningKey)
	authService := auth.New(auth.Config{
		TokenTTL: cfg.TokenTTL, RefreshTokenTTL: cfg.RefreshTokenTTL, MaxLoginAttempts: cfg.MaxLoginAttempts, LoginBlockDuration: cfg.LoginBlockDuration,
	}, auth.Dependencies{Logger: log, Users: db, Sessions: db, Tokens: tokens})
	userService := user.New(log, db)
	storeService := store.New(log, db)
	productService := product.New(log, db)
	oneCService, err := onec.New(cfg.OneC, storeService, productService)
	if err != nil {
		return nil, errors.Join(err, db.Stop())
	}
	server := httpapp.New(httpapp.Config{
		TokenTTL: cfg.TokenTTL, RefreshTokenTTL: cfg.RefreshTokenTTL, AppOrigins: cfg.AllowedAppOrigins(), Port: cfg.Port, Timeout: cfg.Timeout, IdleTimeout: cfg.IdleTimeout,
	}, httpapp.Dependencies{Logger: log, Auth: authService, Users: userService, Tokens: tokens, Products: productService, Stores: storeService})
	jobs, err := newScheduler(ctx, log, cfg.Scheduler, db, oneCService)
	if err != nil {
		return nil, errors.Join(err, db.Stop())
	}
	return &App{HTTPServer: server, storage: db, scheduler: jobs}, nil
}

func (a *App) Stop() error {
	httpErr := a.HTTPServer.Stop()
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	storageErr := a.storage.Stop()
	return errors.Join(httpErr, storageErr)
}

// Run supervises HTTP and releases resources on a signal or a listener failure.
func (a *App) Run(ctx context.Context) error {
	if a.scheduler != nil {
		a.scheduler.Start()
	}
	result := make(chan error, 1)
	go func() { result <- a.HTTPServer.Run() }()
	select {
	case err := <-result:
		return errors.Join(err, a.Stop())
	case <-ctx.Done():
		stopErr := a.Stop()
		return errors.Join(stopErr, <-result)
	}
}
