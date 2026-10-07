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
	PriceFrom            *string     `json:"price_from"`
	PricePromoFrom       *string     `json:"price_promo_from"`
	PricePromoTo         *string     `json:"price_promo_to"`
	PricePromo           json.Number `json:"price_promo"`
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
	SoldYesterdayQuantity     json.Number `json:"sold_yesterday_quantity"`
	SoldWeekQuantity          json.Number `json:"sold_week_quantity"`
	ReceiptsYesterdayQuantity json.Number `json:"receipts_yesterday_quantity"`
	ReceiptsWeekQuantity      json.Number `json:"receipts_week_quantity"`
	StockDays                 int64       `json:"stock_days"`
	Receipts                  []Receipt   `json:"receipts"`
}

// OneCProductInfo contains live product data returned by GetProductInfo.
type OneCProductInfo struct {
	Receipts []Receipt      `json:"receipts"`
	Sales    []ProductSales `json:"sales"`
}

type ProductSales struct {
	StoreID                   uuid.UUID   `json:"store_id"`
	ProductID                 uuid.UUID   `json:"product_id"`
	SoldYesterdayQuantity     json.Number `json:"sold_yesterday_quantity"`
	SoldWeekQuantity          json.Number `json:"sold_week_quantity"`
	ReceiptsYesterdayQuantity json.Number `json:"receipts_yesterday_quantity"`
	ReceiptsWeekQuantity      json.Number `json:"receipts_week_quantity"`
}
