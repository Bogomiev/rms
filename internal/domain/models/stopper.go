package models

import "github.com/google/uuid"

// Stopper is a shelf label put on promotional products in stores.
// Stopper is a sticker from the reference book. Days is its duration: setting it on a position makes
// the end date the start date plus Days, unless the position ends earlier; nil keeps an open period.
type Stopper struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Days      *int   `json:"days"`
	Positions int64  `json:"positions"`
}
type StopperInput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Days  *int   `json:"days"`
}

// StopperPositionKey identifies a product of a 1C promotion document;
// DocID is nil for a product added by hand.
type StopperPositionKey struct {
	DocID     *uuid.UUID `json:"doc_id"`
	ProductID uuid.UUID  `json:"product_id"`
}

// StopperPosition is a promotional or manually added product with its stopper period.
// To is nil for a period without an end. Stores counts stores whose promotional price type
// the 1C document covers (0 for manual positions). Prices follow the requested store order.
type StopperPosition struct {
	DocID       *uuid.UUID     `json:"doc_id"`
	ProductID   uuid.UUID      `json:"product_id"`
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	ThermalMode bool           `json:"thermal_mode"`
	StopperID   *int64         `json:"stopper_id"`
	From        string         `json:"from"`
	To          *string        `json:"to"`
	Received    string         `json:"received"`
	Manual      bool           `json:"manual"`
	Stores      int64          `json:"stores"`
	Prices      []StopperPrice `json:"prices"`
}
type StopperPrice struct {
	StoreID    uuid.UUID `json:"store_id"`
	Price      *string   `json:"price"`
	PromoPrice *string   `json:"promo_price"`
}

// StopperPositionFilter selects positions whose period overlaps From..To (dates, YYYY-MM-DD).
type StopperPositionFilter struct {
	From   string
	To     string
	Stores []uuid.UUID
}
type StopperAssignInput struct {
	Positions []StopperPositionKey `json:"positions"`
	StopperID *int64               `json:"stopper_id"`
}
type StopperPeriodInput struct {
	Positions []StopperPositionKey `json:"positions"`
	From      string               `json:"from"`
	To        *string              `json:"to"`
}
type StopperManualInput struct {
	ProductID uuid.UUID `json:"product_id"`
	StopperID int64     `json:"stopper_id"`
	From      string    `json:"from"`
}

// StopperProduct is a catalog product for manual addition; Added is set when it already has a manual position.
type StopperProduct struct {
	ID    uuid.UUID `json:"id"`
	Code  string    `json:"code"`
	Name  string    `json:"name"`
	Added bool      `json:"added"`
}

// StopperTask is the store screen of a «Стопперы» task: stoppers to check on the shelf
// and stoppers whose period ended within the last two weeks and must be removed.
type StopperTask struct {
	TaskID       int64             `json:"task_id"`
	Status       string            `json:"status"`
	Start        string            `json:"start"`
	StoreID      uuid.UUID         `json:"store_id"`
	StoreCode    string            `json:"store_code"`
	StoreName    string            `json:"store_name"`
	StoreAddress string            `json:"store_address"`
	Items        []StopperTaskItem `json:"items"`
}

// StopperTaskItem is a position of the task. Remove marks an ended stopper; Result is
// "", "ok" or "bad" for a stopper to check and "" or "removed" for one to remove.
type StopperTaskItem struct {
	DocID        *uuid.UUID `json:"doc_id"`
	ProductID    uuid.UUID  `json:"product_id"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	ThermalMode  bool       `json:"thermal_mode"`
	Unit         string     `json:"unit"`
	Stopper      string     `json:"stopper"`
	StopperColor string     `json:"stopper_color"`
	From         string     `json:"from"`
	To           *string    `json:"to"`
	Remove       bool       `json:"remove"`
	Stock        *string    `json:"stock"`
	Price        *string    `json:"price"`
	PromoPrice   *string    `json:"promo_price"`
	Barcode      string     `json:"barcode"`
	Result       string     `json:"result"`
}

// StopperTaskCheckInput saves a store result for a position; an empty Result clears it.
type StopperTaskCheckInput struct {
	DocID     *uuid.UUID `json:"doc_id"`
	ProductID uuid.UUID  `json:"product_id"`
	Result    string     `json:"result"`
}
