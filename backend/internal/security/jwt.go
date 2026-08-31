package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenManager JWT 签发与解析。
type TokenManager struct {
	secret        []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewTokenManager(secret string, accessTTLMin, refreshTTLDays int) *TokenManager {
	return &TokenManager{
		secret:     []byte(secret),
		accessTTL:  time.Duration(accessTTLMin) * time.Minute,
		refreshTTL: time.Duration(refreshTTLDays) * 24 * time.Hour,
	}
}

type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	TokenType string `json:"tt"`
	jwt.RegisteredClaims
}

func (m *TokenManager) generate(userID int64, username, role, tokenType string, ttl time.Duration) (string, time.Time, error) {
	exp := time.Now().Add(ttl)
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "quiztrace",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return s, exp, nil
}

func (m *TokenManager) GenerateAccessToken(userID int64, username, role string) (string, time.Time, error) {
	return m.generate(userID, username, role, "access", m.accessTTL)
}

func (m *TokenManager) GenerateRefreshToken(userID int64, username, role string) (string, time.Time, error) {
	return m.generate(userID, username, role, "refresh", m.refreshTTL)
}

func (m *TokenManager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (m *TokenManager) AccessTTL() time.Duration { return m.accessTTL }
