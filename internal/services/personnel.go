package services

import (
	"errors"
	"rms/internal/storage"
	"strings"
	"unicode/utf8"
)

func PersonnelError(err error) error {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, storage.ErrConflict):
		return ErrConflict
	case errors.Is(err, storage.ErrInvalid):
		return ErrInvalid
	}
	return err
}
func ValidName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 255
}
