package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/domain"
)

// Service implements the identity use cases.
type Service struct {
	users      UserRepository
	tokens     RefreshTokenRepository
	hasher     PasswordHasher
	issuer     TokenIssuer
	limiter    LoginLimiter
	refreshTTL time.Duration
	now        func() time.Time

	dummyOnce sync.Once
	dummyHash string
}

// Deps groups the collaborators of Service.
type Deps struct {
	Users      UserRepository
	Tokens     RefreshTokenRepository
	Hasher     PasswordHasher
	Issuer     TokenIssuer
	Limiter    LoginLimiter
	RefreshTTL time.Duration
	Now        func() time.Time // optional
}

// NewService wires a Service.
func NewService(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		users: d.Users, tokens: d.Tokens, hasher: d.Hasher, issuer: d.Issuer,
		limiter: d.Limiter, refreshTTL: d.RefreshTTL, now: now,
	}
}

// Session is the result of a successful login or refresh.
type Session struct {
	User           domain.User
	AccessToken    string
	AccessExpires  time.Time
	RefreshToken   string // opaque; only ever returned here, stored hashed
	RefreshExpires time.Time
}

// Login authenticates by email and password. clientKey identifies the caller
// (typically the client IP) for rate limiting. All credential failures return
// domain.ErrInvalidCredentials without revealing whether the email exists.
func (s *Service) Login(ctx context.Context, email, password, clientKey string) (Session, error) {
	norm, emailErr := domain.NormalizeEmail(email)
	key := clientKey + "|" + norm
	if s.limiter != nil && !s.limiter.Allow(key) {
		return Session{}, domain.ErrTooManyAttempts
	}

	var user domain.User
	var lookupErr error
	if emailErr == nil {
		user, lookupErr = s.users.GetByEmail(ctx, norm)
		if lookupErr != nil && !errors.Is(lookupErr, domain.ErrUserNotFound) {
			return Session{}, lookupErr
		}
	}

	if emailErr != nil || lookupErr != nil {
		// Spend the same hashing effort as a real check.
		s.hasher.Verify(password, s.dummy())
		s.fail(key)
		return Session{}, domain.ErrInvalidCredentials
	}
	ok := s.hasher.Verify(password, user.PasswordHash)
	if !ok || !user.Active {
		s.fail(key)
		return Session{}, domain.ErrInvalidCredentials
	}
	if s.limiter != nil {
		s.limiter.Reset(key)
	}
	return s.newSession(ctx, user)
}

func (s *Service) fail(key string) {
	if s.limiter != nil {
		s.limiter.Fail(key)
	}
}

func (s *Service) dummy() string {
	s.dummyOnce.Do(func() {
		h, err := s.hasher.Hash("dummy-password-for-timing")
		if err == nil {
			s.dummyHash = h
		}
	})
	return s.dummyHash
}

// Refresh rotates a refresh token: the presented token is revoked and a new
// session is issued. Presenting an already-revoked token is treated as reuse
// and revokes every token of that user.
func (s *Service) Refresh(ctx context.Context, rawToken string) (Session, error) {
	if rawToken == "" {
		return Session{}, domain.ErrInvalidToken
	}
	now := s.now()
	rt, err := s.tokens.GetByHash(ctx, hashToken(rawToken))
	if err != nil {
		return Session{}, err
	}
	if rt.RevokedAt != nil {
		if err := s.tokens.RevokeAllForUser(ctx, rt.UserID, now); err != nil {
			return Session{}, err
		}
		return Session{}, domain.ErrInvalidToken
	}
	if !now.Before(rt.ExpiresAt) {
		return Session{}, domain.ErrInvalidToken
	}
	user, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return Session{}, domain.ErrInvalidToken
		}
		return Session{}, err
	}
	if !user.Active {
		return Session{}, domain.ErrInvalidToken
	}
	revoked, err := s.tokens.Revoke(ctx, rt.ID, now)
	if err != nil {
		return Session{}, err
	}
	if !revoked { // lost a race with another use of the same token
		if err := s.tokens.RevokeAllForUser(ctx, rt.UserID, now); err != nil {
			return Session{}, err
		}
		return Session{}, domain.ErrInvalidToken
	}
	return s.newSession(ctx, user)
}

// Logout revokes the presented refresh token. Unknown tokens are ignored so
// logout is idempotent.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	rt, err := s.tokens.GetByHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, domain.ErrInvalidToken) {
			return nil
		}
		return err
	}
	_, err = s.tokens.Revoke(ctx, rt.ID, s.now())
	return err
}

// Me returns the user for an authenticated principal.
func (s *Service) Me(ctx context.Context, id string) (domain.User, error) {
	if !domain.ValidID(id) {
		return domain.User{}, domain.ErrUserNotFound
	}
	return s.users.GetByID(ctx, id)
}

// CreateUserInput holds the fields needed to create a user.
type CreateUserInput struct {
	Name     string
	Email    string
	Password string
	Role     domain.Role
}

// CreateUser registers a new active user.
func (s *Service) CreateUser(ctx context.Context, in CreateUserInput) (domain.User, error) {
	name, err := domain.NormalizeName(in.Name)
	if err != nil {
		return domain.User{}, err
	}
	email, err := domain.NormalizeEmail(in.Email)
	if err != nil {
		return domain.User{}, err
	}
	if !in.Role.Valid() {
		return domain.User{}, domain.ErrInvalidInput
	}
	if err := domain.ValidatePassword(in.Password); err != nil {
		return domain.User{}, err
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	u := domain.User{Name: name, Email: email, PasswordHash: hash, Role: in.Role, Active: true}
	if err := s.users.Create(ctx, &u); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// ListUsers returns all users.
func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.users.List(ctx)
}

// UpdateUserInput holds optional changes; nil fields are left untouched.
type UpdateUserInput struct {
	Name   *string
	Role   *domain.Role
	Active *bool
}

// UpdateUser changes name, role and/or active flag, enforcing that the last
// active admin cannot be demoted or deactivated. Deactivation revokes the
// user's refresh tokens.
func (s *Service) UpdateUser(ctx context.Context, id string, in UpdateUserInput) (domain.User, error) {
	if !domain.ValidID(id) {
		return domain.User{}, domain.ErrUserNotFound
	}
	var name string
	if in.Name != nil {
		n, err := domain.NormalizeName(*in.Name)
		if err != nil {
			return domain.User{}, err
		}
		name = n
	}
	if in.Role != nil && !in.Role.Valid() {
		return domain.User{}, domain.ErrInvalidInput
	}
	updated, err := s.users.Mutate(ctx, id, func(u *domain.User, activeAdmins int) error {
		newRole, newActive := u.Role, u.Active
		if in.Role != nil {
			newRole = *in.Role
		}
		if in.Active != nil {
			newActive = *in.Active
		}
		if err := u.CanChangeTo(newRole, newActive, activeAdmins); err != nil {
			return err
		}
		if in.Name != nil {
			u.Name = name
		}
		u.Role, u.Active = newRole, newActive
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	if !updated.Active {
		if err := s.tokens.RevokeAllForUser(ctx, id, s.now()); err != nil {
			return domain.User{}, err
		}
	}
	return updated, nil
}

// ChangeOwnPassword verifies the current password, sets a new one and revokes
// every refresh token of the user.
func (s *Service) ChangeOwnPassword(ctx context.Context, id, current, next string) error {
	if err := domain.ValidatePassword(next); err != nil {
		return err
	}
	u, err := s.Me(ctx, id)
	if err != nil {
		return err
	}
	if !s.hasher.Verify(current, u.PasswordHash) {
		return domain.ErrInvalidCredentials
	}
	return s.setPassword(ctx, id, next)
}

// ResetPassword sets a new password for any user (admin operation) and
// revokes that user's refresh tokens.
func (s *Service) ResetPassword(ctx context.Context, id, next string) error {
	if !domain.ValidID(id) {
		return domain.ErrUserNotFound
	}
	if err := domain.ValidatePassword(next); err != nil {
		return err
	}
	return s.setPassword(ctx, id, next)
}

func (s *Service) setPassword(ctx context.Context, id, password string) error {
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if _, err := s.users.Mutate(ctx, id, func(u *domain.User, _ int) error {
		u.PasswordHash = hash
		return nil
	}); err != nil {
		return err
	}
	return s.tokens.RevokeAllForUser(ctx, id, s.now())
}

// Bootstrap creates the first admin when no users exist. It reports whether a
// user was created. Empty email/password is a no-op.
func (s *Service) Bootstrap(ctx context.Context, email, password string) (bool, error) {
	if email == "" || password == "" {
		return false, nil
	}
	n, err := s.users.Count(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	_, err = s.CreateUser(ctx, CreateUserInput{
		Name: "Administrator", Email: email, Password: password, Role: domain.RoleAdmin,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// VerifyAccess validates an access token (used by HTTP middleware).
func (s *Service) VerifyAccess(token string) (AccessClaims, error) {
	return s.issuer.ParseAccess(token, s.now())
}

func (s *Service) newSession(ctx context.Context, u domain.User) (Session, error) {
	now := s.now()
	access, accessExp, err := s.issuer.IssueAccess(u, now)
	if err != nil {
		return Session{}, fmt.Errorf("issue access token: %w", err)
	}
	raw, err := newOpaqueToken()
	if err != nil {
		return Session{}, err
	}
	exp := now.Add(s.refreshTTL)
	if err := s.tokens.Create(ctx, RefreshToken{
		UserID: u.ID, TokenHash: hashToken(raw), ExpiresAt: exp, CreatedAt: now,
	}); err != nil {
		return Session{}, err
	}
	return Session{
		User: u, AccessToken: access, AccessExpires: accessExp,
		RefreshToken: raw, RefreshExpires: exp,
	}, nil
}

func newOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
