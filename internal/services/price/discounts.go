package price

import (
	"context"
	"rms/internal/domain/models"
)

type DiscountStorage interface {
	Discounts(context.Context, string) ([]models.Discount, error)
}
type DiscountService struct {
	storage  DiscountStorage
	timezone string
}

func NewDiscounts(s DiscountStorage, timezone string) *DiscountService {
	return &DiscountService{storage: s, timezone: timezone}
}
func (s *DiscountService) Discounts(ctx context.Context) ([]models.Discount, error) {
	return s.storage.Discounts(ctx, s.timezone)
}
