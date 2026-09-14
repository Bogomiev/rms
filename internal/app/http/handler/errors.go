package handler

import (
	"errors"
	"github.com/go-chi/render"
	"net/http"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
	"time"
)

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var blocked *services.LoginBlockedError
	if errors.As(err, &blocked) {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, struct {
			resp.Response
			BlockedUntil    string `json:"blockedUntil"`
			ResultCodeSnake int    `json:"result_code"`
		}{resp.Error(services.ResultLoginBlocked, []string{blocked.Error()}), blocked.Until.Format(time.RFC3339), services.ResultLoginBlocked})
		return
	}
	code := 1
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, services.ErrInvalidCredentials):
		status, message = http.StatusUnauthorized, "invalid credentials"
	case errors.Is(err, services.ErrInvalidSession):
		status, message = http.StatusUnauthorized, "invalid session"
		code = services.ResultSessionInvalid
	case errors.Is(err, services.ErrForbidden):
		status, message = http.StatusForbidden, "insufficient permissions"
	case errors.Is(err, services.ErrUserExists):
		status, message = http.StatusConflict, "user already exists"
	}
	resp.HttpResponseError(w, r, status, code, []string{message})
}
