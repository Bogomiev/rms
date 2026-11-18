package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/token"
)

func TestLoginCodeWorksOnce(t *testing.T) {
	sessions := &sessionsStub{}
	users := &usersStub{user: models.User{ID: 42, UserToken: "seller", Name: "Продавец"}}
	a := New(Config{TokenTTL: time.Minute, RefreshTokenTTL: time.Hour}, Dependencies{Logger: testLogger(), Users: users, Sessions: sessions, Tokens: token.NewJWTMaker("test-key")})
	code, ttl, err := a.IssueLoginCode(context.Background(), 42, "018e166f-4220-11f1-95f7-00155d1a2b22")
	if err != nil || len(code) != loginCodeLength || ttl != LoginCodeTTL {
		t.Fatal(code, ttl, err)
	}
	session, access, refresh, user, store, err := a.LoginByCode(context.Background(), code)
	if err != nil || session == "" || access == "" || refresh == "" || user.ID != 42 || store != "018e166f-4220-11f1-95f7-00155d1a2b22" || sessions.session == nil || sessions.session.UserID != 42 {
		t.Fatal(session, user, store, err)
	}
	if _, _, _, _, _, err = a.LoginByCode(context.Background(), code); !errors.Is(err, services.ErrInvalidCredentials) {
		t.Fatal("second use:", err)
	}
}

func TestLoginCodeRejectsExpiredReplacedAndBlocked(t *testing.T) {
	users := &usersStub{user: models.User{ID: 42, UserToken: "seller"}}
	a := New(Config{TokenTTL: time.Minute, RefreshTokenTTL: time.Hour}, Dependencies{Logger: testLogger(), Users: users, Sessions: &sessionsStub{}, Tokens: token.NewJWTMaker("test-key")})
	ctx := context.Background()
	if _, _, _, _, _, err := a.LoginByCode(ctx, "short"); !errors.Is(err, services.ErrInvalidCredentials) {
		t.Fatal("malformed code:", err)
	}
	if _, _, err := a.IssueLoginCode(ctx, 0, ""); !errors.Is(err, services.ErrInvalid) {
		t.Fatal("no user:", err)
	}
	// A new code replaces the previous one of the same user.
	first, _, _ := a.IssueLoginCode(ctx, 42, "")
	second, _, _ := a.IssueLoginCode(ctx, 42, "")
	if _, _, _, _, _, err := a.LoginByCode(ctx, first); !errors.Is(err, services.ErrInvalidCredentials) {
		t.Fatal("replaced code:", err)
	}
	// An expired code is refused and removed.
	a.codes.mu.Lock()
	for hash, v := range a.codes.byHash {
		v.expires = time.Now().Add(-time.Second)
		a.codes.byHash[hash] = v
	}
	a.codes.mu.Unlock()
	if _, _, _, _, _, err := a.LoginByCode(ctx, second); !errors.Is(err, services.ErrInvalidCredentials) {
		t.Fatal("expired code:", err)
	}
	// A blocked user cannot sign in with a code.
	until := time.Now().Add(time.Hour)
	users.user.LoginBlockedUntil = &until
	code, _, _ := a.IssueLoginCode(ctx, 42, "")
	var blocked *services.LoginBlockedError
	if _, _, _, _, _, err := a.LoginByCode(ctx, code); !errors.As(err, &blocked) {
		t.Fatal("blocked user:", err)
	}
}
