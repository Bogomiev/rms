package handler

import (
	"context"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"net/http"
	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
	"rms/internal/token"
	"time"
)

type StopperService interface {
	Stoppers(context.Context) ([]models.Stopper, error)
	AddStopper(context.Context, models.StopperInput) (models.Stopper, error)
	SaveStopper(context.Context, int64, models.StopperInput) error
	DeleteStopper(context.Context, int64) error
	Positions(context.Context, models.StopperPositionFilter) ([]models.StopperPosition, error)
	SetPositionsStopper(context.Context, models.StopperAssignInput) error
	SetPositionsPeriod(context.Context, models.StopperPeriodInput) error
	AddManualPosition(context.Context, models.StopperManualInput) error
	Products(context.Context, string) ([]models.StopperProduct, error)
	Task(context.Context, int64, time.Time) (models.StopperTask, error)
	SetTaskCheck(context.Context, int64, models.StopperTaskCheckInput, int64) (string, error)
	CanManage(context.Context, int64) (bool, error)
}

func (h *Handler) ConfigureStoppers(s StopperService) { h.stoppers = s }
func (h *Handler) StopperList(w http.ResponseWriter, r *http.Request) {
	v, e := h.stoppers.Stoppers(r.Context())
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
func (h *Handler) CreateStopper(w http.ResponseWriter, r *http.Request) {
	var in models.StopperInput
	if !decodeRequest(w, r, &in) {
		return
	}
	v, e := h.stoppers.AddStopper(r.Context(), in)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	render.Status(r, http.StatusCreated)
	resourceResponse(w, r, v)
}
func (h *Handler) SaveStopper(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var in models.StopperInput
	if !decodeRequest(w, r, &in) {
		return
	}
	stopperResult(w, r, h.stoppers.SaveStopper(r.Context(), id, in))
}
func (h *Handler) DeleteStopper(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	stopperResult(w, r, h.stoppers.DeleteStopper(r.Context(), id))
}
func (h *Handler) StopperPositions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := models.StopperPositionFilter{From: q.Get("from"), To: q.Get("to")}
	for _, value := range q["store_id"] {
		id, err := uuid.Parse(value)
		if err != nil {
			writeServiceError(w, r, services.ErrInvalid)
			return
		}
		f.Stores = append(f.Stores, id)
	}
	v, e := h.stoppers.Positions(r.Context(), f)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
func (h *Handler) AddStopperPosition(w http.ResponseWriter, r *http.Request) {
	var in models.StopperManualInput
	if !decodeRequest(w, r, &in) {
		return
	}
	if e := h.stoppers.AddManualPosition(r.Context(), in); e != nil {
		writeServiceError(w, r, e)
		return
	}
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, resp.OK())
}
func (h *Handler) SetPositionsStopper(w http.ResponseWriter, r *http.Request) {
	var in models.StopperAssignInput
	if !decodeRequest(w, r, &in) {
		return
	}
	stopperResult(w, r, h.stoppers.SetPositionsStopper(r.Context(), in))
}
func (h *Handler) SetPositionsPeriod(w http.ResponseWriter, r *http.Request) {
	var in models.StopperPeriodInput
	if !decodeRequest(w, r, &in) {
		return
	}
	stopperResult(w, r, h.stoppers.SetPositionsPeriod(r.Context(), in))
}

func (h *Handler) StopperProducts(w http.ResponseWriter, r *http.Request) {
	v, e := h.stoppers.Products(r.Context(), r.URL.Query().Get("q"))
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
func stopperResult(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, r, resp.OK())
}

// StopperTask returns the store screen of a «Стопперы» task.
func (h *Handler) StopperTask(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	v, e := h.stoppers.Task(r.Context(), id, time.Now())
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}

// SetStopperTaskCheck saves a store result for a task position; the author comes from the token.
func (h *Handler) SetStopperTaskCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	claims, ok := r.Context().Value(authKey{}).(*token.UserClaims)
	if !ok || claims.UserID <= 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var in models.StopperTaskCheckInput
	if !decodeRequest(w, r, &in) {
		return
	}
	status, e := h.stoppers.SetTaskCheck(r.Context(), id, in, claims.UserID)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, map[string]string{"status": status})
}

// StopperManagerOnly lets through admins and users with the manager role; it runs after the auth middleware.
func (h *Handler) StopperManagerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(authKey{}).(*token.UserClaims)
		if !ok || claims.UserID <= 0 {
			writeServiceError(w, r, services.ErrInvalidSession)
			return
		}
		allowed, err := h.stoppers.CanManage(r.Context(), claims.UserID)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		if !allowed {
			writeServiceError(w, r, services.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
