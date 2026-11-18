package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"rms/internal/domain/models"
	srvc "rms/internal/services"
)

// LoginCodeTTL is how long a QR login code is valid.
const LoginCodeTTL = 2 * time.Minute

// loginCodeLength is the length of a code: 32 random bytes in unpadded base64url.
const loginCodeLength = 43

// A login code lets a signed-in user open the service on a phone by scanning a QR code.
// It works once, lives LoginCodeTTL and is kept only in memory, so a restart drops unused codes.
// Codes are stored by SHA-256, and a user has at most one: a new code replaces the previous one.
type loginCode struct {
	userID  int64
	store   string
	expires time.Time
}

type loginCodes struct {
	mu     sync.Mutex
	byHash map[[32]byte]loginCode
	byUser map[int64][32]byte
}

func (c *loginCodes) purge(now time.Time) {
	for hash, v := range c.byHash {
		if !now.Before(v.expires) {
			delete(c.byHash, hash)
			if c.byUser[v.userID] == hash {
				delete(c.byUser, v.userID)
			}
		}
	}
}

func (c *loginCodes) issue(userID int64, store string, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(code))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byHash == nil {
		c.byHash, c.byUser = map[[32]byte]loginCode{}, map[int64][32]byte{}
	}
	c.purge(now)
	if old, ok := c.byUser[userID]; ok {
		delete(c.byHash, old)
	}
	c.byHash[hash] = loginCode{userID: userID, store: store, expires: now.Add(LoginCodeTTL)}
	c.byUser[userID] = hash
	return code, nil
}

// take removes the code and returns it when it is still valid.
func (c *loginCodes) take(code string, now time.Time) (loginCode, bool) {
	if len(code) != loginCodeLength {
		return loginCode{}, false
	}
	hash := sha256.Sum256([]byte(code))
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.byHash[hash]
	if !ok {
		return loginCode{}, false
	}
	delete(c.byHash, hash)
	if c.byUser[v.userID] == hash {
		delete(c.byUser, v.userID)
	}
	return v, now.Before(v.expires)
}

// IssueLoginCode creates a one-time code for the signed-in user; store is the store the phone opens.
func (a *Auth) IssueLoginCode(_ context.Context, userID int64, store string) (string, time.Duration, error) {
	if userID <= 0 {
		return "", 0, srvc.ErrInvalid
	}
	code, err := a.codes.issue(userID, store, time.Now())
	if err != nil {
		return "", 0, fmt.Errorf("Auth.IssueLoginCode: %w", err)
	}
	return code, LoginCodeTTL, nil
}

// LoginByCode spends a one-time code and opens a new session for its user, like Login does.
// It returns the session, tokens, user and the store chosen when the code was created.
func (a *Auth) LoginByCode(ctx context.Context, code string) (string, string, string, *models.User, string, error) {
	const op = "Auth.LoginByCode"
	v, ok := a.codes.take(code, time.Now())
	if !ok {
		return "", "", "", nil, "", fmt.Errorf("%s: %w", op, ErrInvalidCredentials)
	}
	details, err := a.authStorage.UserByID(ctx, v.userID)
	if err != nil {
		return "", "", "", nil, "", fmt.Errorf("%s: %w", op, ErrInvalidCredentials)
	}
	if details.LoginBlockedUntil != nil && details.LoginBlockedUntil.After(time.Now()) {
		return "", "", "", nil, "", &srvc.LoginBlockedError{Until: *details.LoginBlockedUntil}
	}
	user := details.User
	session, access, refresh, err := a.startSession(ctx, user, details.HasAdminAccess())
	if err != nil {
		return "", "", "", nil, "", fmt.Errorf("%s: %w", op, err)
	}
	return session, access, refresh, &user, v.store, nil
}
