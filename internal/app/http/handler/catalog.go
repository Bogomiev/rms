package handler

import (
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"net/http"
)

func (h *Handler) Products(w http.ResponseWriter, r *http.Request) {
	data, err := h.productService.Products(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, r, data)
}

type storeData struct {
	ID      uuid.UUID `json:"id"`
	UID1C   uuid.UUID `json:"uid_1c"`
	Code    string    `json:"code"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
}
type storesData struct {
	Page       int         `json:"page"`
	PerPage    int         `json:"perPage"`
	TotalPages int         `json:"totalPages"`
	TotalItems int         `json:"totalItems"`
	Items      []storeData `json:"items"`
}

func (h *Handler) Stores(w http.ResponseWriter, r *http.Request) {
	data, err := h.storeService.Stores(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items := make([]storeData, 0, len(data.Items))
	for _, v := range data.Items {
		items = append(items, storeData{ID: v.ID, UID1C: v.ID, Code: v.Code, Name: v.Name, Address: v.Address})
	}
	render.JSON(w, r, storesData{Page: data.Page, PerPage: data.PerPage, TotalPages: data.TotalPages, TotalItems: data.TotalItems, Items: items})
}
func (h *Handler) UserTokenValid(w http.ResponseWriter, r *http.Request) {
	valid, err := h.userService.UserTokenValid(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if !valid {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}
