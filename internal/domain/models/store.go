package models

import (
	"github.com/google/uuid"
	"time"
)

type Store struct {
	PriceType uuid.UUID `json:"price_type" db:"price_type"`
	CreatedAt time.Time `json:"-" db:"created_at"`
	UpdatedAt time.Time `json:"-" db:"updated_at"`
	ID        uuid.UUID `json:"id" db:"id"`
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	Address   string    `json:"address" db:"address"`
}
type StoresResponse struct {
	Page       int     `json:"page"`
	PerPage    int     `json:"perPage"`
	TotalPages int     `json:"totalPages"`
	TotalItems int     `json:"totalItems"`
	Items      []Store `json:"items"`
}
