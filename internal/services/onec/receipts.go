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

// Receipts fetches product receipts for a store without caching them.
func (s *Service) Receipts(ctx context.Context, store uuid.UUID, products []uuid.UUID) ([]models.Receipt, error) {
	if store == uuid.Nil {
		return nil, fmt.Errorf("receipts store_id is required")
	}
	if len(products) == 0 {
		return []models.Receipt{}, nil
	}
	ids := make([]string, len(products))
	for i, id := range products {
		if id == uuid.Nil {
			return nil, fmt.Errorf("invalid receipts product at index %d", i)
		}
		ids[i] = id.String()
	}
	query := url.Values{"products": {strings.Join(ids, ",")}, "store_id": {store.String()}}
	resp, err := s.Do(ctx, http.MethodGet, "v1/GetProductReceipts?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Code *int             `json:"code"`
		Mess string           `json:"mess"`
		Data []models.Receipt `json:"data"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode 1C receipts: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("unexpected data after 1C receipts")
	}
	if result.Code == nil {
		return nil, fmt.Errorf("missing 1C receipts code")
	}
	if *result.Code != 0 {
		return nil, fmt.Errorf("1C receipts returned code %d: %s", *result.Code, result.Mess)
	}
	if result.Data == nil {
		return nil, fmt.Errorf("1C receipts response must contain a data array")
	}
	for i, receipt := range result.Data {
		if _, err := time.Parse("2006-01-02T15:04:05", receipt.Date); err != nil {
			return nil, fmt.Errorf("invalid 1C receipt date at index %d: %w", i, err)
		}
		if receipt.StoreID == uuid.Nil || receipt.ProductID == uuid.Nil {
			return nil, fmt.Errorf("invalid 1C receipt at index %d", i)
		}
	}
	return result.Data, nil
}
