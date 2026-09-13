package app

import (
	"context"
	"log/slog"
	"rms/internal/scheduler"
)

type sessionCleaner interface {
	DeleteExpiredSessions(context.Context, int) (int64, error)
}

func sessionCleanupJob(log *slog.Logger, db sessionCleaner) scheduler.Job {
	return func(ctx context.Context) error {
		count, err := db.DeleteExpiredSessions(ctx, 1000)
		if err != nil {
			return err
		}
		if count > 0 {
			log.Info("expired sessions removed", "count", count)
		}
		return nil
	}
}
