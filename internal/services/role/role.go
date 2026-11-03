package role

import (
	"context"
	"rms/internal/domain/models"
	"rms/internal/services"
)

type Storage interface {
	Roles(context.Context, models.UserPage) ([]models.Role, error)
	Role(context.Context, int64) (models.Role, error)
	AddRole(context.Context, string) (int64, error)
	SaveRole(context.Context, int64, string) error
	DeleteRole(context.Context, int64) error
}
type Role struct{ storage Storage }

func New(storage Storage) *Role { return &Role{storage: storage} }
func (s *Role) RoleList(ctx context.Context, page models.UserPage) ([]models.Role, error) {
	if !page.Valid() {
		return nil, services.ErrInvalid
	}
	v, e := s.storage.Roles(ctx, page)
	return v, services.PersonnelError(e)
}
func (s *Role) RoleByID(ctx context.Context, id int64) (models.Role, error) {
	if id <= 0 {
		return models.Role{}, services.ErrInvalid
	}
	v, e := s.storage.Role(ctx, id)
	return v, services.PersonnelError(e)
}
func validate(name string) error {
	switch name {
	case "admin", "orders", "products", "documents", "goods-receipts", "goods-distribution", "cash-collection":
		return nil
	default:
		return services.ErrInvalid
	}
}
func (s *Role) AddRole(ctx context.Context, name string) (int64, error) {
	if e := validate(name); e != nil {
		return 0, e
	}
	id, e := s.storage.AddRole(ctx, name)
	return id, services.PersonnelError(e)
}
func (s *Role) SaveRole(ctx context.Context, id int64, name string) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	if e := validate(name); e != nil {
		return e
	}
	return services.PersonnelError(s.storage.SaveRole(ctx, id, name))
}
func (s *Role) DeleteRole(ctx context.Context, id int64) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.DeleteRole(ctx, id))
}
