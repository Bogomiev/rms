package onec

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"rms/internal/domain/models"
	"strconv"
	"time"
)

type Inventory interface {
	ReplacePrices(context.Context, []models.Price) error
	ReplaceStocks(context.Context, []models.Stock) error
}

func (s *Service) ConfigureInventory(inventory Inventory) { s.inventory = inventory }

type storeStocks struct {
	StoreID uuid.UUID `json:"store_id"`
	Stocks  []struct {
		ProductID uuid.UUID   `json:"id"`
		Stock     json.Number `json:"stock"`
	} `json:"stocks"`
}

func fetchPage[T any](ctx context.Context, s *Service, path string) (models.Page[T], error) {
	var result models.Page[T]
	resp, err := s.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode 1C response: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return result, fmt.Errorf("unexpected trailing data")
	}
	return result, nil
}

func (s *Service) GetTodayPriceChanges(ctx context.Context) ([]models.TodayPriceChange, error) {
	result, err := fetchPage[models.TodayPriceChange](ctx, s, "v1/GetTodayPriceChanges")
	if err != nil {
		return nil, err
	}
	if result.Items == nil || result.Page != 1 || result.TotalPages < 0 || result.TotalPages > 1 ||
		result.TotalItems != len(result.Items) || result.PerPage < len(result.Items) ||
		(result.TotalItems > 0 && result.TotalPages != 1) {
		return nil, fmt.Errorf("invalid GetTodayPriceChanges snapshot")
	}
	return result.Items, nil
}

// Validate every page before replacing any data. Inconsistent pagination fails closed.
func fetchPages[T any](ctx context.Context, s *Service, method string) ([]T, error) {
	items := []T{}
	total, pages, perPage := -1, -1, -1
	for page := 1; ; page++ {
		result, err := fetchPage[T](ctx, s, "v1/"+method+"?page="+strconv.Itoa(page))
		if err != nil {
			return nil, fmt.Errorf("fetch %s page %d: %w", method, page, err)
		}
		if result.Items == nil || result.Page != page || result.TotalItems < 0 || result.TotalPages < 0 || result.PerPage < 0 {
			return nil, fmt.Errorf("invalid %s pagination", method)
		}
		if page == 1 {
			total, pages, perPage = result.TotalItems, result.TotalPages, result.PerPage
		}
		if result.TotalItems != total || result.TotalPages != pages || result.PerPage != perPage || len(result.Items) > perPage || (total > 0 && (pages < 1 || perPage < 1)) {
			return nil, fmt.Errorf("inconsistent %s pagination", method)
		}
		items = append(items, result.Items...)
		if len(items) > total {
			return nil, fmt.Errorf("too many %s items", method)
		}
		if page >= pages {
			break
		}
		if len(result.Items) == 0 {
			return nil, fmt.Errorf("empty intermediate %s page", method)
		}
	}
	if len(items) != total {
		return nil, fmt.Errorf("incomplete %s snapshot", method)
	}
	return items, nil
}
func (s *Service) SyncPrices(ctx context.Context) error {
	if s.inventory == nil {
		return fmt.Errorf("inventory storage not configured")
	}
	items, err := fetchPages[models.Price](ctx, s, "GetPrices")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, v := range items {
		t, err := time.Parse("2006-01-02T15:04:05", v.Period)
		if err != nil || v.DocID == uuid.Nil || v.PriceType == uuid.Nil || v.ProductID == uuid.Nil || v.Price == "" {
			return fmt.Errorf("invalid price at index %d", i)
		}
		key := t.Format("2006-01-02T15:04:05.999999999") + v.PriceType.String() + v.ProductID.String() + v.DocID.String()
		if seen[key] {
			return fmt.Errorf("duplicate price at index %d", i)
		}
		seen[key] = true
	}
	return s.inventory.ReplacePrices(ctx, items)
}
func (s *Service) SyncStocks(ctx context.Context) error {
	if s.inventory == nil {
		return fmt.Errorf("inventory storage not configured")
	}
	stores, err := fetchPages[storeStocks](ctx, s, "GetStocks")
	if err != nil {
		return err
	}
	seen := map[[2]uuid.UUID]bool{}
	items := []models.Stock{}
	for i, store := range stores {
		if store.StoreID == uuid.Nil || store.Stocks == nil {
			return fmt.Errorf("invalid stock store at index %d: store_id or stocks is missing", i)
		}
		for j, stock := range store.Stocks {
			if stock.ProductID == uuid.Nil || stock.Stock == "" {
				return fmt.Errorf("invalid stock at store index %d, stock index %d", i, j)
			}
			key := [2]uuid.UUID{store.StoreID, stock.ProductID}
			if seen[key] {
				return fmt.Errorf("duplicate stock at store index %d, stock index %d", i, j)
			}
			seen[key] = true
			items = append(items, models.Stock{StoreID: store.StoreID, ProductID: stock.ProductID, Stock: stock.Stock})
		}
	}
	return s.inventory.ReplaceStocks(ctx, items)
}
