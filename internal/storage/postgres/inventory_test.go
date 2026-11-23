package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	"rms/internal/domain/models"
	"testing"
	"time"
)

func TestInventorySnapshots(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	store := models.Store{ID: uuid.New(), PriceType: uuid.New()}
	if _, err := s.AddStore(ctx, &store); err != nil {
		t.Fatal(err)
	}
	product := uuid.New()
	prices := []models.Price{{Period: "2025-12-30T00:00:09", PriceType: store.PriceType, ProductID: product, Price: json.Number("123456789.123456789")}, {Period: "2025-12-30T00:00:09", PriceType: uuid.New(), ProductID: product, Price: "9"}}
	for i := 0; i < 3; i++ {
		if err := s.ReplacePrices(ctx, prices); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Prices(ctx, store.ID, &product)
	if err != nil || got.TotalItems != 1 || got.Items[0].Price != prices[0].Price {
		t.Fatalf("prices: %+v %v", got, err)
	}
	if err = s.ReplacePrices(ctx, append(prices, prices[0])); err == nil {
		t.Fatal("duplicate accepted")
	}
	got, err = s.Prices(ctx, store.ID, nil)
	if err != nil || got.TotalItems != 1 {
		t.Fatalf("rollback: %+v %v", got, err)
	}
	stocks := []models.Stock{{StoreID: store.ID, ProductID: product, Stock: "990.125"}, {StoreID: store.ID, ProductID: uuid.New(), Stock: "0"}, {StoreID: uuid.New(), ProductID: product, Stock: "7"}}
	for i := 0; i < 3; i++ {
		if err = s.ReplaceStocks(ctx, stocks); err != nil {
			t.Fatal(err)
		}
	}
	stockPage, err := s.Stocks(ctx, store.ID, []uuid.UUID{product})
	if err != nil || stockPage.TotalItems != 1 || stockPage.Items[0].Stock != "990.125" {
		t.Fatalf("stocks: %+v %v", stockPage, err)
	}
	if err = s.ReplaceStocks(ctx, append(stocks, stocks[0])); err == nil {
		t.Fatal("duplicate accepted")
	}
	stockPage, err = s.Stocks(ctx, store.ID, nil)
	if err != nil || stockPage.TotalItems != 2 {
		t.Fatalf("rollback: %+v %v", stockPage, err)
	}
	if err = s.ReplacePrices(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.ReplaceStocks(ctx, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.Prices(ctx, store.ID, nil)
	if err != nil || got.TotalItems != 0 || got.Items == nil {
		t.Fatalf("empty: %+v %v", got, err)
	}
	for _, table := range []string{"prices_staging", "stocks_staging"} {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("staging: %d %v", count, err)
		}
	}
}
func TestProductImages(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	p := models.Product{ID: uuid.New(), Images: []models.ProductImage{{URL: "https://example.org/image.jpg", Hash: "abc"}, {URL: "https://example.org/other.jpg", Hash: "def"}}}
	if err := s.UpsertProducts(ctx, []models.Product{p}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProduct(ctx, p.ID)
	if err != nil || len(got.Images) != 2 || got.Images[0] != p.Images[0] {
		t.Fatalf("images: %+v %v", got, err)
	}
	p.Images = nil
	if err = s.UpsertProducts(ctx, []models.Product{p}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetProduct(ctx, p.ID)
	if err != nil || got.Images == nil || len(got.Images) != 0 {
		t.Fatalf("clear: %+v %v", got, err)
	}
}

func TestSnapshotConcurrentReads(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	store := uuid.New()
	product := uuid.New()
	items := []models.Stock{{StoreID: store, ProductID: product, Stock: "1"}}
	if err := s.ReplaceStocks(ctx, items); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 40; i++ {
			if err := s.ReplaceStocks(ctx, items); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 100; i++ {
		got, err := s.Stocks(ctx, store, nil)
		if err != nil || got.TotalItems != 1 {
			t.Errorf("reader saw incomplete snapshot: %+v %v", got, err)
			break
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTodayPriceChangesSnapshot(t *testing.T) {
	s := authTestStorage(t)
	ctx := context.Background()
	item := models.TodayPriceChange{StoreID: uuid.New(), ProductID: uuid.New(), Price: "690.123456789", IsPromo: true}
	for _, p := range []json.Number{"2000", item.Price} {
		current := item
		current.Price = p
		if err := s.ReplaceTodayPriceChanges(ctx, []models.TodayPriceChange{current}); err != nil {
			t.Fatal(err)
		}
	}

	reader, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback()
	var snapshot string
	if err := reader.QueryRowContext(ctx, "SELECT pg_current_snapshot()::text").Scan(&snapshot); err != nil {
		t.Fatal(err)
	}

	replacement := item
	replacement.Price = "2000"
	if err := s.ReplaceTodayPriceChanges(ctx, []models.TodayPriceChange{replacement}); err != nil {
		t.Fatal(err)
	}
	var previousPrice json.Number
	if err := reader.QueryRowContext(ctx, "SELECT price FROM today_price_changes").Scan(&previousPrice); err != nil || previousPrice != item.Price {
		t.Fatalf("previous snapshot price = %s, error = %v", previousPrice, err)
	}
	if err := reader.Commit(); err != nil {
		t.Fatal(err)
	}

	invalid := item
	invalid.Price = "invalid"
	if err := s.ReplaceTodayPriceChanges(ctx, []models.TodayPriceChange{invalid}); err == nil {
		t.Fatal("invalid price accepted")
	}
	// The row changed by the replacement got a new arrival time; the same price keeps it.
	var arrived time.Time
	if err := s.db.QueryRowContext(ctx, "SELECT arrived_at FROM today_price_changes").Scan(&arrived); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceTodayPriceChanges(ctx, []models.TodayPriceChange{replacement}); err != nil {
		t.Fatal(err)
	}
	var same bool
	if err := s.db.QueryRowContext(ctx, "SELECT arrived_at=$1 FROM today_price_changes", arrived).Scan(&same); err != nil || !same {
		t.Fatalf("unchanged row restamped: %v", err)
	}
	var got models.TodayPriceChange
	var count int
	err = s.db.QueryRowContext(ctx, "SELECT store_id, product_id, price, is_promo, count(*) OVER () FROM today_price_changes").
		Scan(&got.StoreID, &got.ProductID, &got.Price, &got.IsPromo, &count)
	if err != nil || got != replacement || count != 1 {
		t.Fatalf("snapshot = %+v, count = %d, error = %v", got, count, err)
	}

	if err := s.ReplaceTodayPriceChanges(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM today_price_changes").Scan(&count); err != nil || count != 0 {
		t.Fatalf("empty snapshot count = %d, error = %v", count, err)
	}
}
