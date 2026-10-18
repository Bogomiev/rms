package handler

import (
	"context"
	"encoding/xml"
	"net/http"
	"rms/internal/domain/models"
)

type DiscountService interface {
	Discounts(context.Context) ([]models.Discount, error)
}

func (h *Handler) ConfigureDiscounts(s DiscountService) { h.discounts = s }
func (h *Handler) Discounts(w http.ResponseWriter, r *http.Request) {
	items, err := h.discounts.Discounts(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	data, err := xml.MarshalIndent(models.Discounts{Items: items}, "", "  ")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="discounts.xml"`)
	w.Write(append([]byte(xml.Header), data...))
}
