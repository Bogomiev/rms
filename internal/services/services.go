package services

import (
	"errors"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrForbidden          = errors.New("insufficient permissions")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidSession     = errors.New("invalid session")
)
