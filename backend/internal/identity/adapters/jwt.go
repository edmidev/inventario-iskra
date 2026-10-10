package adapters

import (
	"errors"
	"fmt"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/golang-jwt/jwt/v5"
)

// JWTIssuer issues and verifies HS256 access tokens.
type JWTIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTIssuer builds an issuer. The caller is responsible for secret length.
func NewJWTIssuer(secret string, ttl time.Duration) (*JWTIssuer, error) {
	if secret == "" {
		return nil, errors.New("jwt: empty secret")
	}
	return &JWTIssuer{secret: []byte(secret), ttl: ttl}, nil
}

type accessClaims struct {
	Role domain.Role `json:"role"`
	jwt.RegisteredClaims
}

// IssueAccess implements app.TokenIssuer.
func (j *JWTIssuer) IssueAccess(u domain.User, now time.Time) (string, time.Time, error) {
	exp := now.Add(j.ttl)
	claims := accessClaims{
		Role: u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign: %w", err)
	}
	return signed, exp, nil
}

// ParseAccess implements app.TokenIssuer.
func (j *JWTIssuer) ParseAccess(token string, now time.Time) (app.AccessClaims, error) {
	var c accessClaims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithExpirationRequired(),
	)
	if err != nil || c.Subject == "" || !c.Role.Valid() {
		return app.AccessClaims{}, domain.ErrInvalidToken
	}
	return app.AccessClaims{UserID: c.Subject, Role: c.Role}, nil
}
