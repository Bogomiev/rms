package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"net/http"
	"rms/internal/domain/models"
	"strings"
)

type PriceService interface {
	Prices(context.Context, uuid.UUID, *uuid.UUID) (models.Page[models.Price], error)
}
type StockService interface {
	Stocks(context.Context, uuid.UUID, []uuid.UUID) (models.Page[models.Stock], error)
}
type InventoryService interface {
	Prices(context.Context, uuid.UUID, *uuid.UUID) (models.Page[models.Price], error)
	Stocks(context.Context, uuid.UUID, []uuid.UUID) (models.Page[models.Stock], error)
}

func (h *Handler) ConfigureInventory(inventory InventoryService) {
	h.ConfigurePriceStock(inventory, inventory)
}
func (h *Handler) ConfigurePriceStock(prices PriceService, stocks StockService) {
	h.prices = prices
	h.stocks = stocks
}
func inventoryParams(w http.ResponseWriter, r *http.Request, multiple bool) (uuid.UUID, []uuid.UUID, bool) {
	storeValue := chi.URLParam(r, "store_id")
	if !multiple {
		storeValue = r.URL.Query().Get("store_id")
	}
	store, err := uuid.Parse(storeValue)
	if err != nil || store == uuid.Nil {
		http.Error(w, "invalid store_id", http.StatusBadRequest)
		return uuid.Nil, nil, false
	}
	products := []uuid.UUID{}
	for _, value := range r.URL.Query()["product_id"] {
		for _, part := range strings.Split(value, ",") {
			id, err := uuid.Parse(strings.TrimSpace(part))
			if err != nil || id == uuid.Nil {
				http.Error(w, "invalid product_id", http.StatusBadRequest)
				return uuid.Nil, nil, false
			}
			products = append(products, id)
		}
	}
	if !multiple && len(products) > 1 {
		http.Error(w, "only one product_id is allowed", http.StatusBadRequest)
		return uuid.Nil, nil, false
	}
	return store, products, true
}
func (h *Handler) Prices(w http.ResponseWriter, r *http.Request) {
	store, products, ok := inventoryParams(w, r, false)
	if !ok {
		return
	}
	var product *uuid.UUID
	if len(products) > 0 {
		product = &products[0]
	}
	data, err := h.prices.Prices(r.Context(), store, product)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, r, data)
}
func (h *Handler) Stocks(w http.ResponseWriter, r *http.Request) {
	store, products, ok := inventoryParams(w, r, true)
	if !ok {
		return
	}
	data, err := h.stocks.Stocks(r.Context(), store, products)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, r, data)
}
