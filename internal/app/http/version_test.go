package httpapp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVersionIsPublic(t *testing.T) {
	a := New(Config{}, Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	w := httptest.NewRecorder()
	a.hTTPServer.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/version", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body struct {
		ResultCode int      `json:"resultCode"`
		Messages   []string `json:"messages"`
		Data       struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ResultCode != 0 || body.Messages == nil || len(body.Messages) != 0 || body.Data.Version != "1.1.1.2" {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}
