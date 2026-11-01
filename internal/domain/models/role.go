package models

import "time"

type Role struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type Position struct {
	SellerSelectionAvailable bool `json:"seller_selection_available"`
	StoreSelectionAvailable  bool `json:"store_selection_available"`
	StoreSelectionRequired   bool `json:"store_selection_required"`

	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Roles []Role `json:"roles"`
}
type UserInput struct {
	LoginBlockedUntil *time.Time `json:"login_blocked_until"`
	Name              string     `json:"name"`
	UserToken         string     `json:"user_token"`
	Password          string     `json:"password"`
	IsAdmin           bool       `json:"is_admin"`
	Position          *int64     `json:"position"`
}
type PositionInput struct {
	SellerSelectionAvailable bool `json:"seller_selection_available"`
	StoreSelectionAvailable  bool `json:"store_selection_available"`
	StoreSelectionRequired   bool `json:"store_selection_required"`

	Name    string  `json:"name"`
	RoleIDs []int64 `json:"role_ids"`
}

type StoreSelectionSettings struct {
	SellerSelectionAvailable bool `json:"seller_selection_available"`
	StoreSelectionAvailable  bool `json:"store_selection_available"`
	StoreSelectionRequired   bool `json:"store_selection_required"`
}
