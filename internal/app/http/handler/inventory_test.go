package handler

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInventoryFilters(t *testing.T) {
	const id = "a0bf11da-f89f-11e9-80ce-0cc47ae051d5"
	for _, tc := range []struct {
		path          string
		multiple      bool
		status, count int
	}{
		{"/" + id, true, 200, 0}, {"/" + id + "?product_id=" + id + "," + id, true, 200, 2}, {"/" + id + "?product_id=" + id + "&product_id=" + id, true, 200, 2}, {"/" + id + "?store_id=" + id + "&product_id=" + id, false, 200, 1}, {"/" + id + "?store_id=" + id + "&product_id=" + id + "," + id, false, 400, 0}, {"/invalid", true, 400, 0}, {"/" + id, false, 400, 0}, {"/" + id + "?store_id=bad", false, 400, 0}, {"/" + id + "?store_id=" + id, false, 200, 0}, {"/" + id + "?product_id=", true, 400, 0},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := chi.NewRouter()
			r.Get("/{store_id}", func(w http.ResponseWriter, req *http.Request) {
				_, ids, ok := inventoryParams(w, req, tc.multiple)
				if ok && len(ids) != tc.count {
					t.Error(ids)
				}
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.status {
				t.Fatal(w.Code)
			}
		})
	}
}
