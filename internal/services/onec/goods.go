package onec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"rms/internal/domain/models"

	"github.com/google/uuid"
)

const goodsBatchSize = 250

// goodsProduct maps the 1C identifier without changing the public product JSON format.
type goodsProduct struct {
	models.Product
	UID uuid.UUID `json:"uid"`
}

func (s *Service) GetGoods(ctx context.Context) ([]models.Product, error) {
	resp, err := s.Do(ctx, http.MethodGet, "v1/GetGoods", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var goods []goodsProduct
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&goods); err != nil {
		return nil, fmt.Errorf("decode 1C goods: %w", err)
	}
	if goods == nil {
		return nil, fmt.Errorf("1C goods response must be an array")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after 1C goods")
	}
	products := make([]models.Product, len(goods))
	seen := make(map[uuid.UUID]struct{}, len(goods))
	for i, item := range goods {
		v := item.Product
		v.ID = item.UID
		if v.ID == uuid.Nil {
			return nil, fmt.Errorf("invalid 1C product at index %d: uid is missing or zero UUID", i)
		}
		if length := utf8.RuneCountInString(v.Code); length > 11 {
			return nil, fmt.Errorf("invalid 1C product at index %d (%s): code length %d exceeds 11 characters", i, v.ID, length)
		}
		if length := utf8.RuneCountInString(v.Name); length > 100 {
			return nil, fmt.Errorf("invalid 1C product at index %d (%s): name length %d exceeds 100 characters", i, v.ID, length)
		}
		if _, ok := seen[v.ID]; ok {
			return nil, fmt.Errorf("duplicate 1C product %s", v.ID)
		}
		seen[v.ID] = struct{}{}
		products[i] = v
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
