package onec

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"rms/internal/domain/models"
	"testing"
	"time"
)

type inventoryStub struct {
	prices []models.Price
	stocks []models.Stock
	calls  int
}

func (s *inventoryStub) ReplacePrices(_ context.Context, v []models.Price) error {
	s.prices = v
	s.calls++
	return nil
}
func (s *inventoryStub) ReplaceStocks(_ context.Context, v []models.Stock) error {
	s.stocks = v
	s.calls++
	return nil
}
func TestInventoryPaginationAndFailure(t *testing.T) {
	for _, mode := range []string{"success", "failure", "duplicate", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/GetPrices" {
					t.Errorf("path %s", r.URL.Path)
				}
				page := r.URL.Query().Get("page")
				if page == "2" && mode == "failure" {
					w.WriteHeader(500)
					return
				}
				id := page
				if mode == "duplicate" {
					id = "1"
				}
				total := 2
				if mode == "incomplete" {
					total = 3
				}
				fmt.Fprintf(w, `{"page":%s,"perPage":1,"totalPages":2,"totalItems":%d,"items":[{"period":"2025-12-30T00:00:09","price_type":"6c33a838-399a-11e9-8102-0cc47ab32259","product_id":"00000000-0000-0000-0000-00000000000%s","price":600.123456789}]}`, page, total, id)
			}))
			defer server.Close()
			svc, err := New(Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			db := &inventoryStub{}
			svc.ConfigureInventory(db)
			err = svc.SyncPrices(context.Background())
			if mode == "success" {
				if err != nil || db.calls != 1 || len(db.prices) != 2 || db.prices[0].Price != "600.123456789" {
					t.Fatalf("%+v %v", db, err)
				}
			} else if err == nil || db.calls != 0 {
				t.Fatalf("partial write: %+v %v", db, err)
			}
			if requests != 2 {
				t.Fatal(requests)
			}
		})
	}
}
func TestStocksSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/GetStocks" {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, `{"page":1,"perPage":1,"totalPages":1,"totalItems":1,"items":[{"product_id":"efeb82ef-9fa7-11e8-80c0-0cc47ab32259","store_id":"a0bf11da-f89f-11e9-80ce-0cc47ae051d5","stock":990.5}]}`)
	}))
	defer server.Close()
	svc, _ := New(Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
	db := &inventoryStub{}
	svc.ConfigureInventory(db)
	if err := svc.SyncStocks(context.Background()); err != nil || db.calls != 1 || db.stocks[0].Stock != "990.5" {
		t.Fatalf("%+v %v", db, err)
	}
}
