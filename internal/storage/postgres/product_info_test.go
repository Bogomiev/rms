package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/services/product"
	"testing"
)

type infoMarketplace struct{ types []uuid.UUID }

type infoReceipts struct{}

func (infoReceipts) Receipts(context.Context, uuid.UUID, []uuid.UUID) ([]models.Receipt, error) {
	return []models.Receipt{}, nil
}

func (m infoMarketplace) MarketplaceData(context.Context) (models.MarketplaceResponse, error) {
	d := map[string]json.RawMessage{}
	for i, k := range []string{"Eshop", "Cooper", "Yandex_Eats"} {
		d[k] = json.RawMessage(fmt.Sprintf(`{"price_type":%q,"price_type_promo":%q}`, m.types[2*i], m.types[2*i+1]))
	}
	return models.MarketplaceResponse{Data: d}, nil
}
func TestProductInfoAndSearch(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	products := make([]models.Product, 12)
	for i := range products {
		products[i] = models.Product{ID: uuid.New(), Code: fmt.Sprintf("%03d", i), Name: "Начало Икра конец", Images: []models.ProductImage{{URL: "image"}}, Barcodes: []models.BarcodeInfo{{Barcode: "123", Unit: "pc", Ratio: 1}}}
	}
	products[0].Name = "100%_товар"
	if err := s.UpsertProducts(ctx, products); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		f models.ProductFilter
		n int
	}{
		{models.ProductFilter{ID: &products[0].ID, Code: products[1].Code, Name: "Икра"}, 1},
		{models.ProductFilter{Code: products[0].Code, Name: "Икра"}, 1},
		{models.ProductFilter{Name: "икра"}, 11},
		{models.ProductFilter{Name: "%_"}, 1},
		{models.ProductFilter{Code: "missing", Name: "Икра"}, 0},
	} {
		got, err := s.SearchProducts(ctx, tc.f, 0)
		if err != nil || len(got) != tc.n {
			t.Fatalf("search %+v: %d %v", tc.f, len(got), err)
		}
	}
	store := models.Store{ID: uuid.New(), PriceType: uuid.New()}
	if _, err := s.AddStore(ctx, &store); err != nil {
		t.Fatal(err)
	}
	types := make([]uuid.UUID, 6)
	for i := range types {
		types[i] = uuid.New()
	}
	all := append([]uuid.UUID{store.PriceType}, types...)
	for i, typ := range all {
		// Include a future price and an older price for every price type.
		_, err := s.db.Exec(`INSERT INTO prices(period,price_type,product_id,price) VALUES
 ('2000-01-01',$1,$2,1),('2001-01-01',$1,$2,$3),('2999-01-01',$1,$2,999)`, typ, products[0].ID, fmt.Sprint(i+10))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReplaceStocks(ctx, []models.Stock{{StoreID: store.ID, ProductID: products[0].ID, Stock: "7.25"}, {StoreID: uuid.New(), ProductID: products[0].ID, Stock: "999"}}); err != nil {
		t.Fatal(err)
	}
	service := product.New(nil, s)
	service.ConfigureInfo(s, infoMarketplace{types}, "Asia/Vladivostok")
	service.ConfigureReceipts(infoReceipts{})
	got, err := service.ProductInfo(ctx, store.ID, models.ProductFilter{ID: &products[0].ID})
	if err != nil || len(got) != 1 {
		t.Fatalf("info: %+v %v", got, err)
	}
	v := got[0]
	values := []json.Number{v.Price, v.PriceEshop, v.PromoPriceEshop, v.PriceOzon, v.PromoPriceOzon, v.PriceYandexEats, v.PromoPriceYandexEats}
	for i, n := range values {
		if n.String() != fmt.Sprint(i+10) {
			t.Fatalf("price %d: %v", i, n)
		}
	}
	if v.Stock != "7.25" || len(v.Images) != 1 || len(v.Barcodes) != 1 {
		t.Fatalf("product fields: %+v", v)
	}
	got, err = service.ProductInfo(ctx, store.ID, models.ProductFilter{})
	if err != nil || len(got) != 10 {
		t.Fatalf("limit: %d %v", len(got), err)
	}
	got, err = service.ProductInfo(ctx, store.ID, models.ProductFilter{ID: &products[1].ID})
	if err != nil || got[0].Price != "0" || got[0].Stock != "0" {
		t.Fatalf("missing values: %+v %v", got, err)
	}

	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"price", "stock", "price_eshop", "price_ozon", "price_yandex_eats", "promo_price_eshop", "promo_price_ozon", "promo_price_yandex_eats"} {
		if string(fields[key]) != "0" {
			t.Fatalf("%s must be numeric zero: %s", key, fields[key])
		}
	}
	got, err = service.ProductInfo(ctx, store.ID, models.ProductFilter{Code: "missing"})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %+v %v", got, err)
	}
}
