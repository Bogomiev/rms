package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"rms/internal/token"
	"strings"
	"testing"
	"time"
)

func TestAuthenticationCookies(t *testing.T) {
	stub := serviceStub{call: func(context.Context) error { return nil }}
	h := NewHandler(stub, stub)
	h.Configure(nil, nil, 7*time.Minute, 2160*time.Hour)
	for _, action := range []string{"login", "refresh", "logout"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/auth/"+action, strings.NewReader(`{"user_token":"test-token","password":"password"}`))
			if action != "login" {
				req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "refresh"})
			}
			w := httptest.NewRecorder()
			switch action {
			case "login":
				h.Login(w, req)
			case "refresh":
				h.RefreshToken(w, req)
			case "logout":
				h.Logout(w, req)
			}
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), `"access"`) || strings.Contains(w.Body.String(), `"refresh"`) {
				t.Fatal("tokens leaked into JSON")
			}
			cookies := w.Result().Cookies()
			if len(cookies) != 2 {
				t.Fatal(cookies)
			}
			for _, c := range cookies {
				mode := http.SameSiteLaxMode
				ttl := 420
				value := "access"
				if c.Name == "refresh_token" {
					mode = http.SameSiteStrictMode
					ttl = 7776000
					value = "refresh"
				} else if c.Name != "access_token" {
					t.Fatal(c.Name)
				}
				if !c.HttpOnly || !c.Secure || c.SameSite != mode || c.Path != "/" || c.Domain != "" {
					t.Fatalf("cookie flags: %+v", c)
				}
				if action == "logout" {
					if c.MaxAge != -1 || c.Value != "" || !c.Expires.Before(time.Now()) {
						t.Fatalf("cookie not deleted: %+v", c)
					}
				} else {
					if c.MaxAge != ttl || c.Value != value || time.Until(c.Expires) < time.Duration(ttl-2)*time.Second {
						t.Fatalf("cookie value/TTL: %+v", c)
					}
				}
			}
		})
	}
}
func TestTokensMustComeFromCookies(t *testing.T) {
	stub := serviceStub{call: func(context.Context) error { t.Fatal("body token accepted"); return nil }}
	h := NewHandler(stub, stub)
	for _, serve := range []http.HandlerFunc{h.RefreshToken, h.Logout} {
		w := httptest.NewRecorder()
		serve(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"refreshToken":"refresh"}`)))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	for _, tc := range []struct {
		cookie, header string
		purpose        string
		status         int
	}{
		{"", "Bearer access", token.Access, 401}, {"refresh", "", token.Refresh, 401}, {"access", "", token.Access, 204},
	} {
		req := httptest.NewRequest("GET", "/products", nil)
		req.Header.Set("Authorization", tc.header)
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: "access_token", Value: tc.cookie})
		}
		w := httptest.NewRecorder()
		GetAuthMiddlewareFunc(verifierStub{claims: &token.UserClaims{Purpose: tc.purpose}})(next).ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatal(w.Code, tc)
		}
	}
}
