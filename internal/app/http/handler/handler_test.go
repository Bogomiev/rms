package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/token"
)

type serviceStub struct{ call func(context.Context) error }

func (s serviceStub) Login(ctx context.Context, _, _ string) (string, string, string, *models.User, error) {
	return "session", "access", "refresh", &models.User{}, s.call(ctx)
}
func (s serviceStub) Logout(ctx context.Context, _ string) error { return s.call(ctx) }
func (s serviceStub) RefreshToken(ctx context.Context, _ string) (string, string, string, error) {
	return "session", "access", "refresh", s.call(ctx)
}
func (s serviceStub) RegisterNewUser(ctx context.Context, _, _, _ string, _ bool) (int64, error) {
	return 1, s.call(ctx)
}
func (s serviceStub) UserList(ctx context.Context, page models.UserPage) ([]*models.User, error) {
	return nil, s.call(ctx)
}

func TestHandlersForwardContextAndMapErrors(t *testing.T) {
	routes := []struct {
		name, body string
		serve      func(*Handler) http.HandlerFunc
	}{
		{"login", `{"user_token":"a@example.com","password":"password"}`, func(h *Handler) http.HandlerFunc { return h.Login }},
		{"logout", `{"refreshToken":"token"}`, func(h *Handler) http.HandlerFunc { return h.Logout }},
		{"refresh", `{"refreshToken":"token"}`, func(h *Handler) http.HandlerFunc { return h.RefreshToken }},
		{"create", `{"user_token":"a@example.com","name":"A","password":"password"}`, func(h *Handler) http.HandlerFunc { return h.CreateUser }},
		{"list", "", func(h *Handler) http.HandlerFunc { return h.UserList }},
	}
	failures := []struct {
		name   string
		err    error
		status int
	}{
		{"credentials", services.ErrInvalidCredentials, 401},
		{"session", services.ErrInvalidSession, 401},
		{"forbidden", services.ErrForbidden, 403},
		{"duplicate", services.ErrUserExists, 409},
		{"database", errors.New("private database details"), 500},
		{"canceled", context.Canceled, 500},
	}
	for _, route := range routes {
		for _, failure := range failures {
			t.Run(route.name+"/"+failure.name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				called := false
				stub := serviceStub{call: func(received context.Context) error {
					called = true
					if received != ctx {
						t.Fatal("request context was replaced")
					}
					cancel()
					if !errors.Is(received.Err(), context.Canceled) {
						t.Fatal("cancellation did not propagate")
					}
					return fmt.Errorf("private service details: %w", failure.err)
				}}
				request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(route.body)).WithContext(ctx)
				request.AddCookie(&http.Cookie{Name: "refresh_token", Value: "token"})
				response := httptest.NewRecorder()
				route.serve(NewHandler(stub, stub))(response, request)
				if !called {
					t.Fatal("service not called")
				}
				if response.Code != failure.status {
					t.Fatalf("status = %d, want %d", response.Code, failure.status)
				}
				if strings.Contains(response.Body.String(), "private") {
					t.Fatal("internal error leaked into response")
				}
			})
		}
	}
}

type verifierStub struct {
	claims *token.UserClaims
	err    error
}

func (v verifierStub) VerifyToken(string) (*token.UserClaims, error) { return v.claims, v.err }
func TestAdminMiddlewareErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		verifier verifierStub
		status   int
	}{
		{"invalid", verifierStub{err: errors.New("private signing details")}, 401},
		{"non-admin", verifierStub{claims: &token.UserClaims{Purpose: token.Access}}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthorized request reached handler") })
			request := httptest.NewRequest(http.MethodPost, "/users", nil)
			request.AddCookie(&http.Cookie{Name: "access_token", Value: "test"})
			request.AddCookie(&http.Cookie{Name: "refresh_token", Value: "token"})
			response := httptest.NewRecorder()
			GetAdminMiddlewareFunc(tc.verifier)(next).ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if strings.Contains(response.Body.String(), "private") {
				t.Fatal("internal error leaked")
			}
		})
	}
}

func (s serviceStub) UserTokenValid(ctx context.Context, _ string) (bool, error) {
	return true, s.call(ctx)
}

func (s serviceStub) ValidateAccess(ctx context.Context, _ *token.UserClaims) error {
	return s.call(ctx)
}
