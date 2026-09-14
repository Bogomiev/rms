package services

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrForbidden          = errors.New("insufficient permissions")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidSession     = errors.New("invalid session")
)

// Result codes are stable across RMS and the browser client.
const ResultLoginBlocked = 1001
const ResultCSRFInvalid = 1002
const ResultSessionInvalid = 1003

type LoginBlockedError struct{ Until time.Time }

func (e *LoginBlockedError) Error() string {
	return fmt.Sprintf("Пользователь заблокирован до %s", e.Until.Format(time.RFC3339))
}
