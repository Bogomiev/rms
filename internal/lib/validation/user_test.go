package validation

import "testing"

func TestPINAndAdminPassword(t *testing.T) {
	for _, tc := range []struct {
		password     string
		admin, valid bool
	}{
		{"12345", false, true}, {"01234", false, true}, {"12345", true, false}, {"short", false, false}, {"1234", false, false}, {"password", true, true},
	} {
		if got := len(NewUser("token", "User", tc.password, tc.admin)) == 0; got != tc.valid {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}
