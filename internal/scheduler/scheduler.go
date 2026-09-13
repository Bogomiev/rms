// Package scheduler registers named, context-aware jobs and runs configured schedules.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

type Job func(context.Context) error

type JobConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Schedule string        `yaml:"schedule"`
	Timeout  time.Duration `yaml:"timeout"`
}

type Config struct {
	Timezone string               `yaml:"timezone" env-default:"UTC"`
	Jobs     map[string]JobConfig `yaml:"jobs"`
}

func (c Config) Validate() error {
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("scheduler timezone: %w", err)
	}
	for name, job := range c.Jobs {
		if !job.Enabled {
			continue
		}
		if name == "" || job.Timeout <= 0 {
			return fmt.Errorf("scheduler job %q requires a name and positive timeout", name)
		}
		if _, err := cron.ParseStandard(job.Schedule); err != nil {
			return fmt.Errorf("scheduler job %q: %w", name, err)
		}
	}
	return nil
}

type Scheduler struct {
	cron   *cron.Cron
	cancel context.CancelFunc
}

// New validates all enabled jobs before starting any goroutines.
// Register new implementations in registry and configure their names in YAML.
func New(parent context.Context, log *slog.Logger, cfg Config, registry map[string]Job) (*Scheduler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	for name, c := range cfg.Jobs {
		if c.Enabled && registry[name] == nil {
			return nil, fmt.Errorf("scheduler job %q is not registered", name)
		}
	}
	location, _ := time.LoadLocation(cfg.Timezone)
	ctx, cancel := context.WithCancel(parent)
	adapter := cronLogger{log}
	// Recover inside SkipIfStillRunning ensures a panic releases the overlap guard.
	runner := cron.New(cron.WithLocation(location), cron.WithLogger(adapter), cron.WithChain(cron.SkipIfStillRunning(adapter), cron.Recover(adapter)))
	for name, c := range cfg.Jobs {
		if !c.Enabled {
			continue
		}
		job := registry[name]
		_, err := runner.AddFunc(c.Schedule, func() {
			if ctx.Err() != nil {
				return
			}
			runCtx, done := context.WithTimeout(ctx, c.Timeout)
			defer done()
			if err := job(runCtx); err != nil {
				log.Error("scheduled job failed", "job", name, "error", err)
			}
		})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("register job %q: %w", name, err)
		}
	}
	return &Scheduler{cron: runner, cancel: cancel}, nil
}

func (s *Scheduler) Start() { s.cron.Start() }

// Stop cancels job contexts and waits for running jobs before storage is closed.
// Job implementations must honor context cancellation.
func (s *Scheduler) Stop() { s.cancel(); <-s.cron.Stop().Done() }

type cronLogger struct{ log *slog.Logger }

func (l cronLogger) Info(msg string, keysAndValues ...interface{}) {
	l.log.Debug(msg, keysAndValues...)
}
func (l cronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.log.Error(msg, append(keysAndValues, "error", err)...)
}
