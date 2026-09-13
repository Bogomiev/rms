package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"rms/internal/config"
	"rms/internal/lib/password"
	"rms/internal/lib/validation"
	"rms/internal/storage/postgres"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("initialization failed", slog.Any("error", err))
		os.Exit(1)
	}
}
func run() error {
	userToken := flag.String("user_token", "", "initial administrator user_token")
	name := flag.String("name", "Administrator", "initial administrator name")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	pass := os.Getenv("RMS_ADMIN_PASSWORD")
	if messages := validation.NewUser(*userToken, *name, pass); len(messages) > 0 {
		return fmt.Errorf("invalid administrator: %s", strings.Join(messages, "; "))
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	hash, err := password.HashPassword(pass)
	if err != nil {
		return err
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, time.Minute)
	defer cancel()
	db, err := postgres.New(ctx, cfg.Db, cfg.FirstAdminPwd)
	if err != nil {
		return err
	}
	defer db.Stop()
	if err = db.Bootstrap(ctx, *userToken, *name, hash); err != nil {
		return err
	}
	slog.Info("administrator initialized; existing credentials preserved")
	return nil
}
