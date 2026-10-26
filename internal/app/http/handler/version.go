package handler

import (
	"net/http"

	"rms/internal/version"

	"github.com/go-chi/render"
)

func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, struct {
		ResultCode int      `json:"resultCode"`
		Messages   []string `json:"messages"`
		Data       struct {
			Version string `json:"version"`
		} `json:"data"`
	}{
		Messages: []string{},
		Data: struct {
			Version string `json:"version"`
		}{Version: version.Current},
	})
}
