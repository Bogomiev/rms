package handler

import (
	"context"
	"crypto/subtle"
	"github.com/go-chi/render"
	"net/http"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
	"rms/internal/token"
)

type SessionValidator interface {
	ValidateAccess(context.Context, *token.UserClaims) error
}

// Origin is checked against configuration, never against client-controlled Host headers.
func OriginMiddleware(origins ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := allowed[r.Header.Get("Origin")]; !ok || len(r.Header.Values("Origin")) != 1 {
				resp.HttpResponseError(w, r, http.StatusForbidden, services.ResultCSRFInvalid, []string{"Недопустимый источник запроса"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func validCSRF(w http.ResponseWriter, r *http.Request, claims *token.UserClaims) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	csrf := r.Header.Get("X-CSRF-Token")
	if claims.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(claims.CSRFToken)) != 1 {
		resp.HttpResponseError(w, r, http.StatusForbidden, services.ResultCSRFInvalid, []string{"Недействительный CSRF-токен"})
		return false
	}
	return true
}

func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(authKey{}).(*token.UserClaims)
	w.Header().Set("Cache-Control", "no-store")
	render.JSON(w, r, struct {
		resp.Response
		CSRFToken string `json:"csrfToken"`
		UserToken string `json:"userToken"`
	}{resp.OK(), claims.CSRFToken, claims.UserToken})
}

func (h *Handler) ConfigureSecurity(tokens TokenVerifier) { h.tokens = tokens }
