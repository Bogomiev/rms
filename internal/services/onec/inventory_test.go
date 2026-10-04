package onec

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	const storeID = "a0bf11da-f89f-11e9-80ce-0cc47ae051d5"
	const productID = "8a617a65-385c-11ee-bbf5-00505601d66f"
	valid := `{"id":"` + productID + `","stock":27.123456789}`
	for _, tc := range []struct {
		name    string
		first   string
		second  string
		want    []models.Stock
		wantErr bool
	}{
		{name: "multiple stores and pages", first: `{"store_id":"` + storeID + `","store_name":"000_ИКОРНЫЙ","stocks":[` + valid + `,{"id":"b0580996-84d0-11e8-a5a3-001dd8b89db0","stock":5309}]}`, second: `{"store_id":"00000000-0000-0000-0000-000000000001","stocks":[{"id":"` + productID + `","stock":0}]}`, want: []models.Stock{
			{StoreID: uuid.MustParse(storeID), ProductID: uuid.MustParse(productID), Stock: "27.123456789"},
			{StoreID: uuid.MustParse(storeID), ProductID: uuid.MustParse("b0580996-84d0-11e8-a5a3-001dd8b89db0"), Stock: "5309"},
			{StoreID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), ProductID: uuid.MustParse(productID), Stock: "0"},
		}},
		{name: "empty store", first: `{"store_id":"` + storeID + `","stocks":[]}`, want: []models.Stock{}},
		{name: "missing store", first: `{"stocks":[` + valid + `]}`, wantErr: true},
		{name: "missing stocks", first: `{"store_id":"` + storeID + `"}`, wantErr: true},
		{name: "null stocks", first: `{"store_id":"` + storeID + `","stocks":null}`, wantErr: true},
		{name: "missing product", first: `{"store_id":"` + storeID + `","stocks":[{"stock":27}]}`, wantErr: true},
		{name: "zero product", first: `{"store_id":"` + storeID + `","stocks":[{"id":"00000000-0000-0000-0000-000000000000","stock":27}]}`, wantErr: true},
		{name: "invalid product", first: `{"store_id":"` + storeID + `","stocks":[{"id":"invalid","stock":27}]}`, wantErr: true},
		{name: "missing quantity", first: `{"store_id":"` + storeID + `","stocks":[{"id":"` + productID + `"}]}`, wantErr: true},
		{name: "duplicate in store", first: `{"store_id":"` + storeID + `","stocks":[` + valid + `,` + valid + `]}`, wantErr: true},
		{name: "duplicate across pages", first: `{"store_id":"` + storeID + `","stocks":[` + valid + `]}`, second: `{"store_id":"` + storeID + `","stocks":[` + valid + `]}`, wantErr: true},
		{name: "invalid second page", first: `{"store_id":"` + storeID + `","stocks":[` + valid + `]}`, second: `{"store_id":"` + storeID + `","stocks":[{}]}`, wantErr: true},
		{name: "empty snapshot", want: []models.Stock{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages, total := 1, 1
			if tc.second != "" {
				pages, total = 2, 2
			}
			if tc.first == "" {
				total = 0
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/GetStocks" {
					t.Error(r.URL.Path)
				}
				page := r.URL.Query().Get("page")
				item := tc.first
				if page == "2" {
					item = tc.second
				}
				fmt.Fprintf(w, `{"page":%s,"perPage":1,"totalPages":%d,"totalItems":%d,"items":[%s]}`, page, pages, total, item)
			}))
			defer server.Close()
			svc, err := New(Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			db := &inventoryStub{}
			svc.ConfigureInventory(db)
			err = svc.SyncStocks(context.Background())
			if tc.wantErr {
				if err == nil || db.calls != 0 {
					t.Fatalf("partial write: %+v %v", db, err)
				}
			} else if err != nil || db.calls != 1 || !reflect.DeepEqual(db.stocks, tc.want) {
				t.Fatalf("stocks=%+v want=%+v calls=%d err=%v", db.stocks, tc.want, db.calls, err)
			}
			if requests != pages {
				t.Fatalf("requests=%d want=%d", requests, pages)
			}
		})
	}
}
