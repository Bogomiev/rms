package storage

import (
	"errors"
)

var (
	ErrInvalid  = errors.New("invalid input")
	ErrConflict = errors.New("record is referenced or already exists")

	ErrNotFound        = errors.New("record not found")
	ErrUserExists      = errors.New("user already exists")
	ErrUserNotFound    = errors.New("user not found")
	ErrSessionNotFound = errors.New("session not found")
)
