package stock

import (
	"context"
	"github.com/google/uuid"
	"rms/internal/domain/models"
)

type Storage interface {
	Stocks(context.Context, uuid.UUID, []uuid.UUID) (models.Page[models.Stock], error)
}
type Service struct{ storage Storage }

func New(s Storage) *Service { return &Service{storage: s} }
func (s *Service) Stocks(ctx context.Context, store uuid.UUID, products []uuid.UUID) (models.Page[models.Stock], error) {
	return s.storage.Stocks(ctx, store, products)
}
