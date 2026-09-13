package storage

import (
	"errors"
)

var (
	ErrNotFound        = errors.New("record not found")
	ErrUserExists      = errors.New("user already exists")
	ErrUserNotFound    = errors.New("user not found")
	ErrSessionNotFound = errors.New("session not found")
)
