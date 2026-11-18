package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
	"rms/internal/token"
)

// loginCodeService issues and spends one-time codes for signing in on a phone by a QR code.
type loginCodeService interface {
	IssueLoginCode(context.Context, int64, string) (string, time.Duration, error)
	LoginByCode(context.Context, string) (string, string, string, *models.User, string, error)
}

type loginCodeRequest struct {
	StoreID string `json:"store_id"`
}

type loginCodeRedeemRequest struct {
	Code string `json:"code"`
}

type loginCodeRedeemResponse struct {
	loginResponse
	StoreID   string `json:"store_id,omitempty"`
	StoreName string `json:"store_name,omitempty"`
}

// storeName finds the store name in RMS, so the phone shows it before the 1C store list arrives.
func (h *Handler) storeName(ctx context.Context, id string) string {
	if id == "" || h.storeService == nil {
		return ""
	}
	stores, err := h.storeService.Stores(ctx)
	if err != nil {
		return ""
	}
	for _, v := range stores.Items {
		if v.ID.String() == id {
			return v.Name
		}
	}
	return ""
}

// CreateLoginCode gives the signed-in user a one-time code for a QR login; store_id is the store the phone opens.
func (h *Handler) CreateLoginCode(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(authKey{}).(*token.UserClaims)
	if !ok || claims.UserID <= 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	codes, ok := h.authService.(loginCodeService)
	if !ok {
		writeServiceError(w, r, services.ErrNotFound)
		return
	}
	var in loginCodeRequest
	if !decodeRequest(w, r, &in) {
		return
	}
	if in.StoreID != "" {
		id, err := uuid.Parse(in.StoreID)
		if err != nil || id == uuid.Nil {
			writeServiceError(w, r, services.ErrInvalid)
			return
		}
		in.StoreID = id.String()
	}
	code, ttl, err := codes.IssueLoginCode(r.Context(), claims.UserID, in.StoreID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	render.Status(r, http.StatusCreated)
	resourceResponse(w, r, struct {
		Code      string `json:"code"`
		ExpiresIn int    `json:"expires_in"`
	}{code, int(ttl / time.Second)})
}

// LoginByCode signs in with a one-time QR code and sets session cookies like Login.
func (h *Handler) LoginByCode(w http.ResponseWriter, r *http.Request) {
	codes, ok := h.authService.(loginCodeService)
	if !ok {
		writeServiceError(w, r, services.ErrNotFound)
		return
	}
	var in loginCodeRedeemRequest
	if !decodeRequest(w, r, &in) {
		return
	}
	if in.Code == "" || len(in.Code) > 255 {
		resp.HttpResponseError(w, r, http.StatusBadRequest, 1, []string{"code is required"})
		return
	}
	sessionID, accessToken, refreshToken, user, store, err := codes.LoginByCode(r.Context(), in.Code)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	claims, err := h.tokens.VerifyToken(accessToken)
	if err != nil || claims == nil || claims.CSRFToken == "" {
		writeServiceError(w, r, services.ErrInvalidSession)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	details, err := h.userService.UserByID(r.Context(), user.ID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	h.setTokenCookies(w, accessToken, refreshToken)
	render.JSON(w, r, loginCodeRedeemResponse{loginResponse: loginBody(sessionID, &details.User, claims.CSRFToken, details.Roles), StoreID: store, StoreName: h.storeName(r.Context(), store)})
}
