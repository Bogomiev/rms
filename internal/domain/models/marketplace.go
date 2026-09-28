package models

import "encoding/json"

type MarketplaceResponse struct {
	ResultCode int                        `json:"resultCode"`
	Messages   []string                   `json:"messages"`
	Data       map[string]json.RawMessage `json:"data"`
}
