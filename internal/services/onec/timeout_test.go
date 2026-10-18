package onec

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Headers arrive immediately; the timeout must also cover the response body.
func TestSharedBodyTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-time.After(150 * time.Millisecond):
			io.WriteString(w, "{}")
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	svc, err := New(Config{BaseURL: server.URL, Timeout: 50 * time.Millisecond}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path    string
		success bool
	}{
		{"v1/GetPrices?page=1", false},
		{"v1/GetStores", false},
	} {
		resp, err := svc.Do(context.Background(), http.MethodGet, tc.path, nil)
		if err == nil {
			_, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if (err == nil) != tc.success {
			t.Fatalf("%s: %v", tc.path, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	resp, err := svc.Do(ctx, http.MethodGet, "v1/GetPrices?page=1", nil)
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("request ignored caller deadline")
	}
}
