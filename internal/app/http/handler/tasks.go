package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"rms/internal/domain/models"
	"rms/internal/services"
)

type TaskService interface {
	List(context.Context, string, models.TaskFilter) ([]json.RawMessage, error)
	ByID(context.Context, string, int64) (json.RawMessage, error)
	Write(context.Context, string, int64, json.RawMessage) (json.RawMessage, error)
	Delete(context.Context, string, int64) error
}

func (h *Handler) ConfigureTasks(s TaskService) { h.tasks = s }
func (h *Handler) TaskResource(k string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.tasks == nil {
			writeServiceError(w, r, services.ErrNotFound)
			return
		}
		var id int64
		if chi.URLParam(r, "id") != "" {
			var ok bool
			id, ok = resourceID(w, r)
			if !ok {
				return
			}
		}
		switch r.Method {
		case http.MethodGet:
			if id > 0 {
				v, e := h.tasks.ByID(r.Context(), k, id)
				if e != nil {
					writeServiceError(w, r, e)
					return
				}
				resourceResponse(w, r, v)
				return
			}
			p, ok := resourcePage(w, r)
			if !ok {
				return
			}
			q := r.URL.Query()
			f := models.TaskFilter{Limit: p.Limit, Offset: p.Offset, StoreID: q.Get("store_id"), Name: q.Get("name"), PeriodFrom: q.Get("period_from"), PeriodTo: q.Get("period_to")}
			for key, dest := range map[string]*int64{"id": &f.ID, "type_id": &f.TypeID, "task_type_id": &f.TypeID, "task_id": &f.TaskID, "user_id": &f.UserID} {
				if q.Has(key) {
					n, e := strconv.ParseInt(q.Get(key), 10, 64)
					if e != nil || n <= 0 {
						writeServiceError(w, r, services.ErrInvalid)
						return
					}
					*dest = n
				}
			}
			if q.Has("type_id") && q.Has("task_type_id") {
				writeServiceError(w, r, services.ErrInvalid)
				return
			}
			v, e := h.tasks.List(r.Context(), k, f)
			if e != nil {
				writeServiceError(w, r, e)
				return
			}
			resourceResponse(w, r, v)
		case http.MethodPost, http.MethodPut:
			var data json.RawMessage
			if !decodeRequestLimit(w, r, &data, 2<<20) {
				return
			}
			v, e := h.tasks.Write(r.Context(), k, id, data)
			if e != nil {
				writeServiceError(w, r, e)
				return
			}
			if r.Method == http.MethodPost {
				render.Status(r, http.StatusCreated)
			}
			resourceResponse(w, r, v)
		case http.MethodDelete:
			if e := h.tasks.Delete(r.Context(), k, id); e != nil {
				writeServiceError(w, r, e)
				return
			}
			resourceResponse(w, r, nil)
		}
	}
}
