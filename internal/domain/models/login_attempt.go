package models

import "time"

// LoginAttempt is updated while the user's database row is locked.
type LoginAttempt struct {
	User         User
	Failures     int
	BlockedUntil time.Time
}
