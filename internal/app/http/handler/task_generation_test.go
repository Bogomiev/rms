package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"rms/internal/services"
)

type taskGenerationStub struct {
	taskActorStub
	id  int64
	now time.Time
	err error
}

func (s *taskGenerationStub) GenerateType(_ context.Context, id int64, now time.Time) error {
	s.id, s.now = id, now
	return s.err
}
func TestTaskGenerateType(t *testing.T) {
	for _, tc := range []struct {
		id     string
		err    error
		status int
	}{
		{"42", nil, 200}, {"42", services.ErrNotFound, 404}, {"0", nil, 400}, {"new", nil, 400},
	} {
		t.Run(tc.id, func(t *testing.T) {
			stub := &taskGenerationStub{err: tc.err}
			h := NewHandler(nil, nil)
			h.ConfigureTasks(stub)
			h.ConfigureTaskTimezone("Asia/Vladivostok")
			r := chi.NewRouter()
			r.Post("/task_types/{id}/generate", h.TaskGenerateType)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("POST", "/task_types/"+tc.id+"/generate", nil))
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.id == "42" {
				if stub.id != 42 || stub.now.Location().String() != "Asia/Vladivostok" {
					t.Fatalf("generation arguments: %d %v", stub.id, stub.now)
				}
			} else if stub.id != 0 {
				t.Fatal("invalid ID reached service")
			}
		})
	}
}
