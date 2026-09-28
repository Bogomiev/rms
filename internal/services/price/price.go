package price

import (
	"context"
	"github.com/google/uuid"
	"rms/internal/domain/models"
)

type Storage interface {
	Prices(context.Context, uuid.UUID, *uuid.UUID) (models.Page[models.Price], error)
}
type Service struct{ storage Storage }

func New(s Storage) *Service { return &Service{storage: s} }
func (s *Service) Prices(ctx context.Context, store uuid.UUID, products *uuid.UUID) (models.Page[models.Price], error) {
	return s.storage.Prices(ctx, store, products)
}
