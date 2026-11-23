package models

import "github.com/google/uuid"

// Price tag sources: why a product is in a «Печать ценников» task.
const (
	PriceTagRevaluation = "revaluation"
	PriceTagPromoStart  = "promo_start"
	PriceTagPromoEnd    = "promo_end"
	PriceTagStopperBad  = "stopper_bad"
	PriceTagManual      = "manual"
)

// Units of measure on a price tag: шт for goods sold by the piece, 100 г or 1 кг for weighed goods.
const (
	PriceTagUnitPiece = "pc"
	PriceTagUnit100g  = "100g"
	PriceTagUnitKg    = "kg"
)

// PriceTagTask is the store screen of a «Печать ценников» task.
type PriceTagTask struct {
	TaskID       int64          `json:"task_id"`
	Status       string         `json:"status"`
	Start        string         `json:"start"`
	StoreID      uuid.UUID      `json:"store_id"`
	StoreCode    string         `json:"store_code"`
	StoreName    string         `json:"store_name"`
	StoreAddress string         `json:"store_address"`
	Items        []PriceTagItem `json:"items"`
}

// PriceTagItem is a product to print tags for. Prices are the store prices of the day: the regular price
// and, while a promotion runs, the promotional price and its period. Pending is true while the tag is to be
// printed: never printed or printed before actual_at, the time the latest reason appeared.
// last_printed_at is the last print of a tag for the product in the store made before actual_at.
// unit is the unit of measure the price is printed for.
type PriceTagItem struct {
	ID            int64      `json:"id"`
	ProductID     uuid.UUID  `json:"product_id"`
	Source        string     `json:"source"`
	DocID         *uuid.UUID `json:"doc_id"`
	TagType       string     `json:"tag_type"`
	Quantity      int        `json:"quantity"`
	Pending       bool       `json:"pending"`
	ActualAt      string     `json:"actual_at"`
	PrintedAt     *string    `json:"printed_at"`
	LastPrintedAt *string    `json:"last_printed_at"`
	Unit          string     `json:"unit"`
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	IsWeight      bool       `json:"is_weight"`
	Barcode       string     `json:"barcode"`
	Price         *string    `json:"price"`
	PriceFrom     *string    `json:"price_from"`
	PromoPrice    *string    `json:"promo_price"`
	PromoFrom     *string    `json:"promo_from"`
	PromoTo       *string    `json:"promo_to"`
}

// PriceTagItemInput changes the tag type (regular, promo — crossed out regular price, card — loyalty
// card price), the number of tags and the unit of measure (pc, 100g, kg) of a position.
type PriceTagItemInput struct {
	TagType  string `json:"tag_type"`
	Quantity int    `json:"quantity"`
	Unit     string `json:"unit"`
}

// PriceTagAddInput adds a product to the task by hand; a product already in the task gets one more tag.
type PriceTagAddInput struct {
	ProductID uuid.UUID `json:"product_id"`
}

// PriceTagPrintedInput marks positions as printed.
type PriceTagPrintedInput struct {
	IDs []int64 `json:"ids"`
}

// PriceTagProductInput puts a product of a store into its standing «Печать ценников» task.
type PriceTagProductInput struct {
	StoreID   uuid.UUID `json:"store_id"`
	ProductID uuid.UUID `json:"product_id"`
}

// PriceTagProductState is the price tag of a product in a store for the product card: the open
// «Печать ценников» task (task_id is null without one), the position of the product in it (item_id is
// null when the product is not there), the last time the store printed a tag for the product and the
// stopper of the product today (null without one).
type PriceTagProductState struct {
	Stopper       *PriceTagStopper `json:"stopper"`
	TaskID        *int64           `json:"task_id"`
	ItemID        *int64           `json:"item_id"`
	Pending       bool             `json:"pending"`
	Source        *string          `json:"source"`
	Quantity      int              `json:"quantity"`
	ActualAt      *string          `json:"actual_at"`
	LastPrintedAt *string          `json:"last_printed_at"`
}

// PriceTagStopper is a stopper on the shelf: its name and color.
type PriceTagStopper struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// PriceTagProduct is a catalogue product found for adding to the task; added is true while the product
// is in the task and still to be printed.
type PriceTagProduct struct {
	ID      uuid.UUID `json:"id"`
	Code    string    `json:"code"`
	Name    string    `json:"name"`
	Barcode string    `json:"barcode"`
	Added   bool      `json:"added"`
}
