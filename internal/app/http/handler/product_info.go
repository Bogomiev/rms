package handler

import (
	"context"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"net/http"
	"rms/internal/domain/models"
)

type MarketplaceService interface {
	MarketplaceData(context.Context) (models.MarketplaceResponse, error)
}
type ProductInfoService interface {
	ProductInfo(context.Context, uuid.UUID, models.ProductFilter) ([]models.ProductInfo, error)
}

func (h *Handler) ConfigureProductInfo(p ProductInfoService, m MarketplaceService) {
	h.productInfo = p
	h.marketplace = m
}
func productFilter(w http.ResponseWriter, r *http.Request) (models.ProductFilter, bool) {
	q := r.URL.Query()
	f := models.ProductFilter{Code: q.Get("code"), Name: q.Get("name")}
	if q.Has("id") {
		id, err := uuid.Parse(q.Get("id"))
		if err != nil || id == uuid.Nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return f, false
		}
		f.ID = &id
	}
	return f, true
}
func (h *Handler) MarketplaceData(w http.ResponseWriter, r *http.Request) {
	data, err := h.marketplace.MarketplaceData(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, r, data)
}
func (h *Handler) ProductInfo(w http.ResponseWriter, r *http.Request) {
	store, err := uuid.Parse(r.URL.Query().Get("store_id"))
	if err != nil || store == uuid.Nil {
		writeProductInfoError(w, r, "invalid store_id")
		return
	}
	f := models.ProductFilter{Code: r.URL.Query().Get("code"), Name: r.URL.Query().Get("name")}
	if r.URL.Query().Has("product_id") {
		value := r.URL.Query().Get("product_id")
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil {
			writeProductInfoError(w, r, "invalid product_id")
			return
		}
		f.ID = &id
	}
	data, err := h.productInfo.ProductInfo(r.Context(), store, f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, r, struct {
		ResultCode int                  `json:"resultCode"`
		Messages   []string             `json:"messages"`
		Data       []models.ProductInfo `json:"data"`
	}{0, []string{}, data})
}

func writeProductInfoError(w http.ResponseWriter, r *http.Request, message string) {
	render.Status(r, http.StatusBadRequest)
	render.JSON(w, r, struct {
		ResultCode int                  `json:"resultCode"`
		Messages   []string             `json:"messages"`
		Data       []models.ProductInfo `json:"data"`
	}{1, []string{message}, []models.ProductInfo{}})
}
