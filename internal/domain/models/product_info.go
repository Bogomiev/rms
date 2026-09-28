package models

import (
	"encoding/json"
	"github.com/google/uuid"
)

type ProductFilter struct {
	ID   *uuid.UUID
	Code string
	Name string
}
type ProductValues struct {
	Price                json.Number `json:"price"`
	Stock                json.Number `json:"stock"`
	PriceEshop           json.Number `json:"price_eshop"`
	PriceOzon            json.Number `json:"price_ozon"`
	PriceYandexEats      json.Number `json:"price_yandex_eats"`
	PromoPriceEshop      json.Number `json:"promo_price_eshop"`
	PromoPriceOzon       json.Number `json:"promo_price_ozon"`
	PromoPriceYandexEats json.Number `json:"promo_price_yandex_eats"`
}
type ProductInfo struct {
	Product
	ProductValues
	Receipts []Receipt `json:"receipts"`
}
