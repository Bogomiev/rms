package postgres

import (
	"context"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"testing"
)

func TestDiscountsCurrentPriceAndEnd(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	store := models.Store{ID: uuid.New(), PriceType: uuid.New(), PriceTypePromo: uuid.New()}
	if _, err := s.AddStore(ctx, &store); err != nil {
		t.Fatal(err)
	}
	product, cancelled, future, doc := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	const productCode = "00000123456"
	if _, err := s.AddProduct(ctx, &models.Product{ID: product, Code: productCode, Name: "Product"}); err != nil {
		t.Fatal(err)
	}
	items := []models.Price{
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: product, DocID: doc, Price: "149"},
		{Period: "2099-01-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: product, DocID: doc, Price: "0"},
		{Period: "2020-01-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: cancelled, DocID: doc, Price: "100"},
		{Period: "2021-01-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: cancelled, DocID: doc, Price: "0"},
		{Period: "2099-01-01T00:00:00", PriceType: store.PriceTypePromo, ProductID: future, DocID: doc, Price: "100"},
	}
	if err := s.ReplacePrices(ctx, items); err != nil {
		t.Fatal(err)
	}
	got, err := s.Discounts(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("discounts: %+v", got)
	}
	v := got[0]
	if v.SalePointTag != store.ID.String() || v.ProductID != productCode || v.DocID != doc.String() || v.Price != "149.00" || v.Start != "2020-01-01 00:00:00" || v.End != "2099-01-01 00:00:00" || !v.WriteOffBonus || v.ApplyOtherDiscounts {
		t.Fatalf("%+v", v)
	}
	const tag = "Магазин № 1"
	if _, err := s.UpdateStoreSalePointTag(ctx, store.ID, tag); err != nil {
		t.Fatal(err)
	}
	// The import refreshes catalog fields without overwriting the administrator's tag.
	store.Name = "Updated by 1C"
	saved, err := s.UpdateStore(ctx, &store)
	if err != nil || saved.SalePointTag != tag {
		t.Fatalf("store after import: %+v, %v", saved, err)
	}
	got, err = s.Discounts(ctx, "UTC")
	if err != nil || len(got) != 1 || got[0].SalePointTag != tag {
		t.Fatalf("discount with custom tag: %+v, %v", got, err)
	}
	if _, err := s.UpdateStoreSalePointTag(ctx, store.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, err = s.Discounts(ctx, "UTC")
	if err != nil || len(got) != 1 || got[0].SalePointTag != store.ID.String() {
		t.Fatalf("discount after clearing tag: %+v, %v", got, err)
	}
	// A different document is a distinct key; an exact duplicate still fails.
	items = append(items, models.Price{Period: items[0].Period, PriceType: store.PriceTypePromo, ProductID: product, DocID: uuid.New(), Price: "150"})
	if err := s.ReplacePrices(ctx, items); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePrices(ctx, append(items, items[0])); err == nil {
		t.Fatal("duplicate accepted")
	}
}
