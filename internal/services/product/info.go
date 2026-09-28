package product

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"rms/internal/domain/models"
)

func (s *ProductService) ProductInfo(ctx context.Context, store uuid.UUID, f models.ProductFilter) ([]models.ProductInfo, error) {
	products, err := s.info.SearchProducts(ctx, f, 10)
	if err != nil {
		return nil, err
	}
	result := make([]models.ProductInfo, 0, len(products))
	if len(products) == 0 {
		return result, nil
	}
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
	values, err := s.info.ProductValues(ctx, store, ids, types, s.timezone)
	if err != nil {
		return nil, err
	}
	receipts, err := s.Receipts(ctx, store, ids)
	if err != nil {
		return nil, err
	}
	byProduct := make(map[uuid.UUID][]models.Receipt)
	for _, receipt := range receipts {
		if receipt.StoreID == store {
			byProduct[receipt.ProductID] = append(byProduct[receipt.ProductID], receipt)
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
		result = append(result, models.ProductInfo{Product: p, ProductValues: v, Receipts: productReceipts})
	}
	return result, nil
}
