package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
	"rms/internal/token"
)

// PriceTagService serves the standing «Печать ценников» store task.
type PriceTagService interface {
	Task(context.Context, int64, time.Time) (models.PriceTagTask, error)
	Add(context.Context, int64, models.PriceTagAddInput, time.Time) error
	Set(context.Context, int64, int64, models.PriceTagItemInput) error
	Delete(context.Context, int64, int64, int64) (string, error)
	Printed(context.Context, int64, models.PriceTagPrintedInput, int64, time.Time) (string, error)
	Products(context.Context, int64, string) ([]models.PriceTagProduct, error)
	Product(context.Context, string, string, time.Time) (models.PriceTagProductState, error)
	AddProduct(context.Context, models.PriceTagProductInput, time.Time) (models.PriceTagProductState, error)
}

// taskNow is the current moment in the scheduler time zone, as task days are counted.
func (h *Handler) taskNow() time.Time { return time.Now().In(h.taskLocation()) }

func claimsUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	claims, ok := r.Context().Value(authKey{}).(*token.UserClaims)
	if !ok || claims.UserID <= 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	return claims.UserID, true
}

func priceTagStatus(w http.ResponseWriter, r *http.Request, status string, err error) {
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resourceResponse(w, r, map[string]string{"status": status})
}

func (h *Handler) ConfigurePriceTags(s PriceTagService) { h.priceTags = s }

func priceTagItemID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(chi.URLParam(r, "item"), 10, 64)
	if e != nil || id <= 0 {
		writeServiceError(w, r, services.ErrInvalid)
		return 0, false
	}
	return id, true
}

func priceTagResult(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, r, resp.OK())
}

// PriceTagTask returns the store screen of a «Печать ценников» task.
func (h *Handler) PriceTagTask(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	v, e := h.priceTags.Task(r.Context(), id, h.taskNow())
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}

// AddPriceTag adds a product to the task by hand.
func (h *Handler) AddPriceTag(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var in models.PriceTagAddInput
	if !decodeRequest(w, r, &in) {
		return
	}
	priceTagResult(w, r, h.priceTags.Add(r.Context(), id, in, h.taskNow()))
}

// SetPriceTag changes the tag type and the number of tags of a position.
func (h *Handler) SetPriceTag(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	item, ok := priceTagItemID(w, r)
	if !ok {
		return
	}
	var in models.PriceTagItemInput
	if !decodeRequest(w, r, &in) {
		return
	}
	priceTagResult(w, r, h.priceTags.Set(r.Context(), id, item, in))
}

// DeletePriceTag removes a position added by hand and returns the task status.
func (h *Handler) DeletePriceTag(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	item, ok := priceTagItemID(w, r)
	if !ok {
		return
	}
	user, ok := claimsUser(w, r)
	if !ok {
		return
	}
	status, e := h.priceTags.Delete(r.Context(), id, item, user)
	priceTagStatus(w, r, status, e)
}

// SetPriceTagsPrinted marks positions as printed; the author comes from the token.
func (h *Handler) SetPriceTagsPrinted(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	user, ok := claimsUser(w, r)
	if !ok {
		return
	}
	var in models.PriceTagPrintedInput
	if !decodeRequest(w, r, &in) {
		return
	}
	status, e := h.priceTags.Printed(r.Context(), id, in, user, h.taskNow())
	priceTagStatus(w, r, status, e)
}

// PriceTagProduct tells the product card about the price tag of a product in a store:
// GET ?store_id=…&product_id=…
func (h *Handler) PriceTagProduct(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, e := h.priceTags.Product(r.Context(), q.Get("store_id"), q.Get("product_id"), h.taskNow())
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}

// AddPriceTagProduct puts a product from the product card into the standing task of the store.
func (h *Handler) AddPriceTagProduct(w http.ResponseWriter, r *http.Request) {
	var in models.PriceTagProductInput
	if !decodeRequest(w, r, &in) {
		return
	}
	v, e := h.priceTags.AddProduct(r.Context(), in, h.taskNow())
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}

// PriceTagProducts finds products to add to the task by name, code or barcode.
func (h *Handler) PriceTagProducts(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	v, e := h.priceTags.Products(r.Context(), id, r.URL.Query().Get("q"))
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
