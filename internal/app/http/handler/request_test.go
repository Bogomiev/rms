package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"rms/internal/domain/models"
	"strings"
	"testing"
)

func TestRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"user_token", `{"user_token":" ","name":"User","password":"password"}`, 400},
		{"short password", `{"user_token":"a@example.com","name":"User","password":"short"}`, 400},
		{"long password", `{"user_token":"a@example.com","name":"User","password":"` + strings.Repeat("x", 73) + `"}`, 400},
		{"blank name", `{"user_token":"a@example.com","name":" ","password":"password"}`, 400},
		{"long name", `{"user_token":"a@example.com","name":"` + strings.Repeat("x", 256) + `","password":"password"}`, 400},
		{"unknown field", `{"unexpected":true}`, 400},
		{"trailing JSON", `{} {}`, 400},
		{"oversized", `{"name":"` + strings.Repeat("x", maxRequestBytes) + `"}`, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := serviceStub{call: func(context.Context) error { t.Fatal("invalid request reached service"); return nil }}
			h := NewHandler(stub, stub)
			w := httptest.NewRecorder()
			h.CreateUser(w, httptest.NewRequest("POST", "/users/", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("status=%d, want %d", w.Code, tc.status)
			}
		})
	}
}

type pagedStub struct {
	serviceStub
	page models.UserPage
}

func (s *pagedStub) UserList(_ context.Context, p models.UserPage) ([]*models.User, error) {
	s.page = p
	return nil, nil
}
func TestPagination(t *testing.T) {
	for _, tc := range []struct {
		query                 string
		status, limit, offset int
	}{
		{"", 200, 50, 0}, {"?limit=10&offset=20", 200, 10, 20}, {"?limit=0", 400, 0, 0},
		{"?limit=101", 400, 0, 0}, {"?offset=-1", 400, 0, 0}, {"?limit=no", 400, 0, 0}, {"?limit=1&limit=2", 400, 0, 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			users := &pagedStub{}
			h := NewHandler(nil, users)
			w := httptest.NewRecorder()
			h.UserList(w, httptest.NewRequest(http.MethodGet, "/users/"+tc.query, nil))
			if w.Code != tc.status {
				t.Fatal(w.Code)
			}
			if users.page.Limit != tc.limit || users.page.Offset != tc.offset {
				t.Fatalf("page=%+v", users.page)
			}
		})
	}
}
