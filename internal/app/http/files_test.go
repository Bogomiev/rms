package httpapp

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilesRequireAuthentication(t *testing.T) {
	a := New(Config{}, Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, tc := range []struct{ method, path string }{{"GET", "/files/"}, {"GET", "/files/by-path?path=a"}, {"GET", "/files/id"}, {"POST", "/files/"}, {"PUT", "/files/id"}, {"DELETE", "/files/id"}} {
		w := httptest.NewRecorder()
		a.hTTPServer.Handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))
		if w.Code != 401 {
			t.Errorf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}
