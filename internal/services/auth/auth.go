package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"rms/internal/domain/models"
	sl "rms/internal/lib/logger/slog"
	lib "rms/internal/lib/password"
	srvc "rms/internal/services"
	"rms/internal/storage"
	"rms/internal/token"
)

type Auth struct {
	log             *slog.Logger
	authStorage     AuthStorage
	tokenMaker      TokenMaker
	sessionStorage  SessionStorage
	tokenTTL        time.Duration
	refreshTokenTTL time.Duration
}

var (
	ErrInvalidCredentials = srvc.ErrInvalidCredentials
)

type AuthStorage interface {
	User(ctx context.Context, userToken string) (models.User, error)
}

type TokenMaker interface {
	CreateToken(int64, string, bool, time.Duration, string) (string, *token.UserClaims, error)
	VerifyToken(string) (*token.UserClaims, error)
}

type Config struct {
	TokenTTL        time.Duration
	RefreshTokenTTL time.Duration
}

type Dependencies struct {
	Logger   *slog.Logger
	Users    AuthStorage
	Sessions SessionStorage
	Tokens   TokenMaker
}

type SessionStorage interface {
	CreateSession(ctx context.Context, ss *models.Session) (*models.Session, error)
	GetSession(ctx context.Context, id string) (*models.Session, error)
	DeleteSession(ctx context.Context, id string) error
}

func New(cfg Config, deps Dependencies) *Auth {
	return &Auth{
		log: deps.Logger, authStorage: deps.Users, sessionStorage: deps.Sessions,
		tokenMaker: deps.Tokens,
		tokenTTL:   cfg.TokenTTL, refreshTokenTTL: cfg.RefreshTokenTTL,
	}
}

// Login checks if user with given credentials exists in the system and returns access + refresh tokens + user data.
//
// If user exists, but password is incorrect, returns error.
// If user doesn't exist, returns error.
func (a *Auth) Login(
	ctx context.Context,
	userToken string,
	password string,
) (string, string, string, *models.User, error) {
	const op = "Auth.Login"

	log := a.log.With(
		slog.String("op", op),
	)

	log.Info("attempting to login user")

	user, err := a.authStorage.User(ctx, userToken)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			log.Warn("user not found", sl.Err(err))

			return "", "", "", nil, fmt.Errorf("%s: %w", op, ErrInvalidCredentials)
		}

		log.Error("failed to get user", sl.Err(err))

		return "", "", "", nil, fmt.Errorf("%s: %w", op, err)
	}

	if err := lib.CheckPassword(password, user.Password); err != nil {
		log.Error("invalid credentials", sl.Err(err))

		return "", "", "", nil, fmt.Errorf("%s: %w", op, ErrInvalidCredentials)
	}

	log.Info("user logged in successfully")

	accessToken, _, err := a.tokenMaker.CreateToken(user.ID, user.UserToken, user.IsAdmin, a.tokenTTL, token.Access)
	if err != nil {
		log.Error("failed to generate token", sl.Err(err))

		return "", "", "", nil, fmt.Errorf("%s: %w", op, err)
	}

	refreshToken, refreshClaims, err := a.tokenMaker.CreateToken(user.ID, user.UserToken, user.IsAdmin, a.refreshTokenTTL, token.Refresh)
	if err != nil {
		log.Error("failed to generate refresh token", sl.Err(err))

		return "", "", "", nil, fmt.Errorf("%s: %w", op, err)
	}

	ss := &models.Session{
		ID:           refreshClaims.RegisteredClaims.ID,
		UserID:       refreshClaims.UserID,
		RefreshToken: refreshToken,
		IsRevoked:    false,
		ExpiresAt:    refreshClaims.RegisteredClaims.ExpiresAt.Time,
	}

	session, err := a.sessionStorage.CreateSession(ctx, ss)
	if err != nil {
		log.Error("error creating session", sl.Err(err))
		return "", "", "", nil, fmt.Errorf("%s: %w", op, err)
	}

	return session.ID, accessToken, refreshToken, &user, nil
}

func (a *Auth) Logout(ctx context.Context, refreshToken string) error {
	const op = "Auth.Logout"

	log := a.log.With(
		slog.String("op", op),
	)

	log.Info("attempting to logout user")

	claims, err := a.tokenMaker.VerifyToken(refreshToken)
	if err != nil || claims == nil || claims.Purpose != token.Refresh {
		log.Error("failed to verify token", sl.Err(err))
		return fmt.Errorf("%s: %w", op, srvc.ErrInvalidSession)
	}

	err = a.sessionStorage.DeleteSession(ctx, claims.RegisteredClaims.ID)
	if err != nil {
		log.Error("failed to deleting session", sl.Err(err))
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// RefreshToken checks if session exists and is not revoked and returns access + refresh tokens.
//
// If refresh token is not verify, returns error.
// If session not exists or invoke, returns error.
func (a *Auth) RefreshToken(
	ctx context.Context,
	refreshToken string,
) (string, string, string, error) {
	const op = "Auth.RefreshToken"

	log := a.log.With(
		slog.String("op", op),
	)

	log.Info("attempting to refresh token")

	claims, err := a.tokenMaker.VerifyToken(refreshToken)
	if err != nil || claims == nil || claims.Purpose != token.Refresh {
		log.Error("failed to verify token", sl.Err(err))
		return "", "", "", fmt.Errorf("%s: %w", op, srvc.ErrInvalidSession)
	}

	session, err := a.sessionStorage.GetSession(ctx, claims.RegisteredClaims.ID)
	if err != nil {
		if errors.Is(err, storage.ErrSessionNotFound) {
			return "", "", "", fmt.Errorf("%s: %w", op, srvc.ErrInvalidSession)
		}
		log.Error("error to getting session", sl.Err(err))
		return "", "", "", fmt.Errorf("%s: %w", op, err)
	}

	if session == nil || session.RefreshToken != refreshToken || session.IsRevoked || session.UserID != claims.UserID || !session.ExpiresAt.After(time.Now()) {
		return "", "", "", srvc.ErrInvalidSession
	}

	accessToken, _, err := a.tokenMaker.CreateToken(claims.UserID, claims.UserToken, claims.IsAdmin, a.tokenTTL, token.Access)
	if err != nil {
		log.Error("failed to generate token", sl.Err(err))
		return "", "", "", fmt.Errorf("%s: %w", op, err)
	}

	refreshToken, refreshClaims, err := a.tokenMaker.CreateToken(claims.UserID, claims.UserToken, claims.IsAdmin, a.refreshTokenTTL, token.Refresh)
	if err != nil {
		log.Error("failed to generate refresh token", sl.Err(err))
		return "", "", "", fmt.Errorf("%s: %w", op, err)
	}

	err = a.sessionStorage.DeleteSession(ctx, session.ID)
	if err != nil {
		log.Error("failed to deleting session", sl.Err(err))
		return "", "", "", fmt.Errorf("%s: %w", op, err)
	}

	ss := &models.Session{
		ID:           refreshClaims.RegisteredClaims.ID,
		UserID:       refreshClaims.UserID,
		RefreshToken: refreshToken,
		IsRevoked:    false,
		ExpiresAt:    refreshClaims.RegisteredClaims.ExpiresAt.Time,
	}

	session, err = a.sessionStorage.CreateSession(ctx, ss)
	if err != nil {
		log.Error("error creating session", sl.Err(err))
		return "", "", "", fmt.Errorf("%s: %w", op, err)
	}

	return session.ID, accessToken, refreshToken, nil
}
