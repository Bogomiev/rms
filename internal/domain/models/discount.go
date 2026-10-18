package models

import "encoding/xml"

type Discounts struct {
	XMLName xml.Name   `xml:"discounts"`
	Items   []Discount `xml:"discount"`
}
type Discount struct {
	StoreID             string `xml:"sale_point_tag,attr"`
	ProductID           string `xml:"product_id,attr"`
	Price               string `xml:"price_card,attr"`
	Start               string `xml:"start,attr"`
	End                 string `xml:"end,attr"`
	DocID               string `xml:"promo_id,attr"`
	ApplyOtherDiscounts bool   `xml:"apply_other_discounts,attr"`
	WriteOffBonus       bool   `xml:"write_off_bonus,attr"`
}
