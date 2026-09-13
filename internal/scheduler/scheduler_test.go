package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func testLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
func testConfig() Config {
	return Config{Timezone: "UTC", Jobs: map[string]JobConfig{"test": {Enabled: true, Schedule: "@every 1s", Timeout: time.Minute}}}
}

func TestSchedulerRunsSkipsOverlapAndStops(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	var calls atomic.Int32
	s, err := New(context.Background(), testLog(), testConfig(), map[string]Job{"test": func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	}})
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	defer s.Stop()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("job did not start")
	}
	// A second invocation must be skipped while the scheduled one is running.
	s.cron.Entries()[0].WrappedJob.Run()
	if calls.Load() != 1 {
		t.Fatal("overlapping job ran")
	}
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel and wait for the job")
	}
	select {
	case <-finished:
	default:
		t.Fatal("stop returned before job finished")
	}
}

func TestTimeoutAndPanicRecovery(t *testing.T) {
	cfg := testConfig()
	cfg.Jobs["test"] = JobConfig{Enabled: true, Schedule: "@every 1m", Timeout: 10 * time.Millisecond}
	var calls int
	s, err := New(context.Background(), testLog(), cfg, map[string]Job{"test": func(ctx context.Context) error {
		calls++
		if calls == 1 {
			panic("test panic")
		}
		<-ctx.Done()
		if ctx.Err() != context.DeadlineExceeded {
			t.Error(ctx.Err())
		}
		return ctx.Err()
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	entry := s.cron.Entries()[0]
	entry.WrappedJob.Run()
	entry.WrappedJob.Run()
	if calls != 2 {
		t.Fatal("panic prevented subsequent invocation")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.Timezone = "Invalid/Timezone" },
		func(c *Config) { c.Jobs["test"] = JobConfig{Enabled: true, Schedule: "bad", Timeout: time.Second} },
		func(c *Config) { c.Jobs["test"] = JobConfig{Enabled: true, Schedule: "* * * * *"} },
	} {
		cfg := testConfig()
		change(&cfg)
		if _, err := New(context.Background(), testLog(), cfg, map[string]Job{"test": func(context.Context) error { return nil }}); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	if _, err := New(context.Background(), testLog(), testConfig(), nil); err == nil {
		t.Fatal("unknown job accepted")
	}
	cfg := testConfig()
	cfg.Jobs["test"] = JobConfig{Enabled: false}
	s, err := New(context.Background(), testLog(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if len(s.cron.Entries()) != 0 {
		t.Fatal("disabled job scheduled")
	}
}
