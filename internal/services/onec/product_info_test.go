package onec

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetProductInfo(t *testing.T) {
	store := uuid.MustParse("840c811d-2d41-11ef-8da0-00155d1a6906")
	products := []uuid.UUID{uuid.MustParse("c046e97f-612d-11ef-8da0-00155d1a6906"), uuid.MustParse("80617dff-6144-11ef-8da0-00155d1a6906")}
	receipt := fmt.Sprintf(`{"number":"00001234","type":"Перемещение","date":"2025-11-20T09:41:56","supplier":"101_Сочи_РЦ","quantity":12.375,"store_id":%q,"product_id":%q}`, store, products[0])
	sale := fmt.Sprintf(`{"store_id":%q,"product_id":%q,"sold_yesterday_quantity":1.25,"sold_week_quantity":4,"receipts_yesterday_quantity":1,"receipts_week_quantity":3}`, store, products[0])
	for _, tc := range []struct {
		name, body, wantErr string
		status, count       int
	}{
		{"success", `{"code":0,"mess":"","data":{"receipts":[` + receipt + `],"sales":[` + sale + `],"operations":[]}}`, "", 200, 1},
		{"empty", `{"code":0,"mess":"","data":{"receipts":[],"sales":[]}}`, "", 200, 0},
		{"empty body", "", "", 200, 0},
		{"whitespace body", " \n\t", "", 200, 0},
		{"no content", "", "", 204, 0},
		{"business error", `{"code":1,"mess":"Ошибка 1С","data":{"receipts":[],"sales":[]}}`, "Ошибка 1С", 200, 0},
		{"missing code", `{"data":{"receipts":[],"sales":[]}}`, "missing", 200, 0},
		{"missing data", `{"code":0}`, "data object", 200, 0},
		{"null data", `{"code":0,"data":null}`, "data object", 200, 0},
		{"invalid json", `{`, "decode", 200, 0},
		{"trailing data", `{"code":0,"data":{"receipts":[],"sales":[]}} {}`, "unexpected", 200, 0},
		{"invalid date", `{"code":0,"data":{"receipts":[` + strings.ReplaceAll(receipt, "2025-11-20T09:41:56", "invalid") + `],"sales":[]}}`, "date", 200, 0},
		{"invalid uuid", `{"code":0,"data":{"receipts":[` + strings.ReplaceAll(receipt, products[0].String(), "invalid") + `],"sales":[]}}`, "decode", 200, 0},
		{"nil uuid", `{"code":0,"data":{"receipts":[` + strings.ReplaceAll(receipt, products[0].String(), uuid.Nil.String()) + `],"sales":[]}}`, "invalid", 200, 0},
		{"invalid sales", `{"code":0,"data":{"receipts":[],"sales":[` + strings.ReplaceAll(sale, products[0].String(), uuid.Nil.String()) + `]}}`, "invalid", 200, 0},
		{"http error", ``, "HTTP 500", 500, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/GetProductInfo" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.URL.Query().Get("store_id") != store.String() || r.URL.Query().Get("products") != products[0].String()+","+products[1].String() {
					t.Errorf("unexpected query: %s", r.URL.RawQuery)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			service, err := New(Config{BaseURL: server.URL, Timeout: time.Second}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := service.GetProductInfo(context.Background(), store, products)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || got == nil || len(got.Receipts) != tc.count {
				t.Fatalf("receipts: %+v, %v", got, err)
			}
			if tc.name == "success" && (len(got.Sales) != 1 || got.Sales[0].SoldYesterdayQuantity != "1.25" || got.Sales[0].SoldWeekQuantity != "4" || got.Sales[0].ReceiptsYesterdayQuantity != "1" || got.Sales[0].ReceiptsWeekQuantity != "3") {
				t.Fatalf("sales fields: %+v", got.Sales)
			}
			if len(got.Receipts) > 0 && (got.Receipts[0].Quantity != "12.375" || got.Receipts[0].Number != "00001234" || got.Receipts[0].Date != "2025-11-20T09:41:56" || got.Receipts[0].Supplier != "101_Сочи_РЦ" || got.Receipts[0].Type != "Перемещение" || got.Receipts[0].StoreID != store || got.Receipts[0].ProductID != products[0]) {
				t.Fatalf("receipt fields: %+v", got.Receipts[0])
			}
		})
	}
}

func TestGetProductInfoEmptyAndInvalidFilters(t *testing.T) {
	service := &Service{}
	got, err := service.GetProductInfo(context.Background(), uuid.New(), nil)
	if err != nil || got == nil || len(got.Receipts) != 0 {
		t.Fatalf("empty products: %+v %v", got, err)
	}
	if _, err := service.GetProductInfo(context.Background(), uuid.Nil, []uuid.UUID{uuid.New()}); err == nil {
		t.Fatal("accepted missing store")
	}
	if _, err := service.GetProductInfo(context.Background(), uuid.New(), []uuid.UUID{uuid.Nil}); err == nil {
		t.Fatal("accepted missing product")
	}
}
