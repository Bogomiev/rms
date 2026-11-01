package handler

import (
	"net/http"
	"rms/internal/lib/validation"
	"rms/internal/services"
	"time"

	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"

	"github.com/go-chi/render"
)

type tokenData struct {
	SessionID string `json:"sessionId"`
}

type loginData struct {
	tokenData
	User loginUserData `json:"user"`
}

type roleData struct {
	Role string `json:"role"`
}

type loginUserData struct {
	Roles []models.Role `json:"roles"`
	userData
	roleData
}

type refreshData struct {
	tokenData
	User loginUserData `json:"user"`
}

type errorResponse struct {
	resp.Response
}

type loginResponse struct {
	CSRFToken string `json:"csrfToken"`
	resp.Response
	Data loginData `json:"data,omitempty"`
}

type logoutResponse struct {
	resp.Response
}

type refreshResponse struct {
	CSRFToken string `json:"csrfToken"`
	resp.Response
	Data refreshData `json:"data,omitempty"`
}

type userData struct {
	SellerSelectionAvailable bool `json:"seller_selection_available"`
	StoreSelectionAvailable  bool `json:"store_selection_available"`
	StoreSelectionRequired   bool `json:"store_selection_required"`

	LoginBlockedUntil *time.Time `json:"login_blocked_until"`
	Position          *int64     `json:"position"`
	PositionName      *string    `json:"position_name"`
	ID                int64      `json:"id"`
	UserToken         string     `json:"user_token"`
	Name              string     `json:"name"`
	IsAdmin           bool       `json:"isAdmin"`
}

type userResponse struct {
	resp.Response
	Data userData `json:"data,omitempty"`
}

type usersResponse struct {
	resp.Response
	Data []*userData `json:"data"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.user.Login"

	var u loginUserReq
	if !decodeRequest(w, r, &u) {
		return
	}

	errMessages := validation.Credentials(u.UserToken, u.Password)

	if len(errMessages) > 0 {
		resp.HttpResponseError(w, r, http.StatusBadRequest, 1, errMessages)
		return
	}

	sessionId, accessToken, refreshToken, user, err := h.authService.Login(
		r.Context(), u.UserToken, u.Password)

	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	claims, err := h.tokens.VerifyToken(accessToken)
	if err != nil || claims == nil || claims.CSRFToken == "" {
		writeServiceError(w, r, services.ErrInvalidSession)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	details, err := h.userService.UserByID(r.Context(), user.ID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	user = &details.User
	h.setTokenCookies(w, accessToken, refreshToken)
	respLoginOK(w, r, sessionId, user, claims.CSRFToken, details.Roles)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.user.Logout"

	cookie, cookieErr := r.Cookie("refresh_token")
	if cookieErr != nil || cookie.Value == "" {
		resp.HttpResponseError(w, r, http.StatusUnauthorized, services.ResultSessionInvalid, []string{"refresh token is required"})
		return
	}

	err := h.authService.Logout(r.Context(), cookie.Value)

	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	clearTokenCookies(w)
	respLogoutOK(w, r)
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.user.RefreshToken"

	cookie, cookieErr := r.Cookie("refresh_token")
	if cookieErr != nil || cookie.Value == "" {
		resp.HttpResponseError(w, r, http.StatusUnauthorized, services.ResultSessionInvalid, []string{"refresh token is required"})
		return
	}

	sessionId, accessToken, refreshToken, err := h.authService.RefreshToken(r.Context(), cookie.Value)

	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	claims, err := h.tokens.VerifyToken(accessToken)
	if err != nil || claims == nil || claims.CSRFToken == "" {
		writeServiceError(w, r, services.ErrInvalidSession)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	details, err := h.userService.UserByID(r.Context(), claims.UserID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	h.setTokenCookies(w, accessToken, refreshToken)
	respRefreshOK(w, r, sessionId, claims.CSRFToken, details)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var in models.UserInput
	if !decodeRequest(w, r, &in) {
		return
	}
	messages := validation.NewUser(in.UserToken, in.Name, in.Password, in.IsAdmin)
	if in.Position != nil && *in.Position <= 0 {
		messages = append(messages, "position must be positive")
	}
	if len(messages) > 0 {
		resp.HttpResponseError(w, r, http.StatusBadRequest, 1, messages)
		return
	}
	id, err := h.userService.AddUser(r.Context(), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	u, err := h.userService.UserByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, userResponse{Response: resp.OK(), Data: toUserData(u.User)})
}

func (h *Handler) UserList(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.user.UserList"

	page, err := userPage(r)
	if err != nil {
		resp.HttpResponseError(w, r, http.StatusBadRequest, 1, []string{"limit must be 1..100 and offset must be nonnegative"})
		return
	}
	users, err := h.userService.UserList(r.Context(), page)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	var usrs []*userData

	for i := range users {
		u := users[i]

		data := toUserData(*u)
		usrs = append(usrs, &data)
	}

	respUsersOK(w, r, usrs)
}

func respLoginOK(w http.ResponseWriter, r *http.Request,
	sessionId string, user *models.User, csrf string, roles []models.Role) {

	if roles == nil {
		roles = []models.Role{}
	}
	render.JSON(w, r, loginResponse{
		CSRFToken: csrf,
		Response:  resp.OK(),
		Data: loginData{
			tokenData: tokenData{
				SessionID: sessionId,
			},
			User: loginUserData{
				Roles:    roles,
				roleData: roleData{Role: userRole((models.UserDetails{User: *user, Roles: roles}).HasAdminAccess())},
				userData: toUserData(*user),
			},
		},
	})
}

func respRefreshOK(w http.ResponseWriter, r *http.Request,
	sessionId string, csrf string, details models.UserDetails) {
	u := loginUserData{userData: toUserData(details.User), roleData: roleData{Role: userRole(details.HasAdminAccess())}, Roles: details.Roles}
	if u.Roles == nil {
		u.Roles = []models.Role{}
	}

	render.JSON(w, r, refreshResponse{
		CSRFToken: csrf,
		Response:  resp.OK(),
		Data: refreshData{
			tokenData: tokenData{SessionID: sessionId},
			User:      u,
		},
	})
}

func respLogoutOK(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, logoutResponse{
		Response: resp.OK(),
	})
}

func respUsersOK(w http.ResponseWriter, r *http.Request, users []*userData) {
	var data []*userData
	if users == nil {
		data = []*userData{}
	} else {
		data = users
	}

	render.JSON(w, r, usersResponse{
		Response: resp.OK(),
		Data:     data,
	})
}

func userRole(isAdmin bool) string {
	if isAdmin {
		return "admin"
	}
	return "seller"
}
