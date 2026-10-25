package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"rms/internal/domain/models"
	"rms/internal/services/files"
	"strings"
	"testing"
)

type fileStub struct {
	in     files.Input
	update bool
	marked bool
	calls  int
	err    error
}

func (s *fileStub) List(context.Context, *string, bool) ([]models.File, error) {
	return []models.File{}, s.err
}
func (s *fileStub) Get(context.Context, string, string) (models.File, error) {
	return models.File{}, s.err
}
func (s *fileStub) Write(_ context.Context, in files.Input, update bool) (models.File, error) {
	s.calls++
	s.in = in
	s.update = update
	return models.File{ID: in.ID}, s.err
}
func (s *fileStub) Delete(_ context.Context, _ string, marked ...bool) error {
	s.calls++
	s.marked = marked[0]
	return s.err
}
func TestFileAPI(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
		marked             bool
	}{
		{"POST", "/files/", `{"id":"abc","name":"a","is_file":true,"content":"aGVsbG8="}`, 201, false},
		{"PUT", "/files/abc", `{"name":"a","is_file":true}`, 200, false},
		{"PUT", "/files/abc", `{"id":"other","name":"a"}`, 400, false},
		{"POST", "/files/", `{"unknown":true}`, 400, false},
		{"POST", "/files/", `{} {}`, 400, false},
		{"DELETE", "/files/abc", "", 204, true},
		{"DELETE", "/files/abc?marked=false", "", 204, false},
		{"DELETE", "/files/abc?marked=nope", "", 400, false},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			s := &fileStub{}
			h := NewHandler(nil, nil)
			h.ConfigureFiles(s)
			r := chi.NewRouter()
			r.Post("/files/", h.CreateFile)
			r.Put("/files/{id}", h.UpdateFile)
			r.Delete("/files/{id}", h.DeleteFile)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.status == 400 && s.calls != 0 {
				t.Fatal("invalid input reached service")
			}
			if tc.method == "DELETE" && tc.status == 204 && s.marked != tc.marked {
				t.Fatal("incorrect delete mode")
			}
			if tc.method == "PUT" && tc.status == 200 && (!s.update || s.in.ID != "abc") {
				t.Fatal("update id missing")
			}
			if tc.method == "POST" && tc.status == 201 && string(s.in.Content) != "hello" {
				t.Fatal("content not decoded")
			}
		})
	}
}
