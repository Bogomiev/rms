package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"

	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
	"rms/internal/services/returnrequest"
	"rms/internal/token"
)

type ReturnRequestService interface {
	GetReturnRequestStatus(context.Context, uuid.UUID, uuid.UUID) (models.ReturnStatus, error)
	List(context.Context, uuid.UUID, int) ([]models.ReturnRequest, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (models.ReturnRequest, error)
	Create(context.Context, models.ReturnRequest) (models.ReturnRequest, error)
	Update(context.Context, models.ReturnRequest) (models.ReturnRequest, error)
	ChangeStatus(context.Context, uuid.UUID, uuid.UUID, models.ReturnStatus) (models.ReturnRequest, error)
}

func (h *Handler) ConfigureReturns(s ReturnRequestService) { h.returns = s }

type returnDraft struct {
	ID     uuid.UUID           `json:"id"`
	Seller uuid.UUID           `json:"seller"`
	Items  []models.ReturnItem `json:"items"`
}

func (h *Handler) ReturnRequests(w http.ResponseWriter, r *http.Request) {
	if h.returns == nil {
		writeServiceError(w, r, services.ErrNotFound)
		return
	}
	ctx := r.Context()
	store, err := uuid.Parse(r.URL.Query().Get("store_id"))
	if err != nil || store == uuid.Nil {
		writeServiceError(w, r, services.ErrInvalid)
		return
	}
	var id uuid.UUID
	if raw := chi.URLParam(r, "id"); raw != "" {
		id, err = uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			writeServiceError(w, r, services.ErrInvalid)
			return
		}
	}
	if r.Method == http.MethodGet {
		if id != uuid.Nil {
			result, err := h.returns.Get(ctx, store, id)
			writeReturnResponse(w, r, result, err)
			return
		}
		days := 0
		if r.URL.Query().Has("history_days") {
			days, err = strconv.Atoi(r.URL.Query().Get("history_days"))
			if err != nil {
				writeServiceError(w, r, services.ErrInvalid)
				return
			}
		}
		items, err := h.returns.List(ctx, store, days)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		resourceResponse(w, r, struct {
			Items      []models.ReturnRequest `json:"items"`
			TotalItems int                    `json:"totalItems"`
		}{items, len(items)})
		return
	}

	if r.Method == http.MethodPost && id != uuid.Nil {
		// Status changes wait for confirmation from 1C.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(27 * time.Second))
		ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		var body struct {
			Status models.ReturnStatus `json:"status"`
		}
		if !decodeRequest(w, r, &body) {
			return
		}
		result, err := h.returns.ChangeStatus(ctx, store, id, body.Status)
		writeReturnResponse(w, r, result, err)
		return
	}

	claims, ok := ctx.Value(authKey{}).(*token.UserClaims)
	if !ok || claims.UserID <= 0 {
		writeServiceError(w, r, services.ErrInvalidSession)
		return
	}
	var body returnDraft
	if !decodeRequestLimit(w, r, &body, 2<<20) {
		return
	}
	if id != uuid.Nil && body.ID != uuid.Nil && id != body.ID {
		writeServiceError(w, r, services.ErrInvalid)
		return
	}
	request := models.ReturnRequest{ID: id, StoreID: store, Seller: body.Seller, Manager: claims.UserID, Items: body.Items}
	if r.Method == http.MethodPost {
		request.ID = body.ID
		result, err := h.returns.Create(ctx, request)
		if err == nil {
			render.Status(r, http.StatusCreated)
		}
		writeReturnResponse(w, r, result, err)
		return
	}
	result, err := h.returns.Update(ctx, request)
	writeReturnResponse(w, r, result, err)
}

func (h *Handler) GetReturnRequestStatus(w http.ResponseWriter, r *http.Request) {
	if h.returns == nil {
		writeServiceError(w, r, services.ErrNotFound)
		return
	}
	store, storeErr := uuid.Parse(r.URL.Query().Get("store_id"))
	id, idErr := uuid.Parse(chi.URLParam(r, "id"))
	if storeErr != nil || idErr != nil || store == uuid.Nil || id == uuid.Nil {
		writeServiceError(w, r, services.ErrInvalid)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(27 * time.Second))
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	status, err := h.returns.GetReturnRequestStatus(ctx, store, id)
	writeReturnResponse(w, r, struct {
		Status models.ReturnStatus `json:"status"`
	}{status}, err)
}

func writeReturnResponse(w http.ResponseWriter, r *http.Request, result any, err error) {
	var oneCError *returnrequest.OneCError
	switch {
	case err == nil:
		resourceResponse(w, r, result)
	case errors.As(err, &oneCError):
		resp.HttpResponseError(w, r, http.StatusBadGateway, 1, []string{oneCError.Error()})
	default:
		writeServiceError(w, r, err)
	}
}
