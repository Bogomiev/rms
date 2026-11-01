package httpapp

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTaskResourcesRequireAuthentication(t *testing.T) {
	a := New(Config{}, Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, kind := range []string{"task_types", "task_scheduling", "tasks", "task_status_history"} {
		for _, tc := range []struct{ method, suffix string }{{"GET", "/"}, {"GET", "/1"}, {"POST", "/"}, {"PUT", "/1"}, {"DELETE", "/1"}} {
			w := httptest.NewRecorder()
			a.hTTPServer.Handler.ServeHTTP(w, httptest.NewRequest(tc.method, "/"+kind+tc.suffix, strings.NewReader("{}")))
			if w.Code != 401 {
				t.Errorf("%s %s: %d", tc.method, kind, w.Code)
			}
		}
	}
}
