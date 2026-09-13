package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type cleanupStub struct {
	started  chan int
	canceled chan struct{}
}

func (s cleanupStub) DeleteExpiredSessions(ctx context.Context, limit int) (int64, error) {
	s.started <- limit
	<-ctx.Done()
	close(s.canceled)
	return 0, ctx.Err()
}
func TestCleanupStopsInFlightBatch(t *testing.T) {
	db := cleanupStub{make(chan int, 1), make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = sessionCleanupJob(slog.New(slog.NewTextHandler(io.Discard, nil)), db)(ctx)
	}()
	defer cancel()
	select {
	case limit := <-db.started:
		if limit != 1000 {
			t.Fatal(limit)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not stop")
	}
	select {
	case <-db.canceled:
	default:
		t.Fatal("database context not canceled")
	}
}
