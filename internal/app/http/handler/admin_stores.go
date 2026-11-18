package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/services"
)

type adminStore struct {
	models.Store
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toAdminStore(v models.Store) adminStore {
	return adminStore{Store: v, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func (h *Handler) AdminStores(w http.ResponseWriter, r *http.Request) {
	stores, err := h.storeService.Stores(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items := make([]adminStore, 0, len(stores.Items))
	for _, store := range stores.Items {
		items = append(items, toAdminStore(store))
	}
	resourceResponse(w, r, items)
}

func (h *Handler) UpdateStoreSalePointTag(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || id == uuid.Nil {
		writeServiceError(w, r, services.ErrInvalid)
		return
	}
	var input struct {
		SalePointTag *string `json:"sale_point_tag"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	if input.SalePointTag == nil {
		writeServiceError(w, r, services.ErrInvalid)
		return
	}
	store, err := h.storeService.UpdateStoreSalePointTag(r.Context(), id, *input.SalePointTag)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resourceResponse(w, r, toAdminStore(*store))
}
