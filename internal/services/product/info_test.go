package product

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"rms/internal/domain/models"
)

type infoStub struct {
	products []models.Product
	receipts []models.Receipt
	sales    []models.ProductSales
	err      error
	store    uuid.UUID
	ids      []uuid.UUID
}

func (s *infoStub) SearchProducts(context.Context, models.ProductFilter, int) ([]models.Product, error) {
	return s.products, nil
}
func (s *infoStub) ProductValues(context.Context, uuid.UUID, []uuid.UUID, []uuid.UUID, string) (map[uuid.UUID]models.ProductValues, error) {
	return nil, nil
}
func (s *infoStub) MarketplaceData(context.Context) (models.MarketplaceResponse, error) {
	return models.MarketplaceResponse{Data: map[string]json.RawMessage{"Eshop": json.RawMessage(`{}`), "Cooper": json.RawMessage(`{}`), "Yandex_Eats": json.RawMessage(`{}`)}}, nil
}
func (s *infoStub) GetProductInfo(_ context.Context, store uuid.UUID, ids []uuid.UUID) (*models.OneCProductInfo, error) {
	s.store, s.ids = store, ids
	return &models.OneCProductInfo{Receipts: s.receipts, Sales: s.sales}, s.err
}
func TestProductInfoReceipts(t *testing.T) {
	store, first, second := uuid.New(), uuid.New(), uuid.New()
	receipt := models.Receipt{Quantity: "12.375", Number: "00001234", Type: "Перемещение", Date: "2025-11-20T09:41:56", Supplier: "101_Сочи_РЦ", StoreID: store, ProductID: first}
	otherStore := receipt
	otherStore.StoreID = uuid.New()
	otherProduct := receipt
	otherProduct.ProductID = uuid.New()
	stub := &infoStub{products: []models.Product{{ID: first}, {ID: second}}, receipts: []models.Receipt{receipt, otherStore, otherProduct, receipt}}
	var logs bytes.Buffer
	service := New(slog.New(slog.NewJSONHandler(&logs, nil)), nil)
	service.ConfigureInfo(stub, stub, "UTC")
	service.ConfigureProductInfo(stub)
	got, err := service.ProductInfo(context.Background(), store, models.ProductFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if stub.store != store || !reflect.DeepEqual(stub.ids, []uuid.UUID{first, second}) {
		t.Fatalf("incorrect receipt filters: %v %v", stub.store, stub.ids)
	}
	if len(got) != 2 || !reflect.DeepEqual(got[0].Receipts, []models.Receipt{receipt, receipt}) {
		t.Fatalf("incorrect receipts: %+v", got)
	}
	rawReceipts, err := json.Marshal(got[0].Receipts)
	if err != nil {
		t.Fatal(err)
	}
	var receiptFields []map[string]json.RawMessage
	if err := json.Unmarshal(rawReceipts, &receiptFields); err != nil {
		t.Fatal(err)
	}
	for _, fields := range receiptFields {
		if string(fields["quantity"]) != "12.375" {
			t.Fatalf("expected numeric receipt quantity: %s", rawReceipts)
		}
	}
	raw, err := json.Marshal(got[1])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["receipts"]) != "[]" {
		t.Fatalf("expected empty array: %s", raw)
	}
	for _, key := range []string{"sold_yesterday_quantity", "sold_week_quantity", "receipts_yesterday_quantity", "receipts_week_quantity"} {
		if string(fields[key]) != "0.000" {
			t.Fatalf("expected numeric zero for %s: %s", key, raw)
		}
	}
	if string(fields["stock_days"]) != "0" {
		t.Fatalf("expected zero stock days without sales: %s", raw)
	}
	stub.sales = []models.ProductSales{
		{StoreID: store, ProductID: first, SoldYesterdayQuantity: "1.25", SoldWeekQuantity: "4", ReceiptsYesterdayQuantity: "1", ReceiptsWeekQuantity: "3"},
		{StoreID: uuid.New(), ProductID: second, SoldWeekQuantity: "99"},
	}
	got, err = service.ProductInfo(context.Background(), store, models.ProductFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].SoldYesterdayQuantity != "1.25" || got[0].SoldWeekQuantity != "4" || got[0].ReceiptsYesterdayQuantity != "1" || got[0].ReceiptsWeekQuantity != "3" || got[1].SoldWeekQuantity != "0.000" {
		t.Fatalf("incorrect sales: %+v", got)
	}
	stub.err = errors.New("1C unavailable")
	if _, err := service.ProductInfo(context.Background(), store, models.ProductFilter{}); !errors.Is(err, stub.err) {
		t.Fatalf("receipt error lost: %v", err)
	}
	var entry struct {
		Message string `json:"msg"`
		Stage   string `json:"stage"`
		StoreID string `json:"store_id"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("expected one error log: %s: %v", &logs, err)
	}
	if entry.Message != "product_info failed" || entry.Stage != "1C product info" || entry.StoreID != store.String() || !strings.Contains(entry.Error, "1C unavailable") {
		t.Fatalf("incomplete error diagnostics: %+v", entry)
	}
	stub.products = nil
	got, err = service.ProductInfo(context.Background(), store, models.ProductFilter{})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty search: %+v %v", got, err)
	}
}

func TestStockDays(t *testing.T) {
	for _, tc := range []struct {
		stock, sales json.Number
		want         int64
	}{
		{"10", "7", 10},
		{"1.005", "2.345", 3},
		{"0.009", "0.021", 3},
		{"1", "14", 0},
		{"10", "0.000", 0},
		{"0", "7", 0},
		{"-1", "7", 0},
		{"10", "-7", 0},
		{"", "7", 0},
		{"10", "", 0},
	} {
		if got := stockDays(tc.stock, tc.sales); got != tc.want {
			t.Errorf("stockDays(%s, %s) = %d, want %d", tc.stock, tc.sales, got, tc.want)
		}
	}
}
