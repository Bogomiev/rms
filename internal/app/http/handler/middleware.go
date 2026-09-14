package handler

import (
	"context"
	"fmt"
	"net/http"

	"rms/internal/services"
	"rms/internal/token"
)

type authKey struct{}

func GetAuthMiddlewareFunc(tokenMaker TokenVerifier, sessions ...SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := verifyClaimsFromCookie(r, tokenMaker)
			if err != nil {
				writeServiceError(w, r, services.ErrInvalidSession)
				return
			}

			if !validCSRF(w, r, claims) {
				return
			}
			for _, validator := range sessions {
				if err := validator.ValidateAccess(r.Context(), claims); err != nil {
					writeServiceError(w, r, err)
					return
				}
			}
			ctx := context.WithValue(r.Context(), authKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAdminMiddlewareFunc(tokenMaker TokenVerifier, sessions ...SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			claims, err := verifyClaimsFromCookie(r, tokenMaker)
			if err != nil {
				writeServiceError(w, r, services.ErrInvalidSession)
				return
			}

			if !claims.IsAdmin {
				writeServiceError(w, r, services.ErrForbidden)
				return
			}

			if !validCSRF(w, r, claims) {
				return
			}
			for _, validator := range sessions {
				if err := validator.ValidateAccess(r.Context(), claims); err != nil {
					writeServiceError(w, r, err)
					return
				}
			}
			ctx := context.WithValue(r.Context(), authKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func verifyClaimsFromCookie(r *http.Request, tokenMaker TokenVerifier, sessions ...SessionValidator) (*token.UserClaims, error) {
	cookie, err := r.Cookie("access_token")
	if err != nil || cookie.Value == "" {
		return nil, fmt.Errorf("access cookie is missing")
	}
	claims, err := tokenMaker.VerifyToken(cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if claims == nil || claims.Purpose != token.Access {
		return nil, fmt.Errorf("access token required")
	}

	return claims, nil
}
