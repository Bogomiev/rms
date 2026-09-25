// Package onec provides access to the 1C PAPI service.
package onec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

type Config struct {
	BaseURL string        `yaml:"base_url" env:"RMS_ONEC_BASE_URL" env-default:"http://dev.1c.ikorniysrv.ru:85/eshop/hs/PAPI/"`
	Timeout time.Duration `yaml:"timeout" env:"RMS_ONEC_TIMEOUT" env-default:"30s"`
}

func (c Config) Validate() error {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("onec.base_url must be an HTTP(S) URL without credentials, query or fragment")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("onec.timeout must be positive")
	}
	return nil
}

type Stores interface {
	AddStore(context.Context, *models.Store) (*models.Store, error)
	UpdateStore(context.Context, *models.Store) (*models.Store, error)
}

type Products interface {
	UpsertProducts(context.Context, []models.Product) error
}

type Service struct {
	baseURL  string
	client   *http.Client
	stores   Stores
	products Products
}

func New(cfg Config, stores Stores, products Products) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Service{baseURL: strings.TrimRight(cfg.BaseURL, "/") + "/", client: &http.Client{Timeout: cfg.Timeout}, stores: stores, products: products}, nil
}

// Do sends a request relative to the configured PAPI base URL. The caller closes the response body.
func (s *Service) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	endpoint, err := url.Parse(path)
	if err != nil || endpoint.IsAbs() || endpoint.Host != "" || strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("1C endpoint must be a relative path")
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create 1C request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request 1C: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("1C returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (s *Service) GetStores(ctx context.Context) ([]models.Store, error) {
	resp, err := s.Do(ctx, http.MethodGet, "v1/GetStores", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result models.StoresResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode 1C stores: %w", err)
	}
	if result.Items == nil {
		return nil, fmt.Errorf("1C stores response must contain an items array")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after 1C stores")
	}
	// Validate the whole response before starting writes.
	for i, v := range result.Items {
		if v.ID == uuid.Nil || utf8.RuneCountInString(v.Address) > 250 {
			return nil, fmt.Errorf("invalid 1C store at index %d", i)
		}
	}
	return result.Items, nil
}

func (s *Service) SyncStores(ctx context.Context) error {
	stores, err := s.GetStores(ctx)
	if err != nil {
		return err
	}
	for _, v := range stores {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := s.stores.UpdateStore(ctx, &v)
		if errors.Is(err, storage.ErrNotFound) {
			_, err = s.stores.AddStore(ctx, &v)
		}
		if err != nil {
			return fmt.Errorf("save 1C store %s: %w", v.ID, err)
		}
	}
	return nil
}
