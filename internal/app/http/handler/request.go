package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"
	"strconv"
)

const maxRequestBytes = 16 * 1024

func decodeRequest(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(value)
	if err == nil {
		var extra any
		err = decoder.Decode(&extra)
		if err == io.EOF {
			return true
		}
	}
	status, message := http.StatusBadRequest, "body must contain one valid JSON object with known fields"
	var sizeError *http.MaxBytesError
	if errors.As(err, &sizeError) {
		status, message = http.StatusRequestEntityTooLarge, "request body exceeds 16 KiB"
	}
	resp.HttpResponseError(w, r, status, 1, []string{message})
	return false
}
func userPage(r *http.Request) (models.UserPage, error) {
	page := models.UserPage{Limit: 50}
	for name, target := range map[string]*int{"limit": &page.Limit, "offset": &page.Offset} {
		if values, ok := r.URL.Query()[name]; ok {
			if len(values) != 1 {
				return page, errors.New("duplicate pagination parameter")
			}
			value, err := strconv.Atoi(values[0])
			if err != nil {
				return page, err
			}
			*target = value
		}
	}
	if !page.Valid() {
		return page, errors.New("invalid pagination")
	}
	return page, nil
}
