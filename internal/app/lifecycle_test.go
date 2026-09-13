package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	httpapp "rms/internal/app/http"
	"strconv"
	"testing"
	"time"
)

type lifecycleStorage struct {
	closed bool
	err    error
}

func (s *lifecycleStorage) Stop() error { s.closed = true; return s.err }
func TestListenerFailureClosesResources(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	server := httpapp.New(httpapp.Config{Port: port}, httpapp.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	sentinel := errors.New("close failure")
	db := &lifecycleStorage{err: sentinel}
	a := &App{HTTPServer: server, storage: db}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = a.Run(ctx)
	if !errors.Is(err, sentinel) {
		t.Fatalf("shutdown error lost: %v", err)
	}
	var networkError *net.OpError
	if !errors.As(err, &networkError) {
		t.Fatalf("listener error lost: %v", err)
	}
	if !db.closed {
		t.Fatal("database not closed")
	}
}
