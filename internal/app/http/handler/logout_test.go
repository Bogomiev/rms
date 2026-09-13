package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rms/internal/services/auth"
	"rms/internal/token"
)

func TestLogoutRejectsInvalidToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := auth.New(auth.Config{}, auth.Dependencies{Logger: logger, Tokens: token.NewJWTMaker("test-secret")})
	h := NewHandler(service, nil)
	request := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader(`{"refreshToken":"invalid-token"}`))
	request.AddCookie(&http.Cookie{Name: "refresh_token", Value: "invalid-token"})
	response := httptest.NewRecorder()
	h.Logout(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	decoder := json.NewDecoder(response.Body)
	var body struct {
		ResultCode int `json:"resultCode"`
	}
	if err := decoder.Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ResultCode == 0 {
		t.Fatal("logout error reported as success")
	}
	if err := decoder.Decode(&body); err != io.EOF {
		t.Fatalf("unexpected trailing response: %v", err)
	}
}
