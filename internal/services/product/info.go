package product

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"math/big"
	"rms/internal/domain/models"
)

func (s *ProductService) ProductInfo(ctx context.Context, store uuid.UUID, f models.ProductFilter) (result []models.ProductInfo, err error) {
	stage := "search products"
	defer func() {
		if err != nil {
			s.logger.ErrorContext(ctx, "product_info failed", "stage", stage, "store_id", store, "error", err)
		}
	}()
	products, err := s.info.SearchProducts(ctx, f, 10)
	if err != nil {
		return nil, err
	}
	result = make([]models.ProductInfo, 0, len(products))
	if len(products) == 0 {
		return result, nil
	}
	stage = "marketplace data"
	market, err := s.marketplaces.MarketplaceData(ctx)
	if err != nil {
		return nil, err
	}
	if market.ResultCode != 0 {
		return nil, fmt.Errorf("marketplaces returned code %d", market.ResultCode)
	}
	types := make([]uuid.UUID, 0, 6)
	for _, key := range []string{"Eshop", "Cooper", "Yandex_Eats"} {
		var m struct {
			PriceType uuid.UUID `json:"price_type"`
			Promo     uuid.UUID `json:"price_type_promo"`
		}
		raw, ok := market.Data[key]
		if !ok {
			return nil, fmt.Errorf("missing marketplace %s", key)
		}
		if err = json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("marketplace %s: %w", key, err)
		}
		types = append(types, m.PriceType, m.Promo)
	}
	ids := make([]uuid.UUID, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	stage = "product prices and stocks"
	values, err := s.info.ProductValues(ctx, store, ids, types, s.timezone)
	if err != nil {
		return nil, err
	}
	stage = "1C product info"
	if s.onecInfo == nil {
		return nil, fmt.Errorf("product info service not configured")
	}
	onecInfo, err := s.onecInfo.GetProductInfo(ctx, store, ids)
	if err != nil {
		return nil, err
	}
	byProduct := make(map[uuid.UUID][]models.Receipt)
	for _, receipt := range onecInfo.Receipts {
		if receipt.StoreID == store {
			byProduct[receipt.ProductID] = append(byProduct[receipt.ProductID], receipt)
		}
	}
	salesByProduct := make(map[uuid.UUID]models.ProductSales)
	for _, sale := range onecInfo.Sales {
		if sale.StoreID == store {
			salesByProduct[sale.ProductID] = sale
		}
	}
	for _, p := range products {
		v, ok := values[p.ID]
		if !ok {
			v.Stock = "0"
		}
		productReceipts := byProduct[p.ID]
		if productReceipts == nil {
			productReceipts = []models.Receipt{}
		}
		info := models.ProductInfo{
			Product: p, ProductValues: v, Receipts: productReceipts,
			SoldYesterdayQuantity: "0.000", SoldWeekQuantity: "0.000",
			ReceiptsYesterdayQuantity: "0.000", ReceiptsWeekQuantity: "0.000",
		}
		if sale, ok := salesByProduct[p.ID]; ok {
			if sale.SoldYesterdayQuantity != "" {
				info.SoldYesterdayQuantity = sale.SoldYesterdayQuantity
			}
			if sale.SoldWeekQuantity != "" {
				info.SoldWeekQuantity = sale.SoldWeekQuantity
			}
			if sale.ReceiptsYesterdayQuantity != "" {
				info.ReceiptsYesterdayQuantity = sale.ReceiptsYesterdayQuantity
			}
			if sale.ReceiptsWeekQuantity != "" {
				info.ReceiptsWeekQuantity = sale.ReceiptsWeekQuantity
			}
		}
		info.StockDays = stockDays(info.Stock, info.SoldWeekQuantity)
		result = append(result, info)
	}
	return result, nil
}

// stockDays returns the number of full days covered by the current stock.
// Missing or nonpositive weekly sales have no defined coverage and return zero.
func stockDays(stock, soldWeek json.Number) int64 {
	quantity, ok := new(big.Rat).SetString(string(stock))
	if !ok || quantity.Sign() <= 0 {
		return 0
	}
	sales, ok := new(big.Rat).SetString(string(soldWeek))
	if !ok || sales.Sign() <= 0 {
		return 0
	}
	days := new(big.Rat).Quo(new(big.Rat).Mul(quantity, big.NewRat(7, 1)), sales)
	whole := new(big.Int).Quo(days.Num(), days.Denom())
	if !whole.IsInt64() {
		return 0
	}
	return whole.Int64()
}
