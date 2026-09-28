package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"rms/internal/domain/models"
	"sync"
	"time"
)

type Client interface {
	Do(context.Context, string, string, io.Reader) (*http.Response, error)
}
type Service struct {
	client  Client
	mu      sync.Mutex
	cached  models.MarketplaceResponse
	expires time.Time
	loading chan struct{}
}

func New(c Client) *Service { return &Service{client: c} }
func (s *Service) fetch(ctx context.Context) (models.MarketplaceResponse, error) {
	result := models.MarketplaceResponse{Messages: []string{}}
	resp, err := s.client.Do(ctx, http.MethodGet, "v1/GetMarketplaces", nil)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	var raw struct {
		Code *int                       `json:"code"`
		Mess json.RawMessage            `json:"mess"`
		Data map[string]json.RawMessage `json:"data"`
	}
	dec := json.NewDecoder(resp.Body)
	if err = dec.Decode(&raw); err != nil {
		return result, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return result, fmt.Errorf("invalid marketplaces trailing data")
	}
	if raw.Code == nil {
		return result, fmt.Errorf("missing marketplaces code")
	}
	result.ResultCode = *raw.Code
	result.Data = raw.Data
	if len(raw.Mess) > 0 && string(raw.Mess) != "null" {
		var message string
		if json.Unmarshal(raw.Mess, &message) == nil {
			if message != "" {
				result.Messages = append(result.Messages, message)
			}
		} else if err = json.Unmarshal(raw.Mess, &result.Messages); err != nil {
			return result, fmt.Errorf("invalid marketplaces messages: %w", err)
		}
	}
	return result, nil
}

// Cache only successful reference data for one minute. Prices and stocks are never cached.
// Concurrent callers share a refresh; waiting callers may cancel independently.
func (s *Service) MarketplaceData(ctx context.Context) (models.MarketplaceResponse, error) {
	for {
		if err := ctx.Err(); err != nil {
			return models.MarketplaceResponse{}, err
		}
		s.mu.Lock()
		if time.Now().Before(s.expires) {
			result := cloneResponse(s.cached)
			s.mu.Unlock()
			return result, nil
		}
		if pending := s.loading; pending != nil {
			s.mu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				return models.MarketplaceResponse{}, ctx.Err()
			}
		}
		s.loading = make(chan struct{})
		s.mu.Unlock()
		result, err := s.fetch(ctx)
		s.mu.Lock()
		if err == nil && result.ResultCode == 0 && result.Data != nil {
			s.cached = cloneResponse(result)
			s.expires = time.Now().Add(15 * time.Minute)
		}
		close(s.loading)
		s.loading = nil
		s.mu.Unlock()
		return result, err
	}
}
func cloneResponse(src models.MarketplaceResponse) models.MarketplaceResponse {
	dst := src
	dst.Messages = append([]string{}, src.Messages...)
	if src.Data != nil {
		dst.Data = make(map[string]json.RawMessage, len(src.Data))
		for key, value := range src.Data {
			dst.Data[key] = append(json.RawMessage(nil), value...)
		}
	}
	return dst
}
