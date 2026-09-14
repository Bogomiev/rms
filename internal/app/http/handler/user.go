package handler

import (
	"net/http"
	"rms/internal/lib/validation"
	"rms/internal/services"

	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"

	"github.com/go-chi/render"
)

type tokenData struct {
	SessionID string `json:"sessionId"`
}

type loginData struct {
	tokenData
	User userData `json:"user"`
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
	Data tokenData `json:"data,omitempty"`
}

type userData struct {
	ID        int64  `json:"id"`
	UserToken string `json:"user_token"`
	Name      string `json:"name"`
	IsAdmin   bool   `json:"isAdmin"`
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
	h.setTokenCookies(w, accessToken, refreshToken)
	respLoginOK(w, r, sessionId, user, claims.CSRFToken)
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
	h.setTokenCookies(w, accessToken, refreshToken)
	respRefreshOK(w, r, sessionId, claims.CSRFToken)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.user.CreateUser"

	var u userReq
	if !decodeRequest(w, r, &u) {
		return
	}

	errMessages := validation.NewUser(u.UserToken, u.Name, u.Password, u.IsAdmin)

	if len(errMessages) > 0 {
		resp.HttpResponseError(w, r, http.StatusBadRequest, 1, errMessages)
		return
	}

	created, err := h.userService.RegisterNewUser(r.Context(), u.UserToken, u.Name, u.Password, u.IsAdmin)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	u.ID = created

	respUserOK(w, r, u)
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

		usrs = append(usrs, &userData{
			ID:        u.ID,
			UserToken: u.UserToken,
			Name:      u.Name,
			IsAdmin:   u.IsAdmin,
		})
	}

	respUsersOK(w, r, usrs)
}

func respLoginOK(w http.ResponseWriter, r *http.Request,
	sessionId string, user *models.User, csrf string) {

	render.JSON(w, r, loginResponse{
		CSRFToken: csrf,
		Response:  resp.OK(),
		Data: loginData{
			tokenData: tokenData{
				SessionID: sessionId,
			},
			User: userData{
				ID:        user.ID,
				UserToken: user.UserToken,
				Name:      user.Name,
				IsAdmin:   user.IsAdmin,
			},
		},
	})
}

func respRefreshOK(w http.ResponseWriter, r *http.Request,
	sessionId string, csrf string) {

	render.JSON(w, r, refreshResponse{
		CSRFToken: csrf,
		Response:  resp.OK(),
		Data: tokenData{
			SessionID: sessionId,
		},
	})
}

func respLogoutOK(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, logoutResponse{
		Response: resp.OK(),
	})
}

func respUserOK(w http.ResponseWriter, r *http.Request, u userReq) {
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, userResponse{
		Response: resp.OK(),
		Data: userData{
			ID:        u.ID,
			UserToken: u.UserToken,
			Name:      u.Name,
			IsAdmin:   u.IsAdmin,
		},
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
