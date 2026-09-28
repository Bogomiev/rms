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

// Validate every page before replacing any data. Inconsistent pagination fails closed.
func fetchPages[T any](ctx context.Context, s *Service, method string) ([]T, error) {
	items := []T{}
	total, pages, perPage := -1, -1, -1
	for page := 1; ; page++ {
		resp, err := s.Do(ctx, http.MethodGet, "v1/"+method+"?page="+strconv.Itoa(page), nil)
		if err != nil {
			return nil, err
		}
		var result models.Page[T]
		decoder := json.NewDecoder(resp.Body)
		err = decoder.Decode(&result)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = fmt.Errorf("unexpected trailing data")
			}
		}
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode %s page %d: %w", method, page, err)
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
		if err != nil || v.PriceType == uuid.Nil || v.ProductID == uuid.Nil || v.Price == "" {
			return fmt.Errorf("invalid price at index %d", i)
		}
		key := t.Format("2006-01-02T15:04:05.999999999") + v.PriceType.String() + v.ProductID.String()
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
	items, err := fetchPages[models.Stock](ctx, s, "GetStocks")
	if err != nil {
		return err
	}
	seen := map[[2]uuid.UUID]bool{}
	for i, v := range items {
		if v.StoreID == uuid.Nil || v.ProductID == uuid.Nil || v.Stock == "" {
			return fmt.Errorf("invalid stock at index %d", i)
		}
		key := [2]uuid.UUID{v.StoreID, v.ProductID}
		if seen[key] {
			return fmt.Errorf("duplicate stock at index %d", i)
		}
		seen[key] = true
	}
	return s.inventory.ReplaceStocks(ctx, items)
}
