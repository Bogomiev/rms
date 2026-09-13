package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"rms/internal/domain/models"
	password "rms/internal/lib/password"
	"rms/internal/services"
	"rms/internal/storage"
	"rms/internal/token"
)

type usersStub struct {
	user models.User
	err  error
	ctx  context.Context
}

func (s *usersStub) User(ctx context.Context, _ string) (models.User, error) {
	s.ctx = ctx
	return s.user, s.err
}

type sessionsStub struct {
	session *models.Session
	err     error
	ctx     context.Context
}

func (s *sessionsStub) CreateSession(ctx context.Context, ss *models.Session) (*models.Session, error) {
	s.ctx = ctx
	s.session = ss
	return ss, s.err
}
func (s *sessionsStub) GetSession(ctx context.Context, _ string) (*models.Session, error) {
	s.ctx = ctx
	return s.session, s.err
}
func (s *sessionsStub) DeleteSession(ctx context.Context, _ string) error { s.ctx = ctx; return s.err }

type tokensStub struct {
	durations []time.Duration
	err       error
}

func (s *tokensStub) CreateToken(id int64, userToken string, admin bool, ttl time.Duration, purpose string) (string, *token.UserClaims, error) {
	s.durations = append(s.durations, ttl)
	claims, err := token.NewUserClaims(id, userToken, admin, ttl, purpose)
	return fmt.Sprintf("token-%d", len(s.durations)), claims, err
}
func (s *tokensStub) VerifyToken(string) (*token.UserClaims, error) {
	return &token.UserClaims{Purpose: token.Refresh}, s.err
}
func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestLoginUsesInjectedTokensAndRequestContext(t *testing.T) {
	hash, err := password.HashPassword("password")
	if err != nil {
		t.Fatal(err)
	}
	users := &usersStub{user: models.User{ID: 1, UserToken: "a@example.com", Password: hash}}
	sessions := &sessionsStub{}
	tokens := &tokensStub{}
	a := New(Config{TokenTTL: time.Minute, RefreshTokenTTL: time.Hour}, Dependencies{Logger: testLogger(), Users: users, Sessions: sessions, Tokens: tokens})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, access, refresh, _, err := a.Login(ctx, "a@example.com", "password")
	if err != nil {
		t.Fatal(err)
	}
	if access != "token-1" || refresh != "token-2" || id != sessions.session.ID {
		t.Fatal("injected tokens not returned and persisted")
	}
	if len(tokens.durations) != 2 || tokens.durations[0] != time.Minute || tokens.durations[1] != time.Hour {
		t.Fatal("incorrect token configuration")
	}
	if users.ctx != ctx || sessions.ctx != ctx {
		t.Fatal("storage context replaced")
	}
	cancel()
	if sessions.ctx.Err() != context.Canceled {
		t.Fatal("cancellation not propagated")
	}
}

func TestAuthClassifiesErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, tc := range []struct {
		name        string
		input, want error
	}{
		{"missing user", storage.ErrUserNotFound, services.ErrInvalidCredentials},
		{"database", dbErr, dbErr},
		{"canceled", context.Canceled, context.Canceled},
	} {
		t.Run("login/"+tc.name, func(t *testing.T) {
			a := New(Config{}, Dependencies{Logger: testLogger(), Users: &usersStub{err: fmt.Errorf("storage: %w", tc.input)}})
			_, _, _, _, err := a.Login(context.Background(), "user_token", "password")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name                       string
		tokenErr, sessionErr, want error
	}{
		{"invalid token", errors.New("invalid signature"), nil, services.ErrInvalidSession},
		{"missing session", nil, storage.ErrSessionNotFound, services.ErrInvalidSession},
		{"database", nil, dbErr, dbErr},
	} {
		t.Run("refresh/"+tc.name, func(t *testing.T) {
			a := New(Config{}, Dependencies{Logger: testLogger(), Tokens: &tokensStub{err: tc.tokenErr}, Sessions: &sessionsStub{err: tc.sessionErr}})
			_, _, _, err := a.RefreshToken(context.Background(), "refresh")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRefreshChecksSessionOwnerAndExpiration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		userID  int64
		expiry  time.Time
		revoked bool
		valid   bool
	}{
		{"expired", 42, time.Now().Add(-time.Second), false, false},
		{"wrong user", 43, time.Now().Add(time.Hour), false, false},
		{"revoked", 42, time.Now().Add(time.Hour), true, false},
		{"user_token changed", 42, time.Now().Add(time.Hour), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			maker := token.NewJWTMaker("test-secret")
			refresh, claims, err := maker.CreateToken(42, "old@example.com", false, time.Hour, token.Refresh)
			if err != nil {
				t.Fatal(err)
			}
			sessions := &sessionsStub{session: &models.Session{RefreshToken: refresh, ID: claims.ID, UserID: tc.userID, ExpiresAt: tc.expiry, IsRevoked: tc.revoked}}
			a := New(Config{TokenTTL: time.Minute, RefreshTokenTTL: time.Hour}, Dependencies{Logger: testLogger(), Tokens: maker, Sessions: sessions})
			_, _, _, err = a.RefreshToken(context.Background(), refresh)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				if sessions.session.UserID != 42 {
					t.Fatal("rotated session lost user ID")
				}
			} else if !errors.Is(err, services.ErrInvalidSession) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
