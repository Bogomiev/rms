package handler

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http/httptest"
	"rms/internal/domain/models"
	"strings"
	"testing"
)

type productInfoStub struct {
	store  uuid.UUID
	filter models.ProductFilter
	calls  int
}

func (s *productInfoStub) ProductInfo(_ context.Context, store uuid.UUID, f models.ProductFilter) ([]models.ProductInfo, error) {
	s.calls++
	s.store = store
	s.filter = f
	return []models.ProductInfo{}, nil
}
func (s *productInfoStub) MarketplaceData(context.Context) (models.MarketplaceResponse, error) {
	return models.MarketplaceResponse{Messages: []string{}, Data: map[string]json.RawMessage{"Eshop": json.RawMessage(`{"name":"shop"}`)}}, nil
}
func TestProductInfoParametersAndEnvelope(t *testing.T) {
	s := &productInfoStub{}
	h := NewHandler(nil, nil)
	h.ConfigureProductInfo(s, s)
	router := chi.NewRouter()
	router.Get("/product_info", h.ProductInfo)
	for _, query := range []string{"", "?store_id=", "?store_id=bad", "?product_id=bad&store_id=" + uuid.NewString(), "?product_id=&store_id=" + uuid.NewString(), "?product_id=" + uuid.Nil.String() + "&store_id=" + uuid.NewString(), "?store_id=" + uuid.Nil.String()} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/product_info"+query, nil))
		if w.Code != 400 || s.calls != 0 {
			t.Fatalf("invalid query %s: %d", query, w.Code)
		}
		var body struct {
			ResultCode int                  `json:"resultCode"`
			Messages   []string             `json:"messages"`
			Data       []models.ProductInfo `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		message := "invalid store_id"
		if strings.Contains(query, "product_id=") {
			message = "invalid product_id"
		}
		if body.ResultCode != 1 || len(body.Messages) != 1 || body.Messages[0] != message || body.Data == nil || len(body.Data) != 0 {
			t.Fatalf("invalid error envelope: %s", w.Body)
		}

	}
	store, id := uuid.New(), uuid.New()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/product_info?product_id="+id.String()+"&store_id="+store.String()+"&code=123&name=test", nil))
	if w.Code != 200 || s.store != store || s.filter.ID == nil || *s.filter.ID != id || s.filter.Code != "123" || s.filter.Name != "test" {
		t.Fatalf("parameters: %+v %s", s, w.Body)
	}
	if w.Body.String() != "{\"resultCode\":0,\"messages\":[],\"data\":[]}\n" {
		t.Fatalf("envelope: %s", w.Body)
	}
	w = httptest.NewRecorder()
	h.MarketplaceData(w, httptest.NewRequest("GET", "/MarketplaceData", nil))
	var got map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if string(got["resultCode"]) != "0" || string(got["messages"]) != "[]" || got["code"] != nil || got["mess"] != nil {
		t.Fatalf("marketplace envelope: %s", w.Body)
	}
}
