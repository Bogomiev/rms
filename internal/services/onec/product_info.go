package onec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"rms/internal/domain/models"
)

// GetProductInfo fetches product receipts and sales for a store without caching them.
func (s *Service) GetProductInfo(ctx context.Context, store uuid.UUID, products []uuid.UUID) (*models.OneCProductInfo, error) {
	if store == uuid.Nil {
		return nil, fmt.Errorf("product info store_id is required")
	}
	if len(products) == 0 {
		return &models.OneCProductInfo{Receipts: []models.Receipt{}, Sales: []models.ProductSales{}}, nil
	}
	ids := make([]string, len(products))
	for i, id := range products {
		if id == uuid.Nil {
			return nil, fmt.Errorf("invalid product info product at index %d", i)
		}
		ids[i] = id.String()
	}
	query := url.Values{"products": {strings.Join(ids, ",")}, "store_id": {store.String()}}
	resp, err := s.Do(ctx, http.MethodGet, "v1/GetProductInfo?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Code *int                    `json:"code"`
		Mess string                  `json:"mess"`
		Data *models.OneCProductInfo `json:"data"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		// 1C may return a successful response without a body when no product data exists.
		if err == io.EOF {
			return &models.OneCProductInfo{Receipts: []models.Receipt{}, Sales: []models.ProductSales{}}, nil
		}
		return nil, fmt.Errorf("decode 1C product info: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("unexpected data after 1C product info")
	}
	if result.Code == nil {
		return nil, fmt.Errorf("missing 1C product info code")
	}
	if *result.Code != 0 {
		return nil, fmt.Errorf("1C product info returned code %d: %s", *result.Code, result.Mess)
	}
	if result.Data == nil {
		return nil, fmt.Errorf("1C product info response must contain a data object")
	}
	for i, receipt := range result.Data.Receipts {
		if _, err := time.Parse("2006-01-02T15:04:05", receipt.Date); err != nil {
			return nil, fmt.Errorf("invalid 1C receipt date at index %d: %w", i, err)
		}
		if receipt.StoreID == uuid.Nil || receipt.ProductID == uuid.Nil {
			return nil, fmt.Errorf("invalid 1C receipt at index %d", i)
		}
	}
	for i, sale := range result.Data.Sales {
		if sale.StoreID == uuid.Nil || sale.ProductID == uuid.Nil {
			return nil, fmt.Errorf("invalid 1C sale at index %d", i)
		}
	}
	if result.Data.Receipts == nil {
		result.Data.Receipts = []models.Receipt{}
	}
	if result.Data.Sales == nil {
		result.Data.Sales = []models.ProductSales{}
	}
	return result.Data, nil
}
