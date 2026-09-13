package handler

import (
	"errors"
	"net/http"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
)

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, services.ErrInvalidCredentials):
		status, message = http.StatusUnauthorized, "invalid credentials"
	case errors.Is(err, services.ErrInvalidSession):
		status, message = http.StatusUnauthorized, "invalid session"
	case errors.Is(err, services.ErrForbidden):
		status, message = http.StatusForbidden, "insufficient permissions"
	case errors.Is(err, services.ErrUserExists):
		status, message = http.StatusConflict, "user already exists"
	}
	resp.HttpResponseError(w, r, status, 1, []string{message})
}
