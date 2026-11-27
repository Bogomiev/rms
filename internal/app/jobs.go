package app

import (
	"context"
	"log/slog"
	"rms/internal/scheduler"
	"rms/internal/services/price"
	"rms/internal/services/task"
	"time"
)

type storeSyncer interface {
	price.TodayPriceChangesSource
	SyncStores(context.Context) error
	SyncGoods(context.Context) error
	SyncPrices(context.Context) error
	SyncStocks(context.Context) error
}

func newScheduler(ctx context.Context, log *slog.Logger, cfg scheduler.Config, db sessionCleaner, oneC storeSyncer, prices *price.Service, queueJob, returnStatusJob scheduler.Job) (*scheduler.Scheduler, error) {
	registry := map[string]scheduler.Job{
		"session_cleanup":   sessionCleanupJob(log, db),
		"store_sync":        oneC.SyncStores,
		"goods_sync":        oneC.SyncGoods,
		"prices_sync":       oneC.SyncPrices,
		"stocks_sync_close": oneC.SyncStocks,
		"stocks_sync":       oneC.SyncStocks,
	}
	registry["today_price_changes_sync"] = func(ctx context.Context) error {
		return prices.SyncTodayPriceChanges(ctx, oneC)
	}
	jobs := make(map[string]scheduler.JobConfig, len(cfg.Jobs)+2)
	for name, job := range cfg.Jobs {
		jobs[name] = job
	}
	cfg.Jobs = jobs
	registry["message_queue"] = queueJob
	registry["return_status_sync"] = returnStatusJob
	if _, exists := jobs["message_queue"]; !exists {
		jobs["message_queue"] = scheduler.JobConfig{Enabled: true, Schedule: "@every 5s", Timeout: 2 * time.Minute}
	}
	if _, exists := jobs["return_status_sync"]; !exists {
		jobs["return_status_sync"] = scheduler.JobConfig{Enabled: true, Schedule: "@every 1m", Timeout: time.Minute}
	}
	if _, exists := jobs["today_price_changes_sync"]; !exists {
		jobs["today_price_changes_sync"] = scheduler.JobConfig{Enabled: true, Schedule: "0 * * * *", Timeout: 5 * time.Minute}
	}
	if storage, ok := db.(task.Storage); ok {
		location, e := time.LoadLocation(cfg.Timezone)
		if e != nil {
			return nil, e
		}
		registry["task_generation"] = func(ctx context.Context) error { return task.New(storage).Generate(ctx, time.Now().In(location)) }
		if _, exists := jobs["task_generation"]; !exists {
			jobs["task_generation"] = scheduler.JobConfig{Enabled: true, Schedule: "0 7-21 * * *", Timeout: 5 * time.Minute}
		}
	}
	return scheduler.New(ctx, log, cfg, registry)
}
