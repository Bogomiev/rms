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
	if v.StoreID != store.ID.String() || v.ProductID != product.String() || v.DocID != doc.String() || v.Price != "149.00" || v.Start != "2020-01-01 00:00:00" || v.End != "2099-01-01 00:00:00" || !v.WriteOffBonus || v.ApplyOtherDiscounts {
		t.Fatalf("%+v", v)
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
