package models

import "encoding/json"

// Task service identifiers shared by validation and task consumers.
const (
	TaskServiceCashCollection = "cash_collection"
	TaskServicePricePrint     = "price_print"
	TaskServiceWithoutService = "without_service"
)

// TaskFilter contains the indexed filters supported by the task API.
type TaskFilter struct {
	Limit, Offset              int
	ID, TypeID, TaskID, UserID int64
	StoreID                    string
	Name                       string
	PeriodFrom, PeriodTo       string
}
type TaskTypeInput struct {
	CustomFields json.RawMessage `json:"custom_fields"`
	Name         string          `json:"name"`
	ShortName    string          `json:"short_name"`
	Service      string          `json:"service"`
	Icon         []byte          `json:"icon"`
	Color        string          `json:"color"`
	ColorDark    string          `json:"color_dark"`
	IsActive     bool            `json:"is_active"`
	SortOrder    int16           `json:"sort_order"`
}
type TaskSchedulingInput struct {
	TaskTypeID int16    `json:"task_type_id"`
	Frequency  int16    `json:"frequency"`
	PeriodFrom string   `json:"period_from"`
	PeriodTo   string   `json:"period_to"`
	Days       []int16  `json:"days"`
	Time       string   `json:"time,omitempty"`
	IsActive   bool     `json:"is_active"`
	Stores     []string `json:"stores"`
}
type TaskInput struct {
	TypeID       int16           `json:"type_id"`
	StoreID      string          `json:"store_id"`
	Start        string          `json:"start"`
	Status       string          `json:"status"`
	CustomFields json.RawMessage `json:"custom_fields"`
}
type TaskHistoryInput struct {
	Period string `json:"period"`
	TaskID int64  `json:"task_id"`
	UserID int64  `json:"user_id"`
	Status string `json:"status"`
}
