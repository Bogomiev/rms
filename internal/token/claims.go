package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const Access = "access"
const Refresh = "refresh"

type UserClaims struct {
	CSRFToken string `json:"csrfToken,omitempty"`
	SessionID string `json:"sid,omitempty"`
	Purpose   string `json:"purpose"`
	UserID    int64  `json:"id"`
	UserToken string `json:"user_token"`
	IsAdmin   bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

func NewUserClaims(id int64, userToken string, isAdmin bool, duration time.Duration, purpose string, sessionID ...string) (*UserClaims, error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("error generating token ID: %w", err)
	}

	csrf := ""
	sid := ""
	if purpose == Access {
		csrfID, err := uuid.NewRandom()
		if err != nil {
			return nil, err
		}
		csrf = csrfID.String()
		if len(sessionID) > 0 {
			sid = sessionID[0]
		}
	}
	return &UserClaims{
		CSRFToken: csrf, SessionID: sid,
		Purpose:   purpose,
		UserToken: userToken,
		UserID:    id,
		IsAdmin:   isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID.String(),
			Subject:   userToken,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
		},
	}, nil
}
