package onec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/google/uuid"
	"rms/internal/domain/models"
)

const goodsBatchSize = 250

func (s *Service) GetGoods(ctx context.Context) ([]models.Product, error) {
	resp, err := s.Do(ctx, http.MethodGet, "v1/GetGoods", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var products []models.Product
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&products); err != nil {
		return nil, fmt.Errorf("decode 1C goods: %w", err)
	}
	if products == nil {
		return nil, fmt.Errorf("1C goods response must be an array")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after 1C goods")
	}
	seen := make(map[uuid.UUID]struct{}, len(products))
	for i, v := range products {
		if v.ID == uuid.Nil || utf8.RuneCountInString(v.Code) > 11 || utf8.RuneCountInString(v.Name) > 100 {
			return nil, fmt.Errorf("invalid 1C product at index %d", i)
		}
		if _, ok := seen[v.ID]; ok {
			return nil, fmt.Errorf("duplicate 1C product %s", v.ID)
		}
		seen[v.ID] = struct{}{}
	}
	return products, nil
}

func (s *Service) SyncGoods(ctx context.Context) error {
	products, err := s.GetGoods(ctx)
	if err != nil {
		return err
	}
	for start := 0; start < len(products); start += goodsBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(start+goodsBatchSize, len(products))
		if err := s.products.UpsertProducts(ctx, products[start:end]); err != nil {
			return fmt.Errorf("save 1C goods batch at index %d: %w", start, err)
		}
	}
	return nil
}
