package app

import (
	"context"
	"log/slog"
	"rms/internal/scheduler"
	"rms/internal/services/task"
	"time"
)

type storeSyncer interface {
	SyncStores(context.Context) error
	SyncGoods(context.Context) error
	SyncPrices(context.Context) error
	SyncStocks(context.Context) error
}

func newScheduler(ctx context.Context, log *slog.Logger, cfg scheduler.Config, db sessionCleaner, oneC storeSyncer) (*scheduler.Scheduler, error) {
	registry := map[string]scheduler.Job{
		"session_cleanup":   sessionCleanupJob(log, db),
		"store_sync":        oneC.SyncStores,
		"goods_sync":        oneC.SyncGoods,
		"prices_sync":       oneC.SyncPrices,
		"stocks_sync_close": oneC.SyncStocks,
		"stocks_sync":       oneC.SyncStocks,
	}
	if storage, ok := db.(task.Storage); ok {
		location, e := time.LoadLocation(cfg.Timezone)
		if e != nil {
			return nil, e
		}
		registry["task_generation"] = func(ctx context.Context) error { return task.New(storage).Generate(ctx, time.Now().In(location)) }
		jobs := make(map[string]scheduler.JobConfig, len(cfg.Jobs)+1)
		for name, job := range cfg.Jobs {
			jobs[name] = job
		}
		cfg.Jobs = jobs
		if _, exists := jobs["task_generation"]; !exists {
			jobs["task_generation"] = scheduler.JobConfig{Enabled: true, Schedule: "0 7-21 * * *", Timeout: 5 * time.Minute}
		}
	}
	return scheduler.New(ctx, log, cfg, registry)
}
