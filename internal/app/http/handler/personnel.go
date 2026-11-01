package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"net/http"
	"rms/internal/domain/models"
	resp "rms/internal/lib/api/response"
	"rms/internal/services"
	"strconv"
)

type UserManagementService interface {
	UserByID(context.Context, int64) (models.UserDetails, error)
	AddUser(context.Context, models.UserInput) (int64, error)
	SaveUser(context.Context, int64, models.UserInput) error
	DeleteUser(context.Context, int64) error
}

func resourceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil || id <= 0 {
		writeServiceError(w, r, services.ErrInvalid)
		return 0, false
	}
	return id, true
}
func resourcePage(w http.ResponseWriter, r *http.Request) (models.UserPage, bool) {
	p, e := userPage(r)
	if e != nil {
		writeServiceError(w, r, services.ErrInvalid)
		return p, false
	}
	return p, true
}
func resourceResponse(w http.ResponseWriter, r *http.Request, data any) {
	render.JSON(w, r, struct {
		resp.Response
		Data any `json:"data"`
	}{resp.OK(), data})
}
func toUserData(u models.User) userData {
	return userData{SellerSelectionAvailable: u.SellerSelectionAvailable, StoreSelectionAvailable: u.StoreSelectionAvailable, StoreSelectionRequired: u.StoreSelectionRequired, LoginBlockedUntil: u.LoginBlockedUntil, ID: u.ID, Name: u.Name, UserToken: u.UserToken, IsAdmin: u.IsAdmin, Position: u.Position, PositionName: u.PositionName}
}
func (h *Handler) UserByID(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	u, e := h.userService.UserByID(r.Context(), id)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, struct {
		userData
		Roles []models.Role `json:"roles"`
	}{toUserData(u.User), u.Roles})
}
func (h *Handler) SaveUser(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var in models.UserInput
	if !decodeRequest(w, r, &in) {
		return
	}
	if e := h.userService.SaveUser(r.Context(), id, in); e != nil {
		writeServiceError(w, r, e)
		return
	}
	h.UserByID(w, r)
}
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	if e := h.userService.DeleteUser(r.Context(), id); e != nil {
		writeServiceError(w, r, e)
		return
	}
	render.JSON(w, r, resp.OK())
}

type RoleService interface {
	RoleList(context.Context, models.UserPage) ([]models.Role, error)
	RoleByID(context.Context, int64) (models.Role, error)
	AddRole(context.Context, string) (int64, error)
	SaveRole(context.Context, int64, string) error
	DeleteRole(context.Context, int64) error
}

func (h *Handler) ConfigureRoles(s RoleService) { h.roles = s }
func (h *Handler) RoleList(w http.ResponseWriter, r *http.Request) {
	p, ok := resourcePage(w, r)
	if !ok {
		return
	}
	v, e := h.roles.RoleList(r.Context(), p)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
func (h *Handler) RoleByID(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	v, e := h.roles.RoleByID(r.Context(), id)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var in models.Role
	if !decodeRequest(w, r, &in) {
		return
	}
	id, e := h.roles.AddRole(r.Context(), in.Name)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	v, e := h.roles.RoleByID(r.Context(), id)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	render.Status(r, http.StatusCreated)
	resourceResponse(w, r, v)
}
func (h *Handler) SaveRole(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var in models.Role
	if !decodeRequest(w, r, &in) {
		return
	}
	if e := h.roles.SaveRole(r.Context(), id, in.Name); e != nil {
		writeServiceError(w, r, e)
		return
	}
	h.RoleByID(w, r)
}
func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	if e := h.roles.DeleteRole(r.Context(), id); e != nil {
		writeServiceError(w, r, e)
		return
	}
	render.JSON(w, r, resp.OK())
}

type PositionService interface {
	PositionList(context.Context, models.UserPage) ([]models.Position, error)
	PositionByID(context.Context, int64) (models.Position, error)
	AddPosition(context.Context, models.PositionInput) (int64, error)
	SavePosition(context.Context, int64, models.PositionInput) error
	DeletePosition(context.Context, int64) error
}

func (h *Handler) ConfigurePositions(s PositionService) { h.positions = s }
func (h *Handler) PositionList(w http.ResponseWriter, r *http.Request) {
	p, ok := resourcePage(w, r)
	if !ok {
		return
	}
	v, e := h.positions.PositionList(r.Context(), p)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
func (h *Handler) PositionByID(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	v, e := h.positions.PositionByID(r.Context(), id)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	resourceResponse(w, r, v)
}
func (h *Handler) CreatePosition(w http.ResponseWriter, r *http.Request) {
	var in models.PositionInput
	if !decodeRequest(w, r, &in) {
		return
	}
	id, e := h.positions.AddPosition(r.Context(), in)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	v, e := h.positions.PositionByID(r.Context(), id)
	if e != nil {
		writeServiceError(w, r, e)
		return
	}
	render.Status(r, http.StatusCreated)
	resourceResponse(w, r, v)
}
func (h *Handler) SavePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var in models.PositionInput
	if !decodeRequest(w, r, &in) {
		return
	}
	if e := h.positions.SavePosition(r.Context(), id, in); e != nil {
		writeServiceError(w, r, e)
		return
	}
	h.PositionByID(w, r)
}
func (h *Handler) DeletePosition(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	if e := h.positions.DeletePosition(r.Context(), id); e != nil {
		writeServiceError(w, r, e)
		return
	}
	render.JSON(w, r, resp.OK())
}
