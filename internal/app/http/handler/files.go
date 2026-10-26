package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"rms/internal/domain/models"
	"rms/internal/services/files"
	"strconv"
	"strings"
)

type FileService interface {
	List(context.Context, *string, bool) ([]models.File, error)
	Get(context.Context, string, string) (models.File, error)
	Write(context.Context, files.Input, bool) (models.File, error)
	Delete(context.Context, string, ...bool) error
}

func (h *Handler) ConfigureFiles(s FileService) { h.files = s }
func fileError(w http.ResponseWriter, r *http.Request, e error) {
	status := http.StatusInternalServerError
	message := "file operation failed"
	switch {
	case errors.Is(e, files.ErrInvalid):
		status = 400
		message = e.Error()
	case errors.Is(e, files.ErrNotFound):
		status = 404
		message = e.Error()
	case errors.Is(e, files.ErrConflict):
		status = 409
		message = e.Error()
	case errors.Is(e, files.ErrUnavailable):
		status = 503
		message = e.Error()
	}
	render.Status(r, status)
	render.JSON(w, r, map[string]string{"error": message})
}
func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	var parent *string
	if v := r.URL.Query().Get("parent_id"); v != "" {
		if _, e := uuid.Parse(v); e != nil {
			fileError(w, r, files.ErrInvalid)
			return
		}
		parent = &v
	}
	deleted := false
	if v := r.URL.Query().Get("include_deleted"); v != "" {
		var e error
		deleted, e = strconv.ParseBool(v)
		if e != nil {
			fileError(w, r, files.ErrInvalid)
			return
		}
	}
	out, e := h.files.List(r.Context(), parent, deleted)
	if e != nil {
		fileError(w, r, e)
		return
	}
	render.JSON(w, r, out)
}
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	out, e := h.files.Get(r.Context(), chi.URLParam(r, "id"), "")
	if e != nil {
		fileError(w, r, e)
		return
	}
	render.JSON(w, r, out)
}
func (h *Handler) FileByPath(w http.ResponseWriter, r *http.Request) {
	out, e := h.files.Get(r.Context(), "", r.URL.Query().Get("path"))
	if e != nil {
		fileError(w, r, e)
		return
	}
	render.JSON(w, r, out)
}
func (h *Handler) CreateFile(w http.ResponseWriter, r *http.Request) { h.writeFile(w, r, false) }
func (h *Handler) UpdateFile(w http.ResponseWriter, r *http.Request) { h.writeFile(w, r, true) }
func (h *Handler) writeFile(w http.ResponseWriter, r *http.Request, update bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 48<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var in files.Input
	e := d.Decode(&in)
	if e == nil {
		var extra any
		if d.Decode(&extra) != io.EOF {
			e = files.ErrInvalid
		}
	}
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		fileError(w, r, files.ErrInvalid)
		return
	}
	if update {
		if in.ID != "" && in.ID != chi.URLParam(r, "id") {
			fileError(w, r, files.ErrInvalid)
			return
		}
		in.ID = chi.URLParam(r, "id")
	}
	out, e := h.files.Write(r.Context(), in, update)
	if e != nil {
		fileError(w, r, e)
		return
	}
	if !update {
		render.Status(r, http.StatusCreated)
	}
	render.JSON(w, r, out)
}
func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	marked := true
	if v := r.URL.Query().Get("marked"); v != "" {
		var e error
		marked, e = strconv.ParseBool(v)
		if e != nil {
			fileError(w, r, files.ErrInvalid)
			return
		}
	}
	if e := h.files.Delete(r.Context(), chi.URLParam(r, "id"), marked); e != nil {
		fileError(w, r, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) FileContent(w http.ResponseWriter, r *http.Request) {
	service, ok := h.files.(interface {
		Content(context.Context, string) (models.File, io.ReadCloser, error)
	})
	if !ok {
		fileError(w, r, files.ErrUnavailable)
		return
	}
	f, body, err := service.Content(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		fileError(w, r, err)
		return
	}
	defer body.Close()
	kind := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(f.Name)) {
	case ".pdf":
		kind = "application/pdf"
	case ".html":
		kind = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": f.Name}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:")
	_, _ = io.Copy(w, body)
}
