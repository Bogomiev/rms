package models

import "time"

type User struct {
	SellerSelectionAvailable bool `json:"seller_selection_available"`
	StoreSelectionAvailable  bool `json:"store_selection_available"`
	StoreSelectionRequired   bool `json:"store_selection_required"`

	LoginBlockedUntil *time.Time
	Position          *int64
	PositionName      *string
	ID                int64
	Name              string
	UserToken         string
	Password          string
	IsAdmin           bool
}

type UserDetails struct {
	User
	Roles []Role
}

// HasAdminAccess includes users whose permissions have not been configured.
// It does not change the stored is_admin flag or position.
func (u UserDetails) HasAdminAccess() bool {
	if u.IsAdmin || u.Position == nil || len(u.Roles) == 0 {
		return true
	}
	for _, role := range u.Roles {
		if role.Name == "admin" {
			return true
		}
	}
	return false
}
