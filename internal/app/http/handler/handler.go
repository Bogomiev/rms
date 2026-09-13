package handler

import (
	"context"
	"rms/internal/domain/models"
	"rms/internal/token"
	"time"
)

type AuthService interface {
	Login(context.Context, string, string) (string, string, string, *models.User, error)
	Logout(context.Context, string) error
	RefreshToken(context.Context, string) (string, string, string, error)
}

type UserService interface {
	RegisterNewUser(context.Context, string, string, string, bool) (int64, error)
	UserTokenValid(context.Context, string) (bool, error)
	UserList(context.Context, models.UserPage) ([]*models.User, error)
}

type TokenVerifier interface {
	VerifyToken(string) (*token.UserClaims, error)
}

type Handler struct {
	authService    AuthService
	userService    UserService
	productService ProductService
	storeService   StoreService
	accessTTL      time.Duration
	refreshTTL     time.Duration
}

func NewHandler(auth AuthService, user UserService) *Handler {
	return &Handler{authService: auth, userService: user, accessTTL: 15 * time.Minute, refreshTTL: 2160 * time.Hour}
}

type ProductService interface {
	Products(context.Context) ([]models.Product, error)
}
type StoreService interface {
	Stores(context.Context) (models.StoresResponse, error)
}

func (h *Handler) Configure(products ProductService, stores StoreService, accessTTL, refreshTTL time.Duration) {
	h.productService = products
	h.storeService = stores
	if accessTTL > 0 {
		h.accessTTL = accessTTL
	}
	if refreshTTL > 0 {
		h.refreshTTL = refreshTTL
	}
}
