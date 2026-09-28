package product

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"rms/internal/domain/models"
)

type infoStub struct {
	products []models.Product
	receipts []models.Receipt
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
func (s *infoStub) Receipts(_ context.Context, store uuid.UUID, ids []uuid.UUID) ([]models.Receipt, error) {
	s.store, s.ids = store, ids
	return s.receipts, s.err
}
func TestProductInfoReceipts(t *testing.T) {
	store, first, second := uuid.New(), uuid.New(), uuid.New()
	receipt := models.Receipt{Number: "00001234", Type: "Перемещение", Date: "2025-11-20T09:41:56", Supplier: "101_Сочи_РЦ", StoreID: store, ProductID: first}
	otherStore := receipt
	otherStore.StoreID = uuid.New()
	otherProduct := receipt
	otherProduct.ProductID = uuid.New()
	stub := &infoStub{products: []models.Product{{ID: first}, {ID: second}}, receipts: []models.Receipt{receipt, otherStore, otherProduct, receipt}}
	service := New(nil, nil)
	service.ConfigureInfo(stub, stub, "UTC")
	service.ConfigureReceipts(stub)
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
	stub.err = errors.New("1C unavailable")
	if _, err := service.ProductInfo(context.Background(), store, models.ProductFilter{}); !errors.Is(err, stub.err) {
		t.Fatalf("receipt error lost: %v", err)
	}
	stub.products = nil
	got, err = service.ProductInfo(context.Background(), store, models.ProductFilter{})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty search: %+v %v", got, err)
	}
}
