package user

import (
	"context"
	"rms/internal/domain/models"
	lib "rms/internal/lib/password"
	"rms/internal/lib/validation"
	"rms/internal/services"
)

type ManagementStorage interface {
	UserByID(context.Context, int64) (models.UserDetails, error)
	AddUserWithPosition(context.Context, models.UserInput) (int64, error)
	SaveUser(context.Context, int64, models.UserInput) error
	DeleteUser(context.Context, int64) error
}

func (s *User) UserByID(ctx context.Context, id int64) (models.UserDetails, error) {
	if id <= 0 {
		return models.UserDetails{}, services.ErrInvalid
	}
	v, e := s.usrStorage.UserByID(ctx, id)
	return v, services.PersonnelError(e)
}
func prepareUser(in models.UserInput, create bool) (models.UserInput, error) {
	if !services.ValidName(in.Name) || in.Position != nil && *in.Position <= 0 {
		return in, services.ErrInvalid
	}
	pass := in.Password
	if !create && pass == "" {
		pass = "unchanged-password"
	}
	if len(validation.NewUser(in.UserToken, in.Name, pass, in.IsAdmin)) > 0 {
		return in, services.ErrInvalid
	}
	if in.Password != "" {
		hash, e := lib.HashPassword(in.Password)
		if e != nil {
			return in, e
		}
		in.Password = hash
	}
	return in, nil
}
func (s *User) AddUser(ctx context.Context, in models.UserInput) (int64, error) {
	in, e := prepareUser(in, true)
	if e != nil {
		return 0, e
	}
	id, e := s.usrStorage.AddUserWithPosition(ctx, in)
	return id, services.PersonnelError(e)
}
func (s *User) SaveUser(ctx context.Context, id int64, in models.UserInput) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	in, e := prepareUser(in, false)
	if e != nil {
		return e
	}
	return services.PersonnelError(s.usrStorage.SaveUser(ctx, id, in))
}
func (s *User) DeleteUser(ctx context.Context, id int64) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.usrStorage.DeleteUser(ctx, id))
}
