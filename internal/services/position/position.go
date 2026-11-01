package position

import (
	"context"
	"rms/internal/domain/models"
	"rms/internal/services"
)

type Storage interface {
	Positions(context.Context, models.UserPage) ([]models.Position, error)
	Position(context.Context, int64) (models.Position, error)
	AddPosition(context.Context, models.PositionInput) (int64, error)
	SavePosition(context.Context, int64, models.PositionInput) error
	DeletePosition(context.Context, int64) error
}
type Position struct{ storage Storage }

func New(storage Storage) *Position { return &Position{storage: storage} }
func (s *Position) PositionList(ctx context.Context, page models.UserPage) ([]models.Position, error) {
	if !page.Valid() {
		return nil, services.ErrInvalid
	}
	v, e := s.storage.Positions(ctx, page)
	return v, services.PersonnelError(e)
}
func (s *Position) PositionByID(ctx context.Context, id int64) (models.Position, error) {
	if id <= 0 {
		return models.Position{}, services.ErrInvalid
	}
	v, e := s.storage.Position(ctx, id)
	return v, services.PersonnelError(e)
}
func validate(in models.PositionInput) error {
	if !services.ValidName(in.Name) {
		return services.ErrInvalid
	}
	seen := map[int64]bool{}
	for _, id := range in.RoleIDs {
		if id <= 0 || seen[id] {
			return services.ErrInvalid
		}
		seen[id] = true
	}
	return nil
}
func (s *Position) AddPosition(ctx context.Context, in models.PositionInput) (int64, error) {
	if e := validate(in); e != nil {
		return 0, e
	}
	id, e := s.storage.AddPosition(ctx, in)
	return id, services.PersonnelError(e)
}
func (s *Position) SavePosition(ctx context.Context, id int64, in models.PositionInput) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	if e := validate(in); e != nil {
		return e
	}
	return services.PersonnelError(s.storage.SavePosition(ctx, id, in))
}
func (s *Position) DeletePosition(ctx context.Context, id int64) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.DeletePosition(ctx, id))
}
