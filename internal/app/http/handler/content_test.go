package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http/httptest"
	"rms/internal/domain/models"
	"strings"
	"testing"
)

type contentFileStub struct{ fileStub }

func (s *contentFileStub) Content(context.Context, string) (models.File, io.ReadCloser, error) {
	return models.File{Name: "Инструкция.html"}, io.NopCloser(strings.NewReader("<h1>Test</h1>")), nil
}
func TestFileContentHeaders(t *testing.T) {
	h := NewHandler(nil, nil)
	h.ConfigureFiles(&contentFileStub{})
	r := chi.NewRouter()
	r.Get("/files/{id}/content", h.FileContent)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/files/id/content", nil))
	if w.Code != 200 || w.Body.String() != "<h1>Test</h1>" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Header())
	}
}
