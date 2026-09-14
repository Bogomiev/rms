package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"rms/internal/services"
	"rms/internal/token"
	"testing"
	"time"
)

func TestCSRFAndOrigin(t *testing.T) {
	maker := token.NewJWTMaker("key")
	access, claims, err := maker.CreateToken(1, "user", true, time.Minute, token.Access, "session")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for _, admin := range []bool{false, true} {
		for _, tc := range []struct {
			method, csrf string
			status       int
		}{
			{"GET", "", 204}, {"POST", "", 403}, {"POST", "wrong", 403}, {"POST", claims.CSRFToken, 204}, {"PUT", claims.CSRFToken, 204}, {"PATCH", "", 403}, {"DELETE", "", 403},
		} {
			t.Run(fmt.Sprintf("%v/%s/%s", admin, tc.method, tc.csrf), func(t *testing.T) {
				req := httptest.NewRequest(tc.method, "/", nil)
				req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
				req.Header.Set("X-CSRF-Token", tc.csrf)
				w := httptest.NewRecorder()
				middleware := GetAuthMiddlewareFunc(maker)
				if admin {
					middleware = GetAdminMiddlewareFunc(maker)
				}
				middleware(next).ServeHTTP(w, req)
				if w.Code != tc.status {
					t.Fatal(w.Code, w.Body.String())
				}
			})
		}
	}
	for _, origin := range []string{"", "null", "https://evil.test", "https://app.test.evil.test", "https://app.test"} {
		req := httptest.NewRequest("POST", "/auth/refresh", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		OriginMiddleware("https://app.test")(next).ServeHTTP(w, req)
		if (w.Code == 204) != (origin == "https://app.test") {
			t.Fatal(origin, w.Code)
		}
	}
	// Persisted revocation is checked even for a correctly signed access token.
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	w := httptest.NewRecorder()
	GetAuthMiddlewareFunc(maker, serviceStub{call: func(context.Context) error { return services.ErrInvalidSession }})(next).ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestLockoutResponse(t *testing.T) {
	until := time.Date(2026, 9, 14, 3, 15, 0, 0, time.UTC)
	w := httptest.NewRecorder()
	writeServiceError(w, httptest.NewRequest("POST", "/auth/login", nil), fmt.Errorf("wrapped: %w", &services.LoginBlockedError{Until: until}))
	var body struct {
		ResultCode   int      `json:"resultCode"`
		Messages     []string `json:"messages"`
		BlockedUntil string   `json:"blockedUntil"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 401 || body.ResultCode != 1001 || len(body.Messages) != 1 || body.BlockedUntil != until.Format(time.RFC3339) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestOriginAllowlist(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	middleware := OriginMiddleware("https://one.test", "https://two.test:8443")
	for _, tc := range []struct {
		origin string
		status int
	}{
		{"https://one.test", 204}, {"https://two.test:8443", 204},
		{"", 403}, {"null", 403}, {"https://two.test", 403},
		{"http://one.test", 403}, {"https://one.test.evil.test", 403},
		{"https://one.test/", 403}, {"https://one.test, https://two.test:8443", 403},
	} {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		middleware(next).ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%q: %d", tc.origin, w.Code)
		}
	}
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.Header.Add("Origin", "https://one.test")
	req.Header.Add("Origin", "https://two.test:8443")
	w := httptest.NewRecorder()
	middleware(next).ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("accepted duplicate Origin headers")
	}
	w = httptest.NewRecorder()
	OriginMiddleware()(next).ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("empty allowlist accepted request")
	}
}
