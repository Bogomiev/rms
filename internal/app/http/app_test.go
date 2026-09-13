package httpapp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunAfterStopReturnsNormally(t *testing.T) {
	a := &App{
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		hTTPServer: &http.Server{},
		address:    "127.0.0.1:0",
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(); err != nil {
		t.Fatalf("normal shutdown returned error: %v", err)
	}
}

func TestStopDrainsActiveRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	defer close(release)
	a := &App{log: slog.New(slog.NewTextHandler(io.Discard, nil)), hTTPServer: server.Config}
	clientDone := make(chan error, 1)
	go func() {
		response, err := server.Client().Get(server.URL)
		if err == nil {
			response.Body.Close()
		}
		clientDone <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	shutdownStarted := make(chan struct{})
	server.Config.RegisterOnShutdown(func() { close(shutdownStarted) })
	stopped := make(chan error, 1)
	go func() { stopped <- a.Stop() }()
	select {
	case <-shutdownStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not start")
	}
	select {
	case err := <-stopped:
		t.Fatalf("shutdown returned before active request completed: %v", err)
	default:
	}
	release <- struct{}{}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not complete")
	}
	if err := <-clientDone; err != nil {
		t.Fatal(err)
	}
}
