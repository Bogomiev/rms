package handler

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"rms/internal/domain/models"
	"rms/internal/token"
	"strings"
	"testing"
)

type personnelUserStub struct {
	serviceStub
	user models.UserDetails
}

func (s personnelUserStub) UserByID(_ context.Context, id int64) (models.UserDetails, error) {
	return s.user, nil
}
func TestUserDetailsAndLoginPosition(t *testing.T) {
	id := int64(7)
	name := "Cashier"
	u := models.User{ID: 1, Name: "User", UserToken: "employee", Password: "secret hash", Position: &id, PositionName: &name}
	stub := personnelUserStub{user: models.UserDetails{User: u, Roles: []models.Role{{ID: 2, Name: "Sales"}}}}
	h := NewHandler(nil, stub)
	router := chi.NewRouter()
	router.Get("/users/{id}", h.UserByID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/users/1", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "password") {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			Position     int64         `json:"position"`
			PositionName string        `json:"position_name"`
			Roles        []models.Role `json:"roles"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Position != id || response.Data.PositionName != name || len(response.Data.Roles) != 1 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	respLoginOK(w, httptest.NewRequest("POST", "/auth/login", nil), "session", &u, "csrf", stub.user.Roles)
	var login struct {
		Data struct {
			User userData `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.Data.User.Position == nil || *login.Data.User.Position != id || login.Data.User.PositionName == nil || *login.Data.User.PositionName != name || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Body.String())
	}
}

func TestSessionResponsesIncludeCurrentPersonnel(t *testing.T) {
	positionID := int64(9)
	positionName := "seller"
	details := models.UserDetails{User: models.User{ID: 3, Name: "Employee", UserToken: "employee", SellerSelectionAvailable: true, StoreSelectionAvailable: true, StoreSelectionRequired: true, Password: "never-return-hash", Position: &positionID, PositionName: &positionName}, Roles: []models.Role{{ID: 2, Name: "order"}}}
	stub := personnelUserStub{serviceStub: serviceStub{call: func(context.Context) error { return nil }}, user: details}
	h := NewHandler(stub, stub)
	h.ConfigureSecurity(verifierStub{claims: &token.UserClaims{UserID: 3, CSRFToken: "csrf"}})
	for _, action := range []string{"login", "refresh", "session"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/auth/"+action, strings.NewReader(`{"user_token":"employee","password":"password"}`))
			req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "refresh"})
			req = req.WithContext(context.WithValue(req.Context(), authKey{}, &token.UserClaims{UserID: 3, CSRFToken: "csrf"}))
			w := httptest.NewRecorder()
			switch action {
			case "login":
				h.Login(w, req)
			case "refresh":
				h.RefreshToken(w, req)
			default:
				h.Session(w, req)
			}
			var body struct {
				Data struct {
					User struct {
						Position     *int64        `json:"position"`
						PositionName *string       `json:"position_name"`
						Roles        []models.Role `json:"roles"`
					} `json:"user"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			u := body.Data.User
			if w.Code != 200 || u.Position == nil || *u.Position != positionID || u.PositionName == nil || *u.PositionName != positionName || len(u.Roles) != 1 || u.Roles[0].Name != "order" || strings.Contains(w.Body.String(), "never-return") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestUserDataStoreSelectionFlags(t *testing.T) {
	for _, required := range []bool{false, true} {
		got := toUserData(models.User{SellerSelectionAvailable: true, StoreSelectionAvailable: true, StoreSelectionRequired: required})
		if !got.SellerSelectionAvailable || !got.StoreSelectionAvailable || got.StoreSelectionRequired != required {
			t.Fatalf("store flags lost: %+v", got)
		}
	}
}
