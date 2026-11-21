package price

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"rms/internal/domain/models"
)

type Storage interface {
	Prices(context.Context, uuid.UUID, *uuid.UUID) (models.Page[models.Price], error)
	ReplaceTodayPriceChanges(context.Context, []models.TodayPriceChange) error
}
type TodayPriceChangesSource interface {
	GetTodayPriceChanges(context.Context) ([]models.TodayPriceChange, error)
}

type Service struct{ storage Storage }

func New(s Storage) *Service { return &Service{storage: s} }
func (s *Service) Prices(ctx context.Context, store uuid.UUID, products *uuid.UUID) (models.Page[models.Price], error) {
	return s.storage.Prices(ctx, store, products)
}

func (s *Service) SyncTodayPriceChanges(ctx context.Context, source TodayPriceChangesSource) error {
	items, err := source.GetTodayPriceChanges(ctx)
	if err != nil {
		return fmt.Errorf("get today's price changes: %w", err)
	}
	for i, item := range items {
		if item.StoreID == uuid.Nil || item.ProductID == uuid.Nil || item.Price == "" {
			return fmt.Errorf("invalid price change at index %d", i)
		}
	}
	return s.storage.ReplaceTodayPriceChanges(ctx, items)
}
