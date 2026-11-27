package models

import (
	"time"

	"github.com/google/uuid"
)

type ReturnStatus string

const (
	ReturnNew       ReturnStatus = "NEW"
	ReturnSent      ReturnStatus = "SENT"
	ReturnProcessed ReturnStatus = "PROCESSED"
	ReturnCancelled ReturnStatus = "CANCELLED"
	ReturnCompleted ReturnStatus = "COMPLETED"
)

func (s ReturnStatus) CanCancel() bool {
	return s == ReturnNew || s == ReturnSent || s == ReturnProcessed
}

func (s ReturnStatus) Valid() bool {
	return s.CanCancel() || s == ReturnCancelled || s == ReturnCompleted
}

type ReturnRequest struct {
	ID        uuid.UUID    `json:"id"`
	Date      time.Time    `json:"date"`
	Number    *string      `json:"number"`
	Manager   int64        `json:"manager"`
	StoreID   uuid.UUID    `json:"store_id"`
	Seller    uuid.UUID    `json:"seller"`
	Status    ReturnStatus `json:"status"`
	ItemCount int          `json:"item_count"`
	Items     []ReturnItem `json:"items"`
}

type ReturnItem struct {
	Line      int16     `json:"line"`
	ProductID uuid.UUID `json:"product_id"`
	Amount    float64   `json:"amount"`
	Marks     []string  `json:"marks,omitempty"`
}

type ReturnCommand struct {
	ID      uuid.UUID    `json:"id"`
	StoreID uuid.UUID    `json:"store_id"`
	Status  ReturnStatus `json:"status,omitempty"`
}

type ReturnResult struct {
	Number *string      `json:"returnNumber"`
	Status ReturnStatus `json:"status"`
}
