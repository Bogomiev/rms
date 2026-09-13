package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"net/http/httptest"
	"rms/internal/domain/models"
	"testing"
)

type catalogStub struct {
	serviceStub
	valid    bool
	err      error
	received string
}

func (s *catalogStub) UserTokenValid(_ context.Context, value string) (bool, error) {
	s.received = value
	return s.valid, s.err
}
func (s *catalogStub) Products(context.Context) ([]models.Product, error) {
	return []models.Product{{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Code: "001", Name: "Product", MarkingType: "none", Barcodes: []models.BarcodeInfo{{Barcode: "123", Unit: "pc", Ratio: 1, IsBase: true}}}}, s.err
}
func (s *catalogStub) Stores(context.Context) (models.StoresResponse, error) {
	return models.StoresResponse{Page: 1, PerPage: 1, TotalPages: 1, TotalItems: 1, Items: []models.Store{{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Code: "001", Name: "Store", Address: "address"}}}, s.err
}
func TestCatalogResponses(t *testing.T) {
	s := &catalogStub{}
	h := NewHandler(nil, s)
	h.Configure(s, s, 0, 0)
	for _, tc := range []struct{ route, want string }{
		{"products", `[{"uid":"00000000-0000-0000-0000-000000000001","parent_id":"00000000-0000-0000-0000-000000000000","marked":false,"code":"001","name":"Product","markingType":"none","isWeight":false,"isThermalMode":false,"barcodes":[{"barcode":"123","unit":"pc","ratio":1,"isBase":true}]}]`},
		{"stores", `{"page":1,"perPage":1,"totalPages":1,"totalItems":1,"items":[{"id":"00000000-0000-0000-0000-000000000002","code":"001","name":"Store","uid_1c":"00000000-0000-0000-0000-000000000002","address":"address"}]}`},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/"+tc.route, nil)
		if tc.route == "products" {
			h.Products(w, r)
		} else {
			h.Stores(w, r)
		}
		var got, want any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		json.Unmarshal([]byte(tc.want), &want)
		g, _ := json.Marshal(got)
		e, _ := json.Marshal(want)
		if w.Code != 200 || string(g) != string(e) {
			t.Fatal(w.Code, string(g))
		}
	}
}
func TestUserTokenValidStatus(t *testing.T) {
	for _, tc := range []struct {
		valid  bool
		err    error
		status int
	}{{true, nil, 200}, {false, nil, 400}, {false, errors.New("database"), 500}} {
		s := &catalogStub{valid: tc.valid, err: tc.err}
		h := NewHandler(nil, s)
		w := httptest.NewRecorder()
		h.UserTokenValid(w, httptest.NewRequest("GET", "/usertokenvalid?token=5f133fe4-7403-4741-9cb8-5da588fe83fd", nil))
		if w.Code != tc.status || s.received != "5f133fe4-7403-4741-9cb8-5da588fe83fd" {
			t.Fatal(w.Code, s.received)
		}
	}
}
