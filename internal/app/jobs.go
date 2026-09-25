package app

import (
	"context"
	"log/slog"
	"rms/internal/scheduler"
)

type storeSyncer interface {
	SyncStores(context.Context) error
	SyncGoods(context.Context) error
}

func newScheduler(ctx context.Context, log *slog.Logger, cfg scheduler.Config, db sessionCleaner, oneC storeSyncer) (*scheduler.Scheduler, error) {
	return scheduler.New(ctx, log, cfg, map[string]scheduler.Job{
		"session_cleanup": sessionCleanupJob(log, db),
		"store_sync":      oneC.SyncStores,
		"goods_sync":      oneC.SyncGoods,
	})
}
