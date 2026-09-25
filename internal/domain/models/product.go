package models

import (
	"github.com/google/uuid"
	"time"
)

type BarcodeInfo struct {
	Barcode string  `json:"barcode" db:"barcode"`
	Unit    string  `json:"unit" db:"unit"`
	Ratio   float64 `json:"ratio" db:"ratio"`
	IsBase  bool    `json:"isBase" db:"is_base"`
}
type Product struct {
	ID            uuid.UUID     `json:"uid" db:"id"`
	ParentID      uuid.UUID     `json:"parent_id" db:"parent_id"`
	Marked        bool          `json:"marked" db:"marked"`
	CreatedAt     time.Time     `json:"-" db:"created_at"`
	UpdatedAt     time.Time     `json:"-" db:"updated_at"`
	Code          string        `json:"code" db:"code"`
	Name          string        `json:"name" db:"name"`
	MarkingType   string        `json:"markingType" db:"marking_type"`
	IsWeight      bool          `json:"isWeight" db:"is_weight"`
	IsThermalMode bool          `json:"isThermalMode" db:"is_thermal_mode"`
	Barcodes      []BarcodeInfo `json:"barcodes" db:"-"`
}
