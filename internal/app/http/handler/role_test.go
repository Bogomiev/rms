package handler

import (
	"encoding/json"
	"net/http/httptest"
	"rms/internal/domain/models"
	"testing"
)

func TestAuthResponseRole(t *testing.T) {
	for _, tc := range []struct {
		role  string
		admin bool
	}{{"admin", true}, {"seller", false}} {
		for _, action := range []string{"login", "refresh"} {
			t.Run(action+"/"+tc.role, func(t *testing.T) {
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/auth/"+action, nil)
				if action == "login" {
					respLoginOK(w, r, "session", &models.User{IsAdmin: tc.admin}, "csrf")
				} else {
					respRefreshOK(w, r, "session", "csrf", tc.admin)
				}
				var body struct {
					Role string `json:"role"`
					Data struct {
						User roleData `json:"user"`
					} `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Role != "" {
					t.Fatalf("unexpected top-level role: %q", body.Role)
				}
				if body.Data.User.Role != tc.role {
					t.Fatalf("role = %q, want %q", body.Data.User.Role, tc.role)
				}
			})
		}
	}
}
