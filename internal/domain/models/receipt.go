package models

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Receipt describes a product receipt from 1C.
// Date retains the local datetime supplied by 1C without adding a timezone.
type Receipt struct {
	Number    string      `json:"number"`
	Type      string      `json:"type"`
	Date      string      `json:"date"`
	Supplier  string      `json:"supplier"`
	StoreID   uuid.UUID   `json:"store_id"`
	ProductID uuid.UUID   `json:"product_id"`
	Quantity  json.Number `json:"quantity"`
}
