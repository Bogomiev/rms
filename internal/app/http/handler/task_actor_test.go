package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"rms/internal/domain/models"
	"rms/internal/token"
	"strings"
	"testing"
)

type taskActorStub struct{ userID int64 }

func (s *taskActorStub) List(context.Context, string, models.TaskFilter) ([]json.RawMessage, error) {
	return nil, nil
}
func (s *taskActorStub) ByID(context.Context, string, int64) (json.RawMessage, error) {
	return nil, nil
}
func (s *taskActorStub) Delete(context.Context, string, int64) error { return nil }
func (s *taskActorStub) Write(_ context.Context, _ string, _ int64, _ json.RawMessage, userID int64) (json.RawMessage, error) {
	s.userID = userID
	return json.RawMessage(`{"id":1}`), nil
}
func TestTaskWriteUsesAuthenticatedActor(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		stub := &taskActorStub{}
		h := NewHandler(nil, nil)
		h.ConfigureTasks(stub)
		req := httptest.NewRequest("POST", "/tasks/", strings.NewReader(`{"user_id":999}`))
		if authenticated {
			req = req.WithContext(context.WithValue(req.Context(), authKey{}, &token.UserClaims{UserID: 42}))
		}
		w := httptest.NewRecorder()
		h.TaskResource("tasks")(w, req)
		if authenticated {
			if w.Code != 201 || stub.userID != 42 {
				t.Fatalf("actor: %d %d %s", w.Code, stub.userID, w.Body.String())
			}
		} else if w.Code != 401 || stub.userID != 0 {
			t.Fatalf("anonymous write: %d %d", w.Code, stub.userID)
		}
	}
}
