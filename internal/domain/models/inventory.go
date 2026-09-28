package models

import (
	"encoding/json"
	"github.com/google/uuid"
)

// Period is a 1C local datetime without a timezone; keep its wall-clock value.
type Price struct {
	Period    string      `json:"period"`
	PriceType uuid.UUID   `json:"price_type"`
	ProductID uuid.UUID   `json:"product_id"`
	Price     json.Number `json:"price"`
}
type Stock struct {
	ProductID uuid.UUID   `json:"product_id"`
	StoreID   uuid.UUID   `json:"store_id"`
	Stock     json.Number `json:"stock"`
}
type Page[T any] struct {
	Page       int `json:"page"`
	PerPage    int `json:"perPage"`
	TotalPages int `json:"totalPages"`
	TotalItems int `json:"totalItems"`
	Items      []T `json:"items"`
}

func SinglePage[T any](items []T) Page[T] {
	if items == nil {
		items = []T{}
	}
	pages := 0
	if len(items) > 0 {
		pages = 1
	}
	return Page[T]{Page: 1, PerPage: len(items), TotalPages: pages, TotalItems: len(items), Items: items}
}
