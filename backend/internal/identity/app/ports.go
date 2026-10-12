// Package app contains the identity use cases and the ports they depend on.
package app

import (
	"context"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/domain"
)

// RefreshToken is the persisted (hashed) form of an opaque refresh token.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	// RotatedAt is set when the token was revoked by rotation (as opposed to
	// logout or a security revocation). ReplacedBy is the id of its successor.
	RotatedAt  *time.Time
	ReplacedBy *string
}

// UserRepository persists users.
type UserRepository interface {
	// Create stores u, filling ID and timestamps. Returns domain.ErrEmailTaken
	// when the email is already registered.
	Create(ctx context.Context, u *domain.User) error
	// GetByID returns domain.ErrUserNotFound when absent.
	GetByID(ctx context.Context, id string) (domain.User, error)
	// GetByEmail expects a normalized email; returns domain.ErrUserNotFound.
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	List(ctx context.Context) ([]domain.User, error)
	Count(ctx context.Context) (int, error)
	// Mutate loads the user and the current number of active admins under a
	// lock, runs fn, and persists the (possibly modified) user atomically.
	// If fn returns an error nothing is persisted.
	Mutate(ctx context.Context, id string, fn func(u *domain.User, activeAdmins int) error) (domain.User, error)
}

// RefreshTokenRepository persists hashed refresh tokens.
type RefreshTokenRepository interface {
	Create(ctx context.Context, t RefreshToken) error
	// GetByHash returns domain.ErrInvalidToken when absent.
	GetByHash(ctx context.Context, hash string) (RefreshToken, error)
	// GetByID returns domain.ErrInvalidToken when absent.
	GetByID(ctx context.Context, id string) (RefreshToken, error)
	// Rotate atomically revokes the token id as rotated at the given time and
	// stores next as its successor. It returns false, storing nothing, when
	// the token was already revoked (for example by a concurrent rotation).
	Rotate(ctx context.Context, id string, next RefreshToken, at time.Time) (bool, error)
	// Revoke marks the token revoked and reports whether this call did it
	// (false when it was already revoked).
	Revoke(ctx context.Context, id string, at time.Time) (bool, error)
	RevokeAllForUser(ctx context.Context, userID string, at time.Time) error
}

// PasswordHasher hashes and verifies passwords.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) bool
}

// AccessClaims is what an access token proves about its bearer.
type AccessClaims struct {
	UserID string
	Role   domain.Role
}

// TokenIssuer creates and verifies short-lived access tokens.
type TokenIssuer interface {
	IssueAccess(u domain.User, now time.Time) (token string, expiresAt time.Time, err error)
	ParseAccess(token string, now time.Time) (AccessClaims, error)
}

// LoginLimiter throttles repeated failed logins for a key.
type LoginLimiter interface {
	// Allow reports whether another attempt is permitted for key.
	Allow(key string) bool
	// Fail records a failed attempt.
	Fail(key string)
	// Reset clears the record after a successful login.
	Reset(key string)
}
