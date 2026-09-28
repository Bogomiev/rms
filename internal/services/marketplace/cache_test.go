package marketplace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"rms/internal/services/onec"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheConcurrentRefreshExpiryAndIsolation(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		w.Write([]byte(`{"code":0,"mess":"","data":{"Eshop":{"name":"original"}}}`))
	}))
	defer server.Close()
	client, _ := onec.New(onec.Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
	s := New(client)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.MarketplaceData(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.MarketplaceData(ctx); err != context.Canceled {
		t.Errorf("cancel: %v", err)
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("refresh calls: %d", calls.Load())
	}
	got, err := s.MarketplaceData(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got.Data["Eshop"][0] = '!'
	delete(got.Data, "Eshop")
	got, err = s.MarketplaceData(context.Background())
	if err != nil || string(got.Data["Eshop"]) != `{"name":"original"}` {
		t.Fatalf("cache mutated: %+v %v", got, err)
	}
	s.mu.Lock()
	s.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if _, err = s.MarketplaceData(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expired refresh calls: %d", calls.Load())
	}
}
func TestCacheDoesNotStoreErrors(t *testing.T) {
	for _, body := range []string{`{"code":1,"mess":"failed"}`, `broken`} {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(body)) }))
		client, _ := onec.New(onec.Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
		s := New(client)
		for i := 0; i < 2; i++ {
			s.MarketplaceData(context.Background())
		}
		server.Close()
		if calls != 2 {
			t.Fatalf("cached error %s", body)
		}
	}
}
