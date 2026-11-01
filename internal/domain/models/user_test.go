package models

import "testing"

func TestHasAdminAccess(t *testing.T) {
	position := int64(2)
	for _, tc := range []struct {
		name string
		user UserDetails
		want bool
	}{
		{"no position", UserDetails{Roles: []Role{{Name: "order"}}}, true},
		{"no roles", UserDetails{User: User{Position: &position}, Roles: []Role{}}, true},
		{"nil roles", UserDetails{User: User{Position: &position}}, true},
		{"seller", UserDetails{User: User{Position: &position}, Roles: []Role{{Name: "order"}}}, false},
		{"admin flag", UserDetails{User: User{Position: &position, IsAdmin: true}, Roles: []Role{{Name: "order"}}}, true},
		{"admin role", UserDetails{User: User{Position: &position}, Roles: []Role{{Name: "admin"}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.HasAdminAccess(); got != tc.want {
				t.Fatalf("HasAdminAccess()=%v, want %v", got, tc.want)
			}
		})
	}
}
