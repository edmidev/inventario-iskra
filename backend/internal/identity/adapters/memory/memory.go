// Package memory provides in-memory identity repositories. They back unit
// tests of the use cases and HTTP adapter; they are not used in production.
package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
)

// Store implements app.UserRepository and app.RefreshTokenRepository.
type Store struct {
	mu     sync.Mutex
	seq    int
	users  map[string]domain.User
	tokens map[string]app.RefreshToken // by id
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{users: map[string]domain.User{}, tokens: map[string]app.RefreshToken{}}
}

func (s *Store) nextID() string {
	s.seq++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.seq)
}

// Create implements app.UserRepository.
func (s *Store) Create(_ context.Context, u *domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.users {
		if strings.EqualFold(e.Email, u.Email) {
			return domain.ErrEmailTaken
		}
	}
	now := time.Now()
	u.ID, u.CreatedAt, u.UpdatedAt = s.nextID(), now, now
	s.users[u.ID] = *u
	return nil
}

// GetByID implements app.UserRepository.
func (s *Store) GetByID(_ context.Context, id string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

// GetByEmail implements app.UserRepository.
func (s *Store) GetByEmail(_ context.Context, email string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

// List implements app.UserRepository.
func (s *Store) List(_ context.Context) ([]domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) || out[i].ID < out[j].ID })
	return out, nil
}

// Count implements app.UserRepository.
func (s *Store) Count(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users), nil
}

// Mutate implements app.UserRepository.
func (s *Store) Mutate(_ context.Context, id string, fn func(*domain.User, int) error) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	admins := 0
	for _, e := range s.users {
		if e.Role == domain.RoleAdmin && e.Active {
			admins++
		}
	}
	if err := fn(&u, admins); err != nil {
		return domain.User{}, err
	}
	u.UpdatedAt = time.Now()
	s.users[id] = u
	return u, nil
}

// createToken backs Tokens().Create (a separate name avoids clashing
// with UserRepository.Create on the same type; see Tokens()).
func (s *Store) createToken(t app.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.ID = s.nextID()
	s.tokens[t.ID] = t
	return nil
}

// Tokens returns the refresh-token repository view of the store.
func (s *Store) Tokens() app.RefreshTokenRepository { return tokenView{s} }

type tokenView struct{ s *Store }

func (v tokenView) Create(_ context.Context, t app.RefreshToken) error { return v.s.createToken(t) }

func (v tokenView) GetByHash(_ context.Context, hash string) (app.RefreshToken, error) {
	v.s.mu.Lock()
	defer v.s.mu.Unlock()
	for _, t := range v.s.tokens {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return app.RefreshToken{}, domain.ErrInvalidToken
}

func (v tokenView) GetByID(_ context.Context, id string) (app.RefreshToken, error) {
	v.s.mu.Lock()
	defer v.s.mu.Unlock()
	t, ok := v.s.tokens[id]
	if !ok {
		return app.RefreshToken{}, domain.ErrInvalidToken
	}
	return t, nil
}

func (v tokenView) Rotate(_ context.Context, id string, next app.RefreshToken, at time.Time) (bool, error) {
	v.s.mu.Lock()
	defer v.s.mu.Unlock()
	t, ok := v.s.tokens[id]
	if !ok || t.RevokedAt != nil {
		return false, nil
	}
	next.ID = v.s.nextID()
	v.s.tokens[next.ID] = next
	t.RevokedAt, t.RotatedAt, t.ReplacedBy = &at, &at, &next.ID
	v.s.tokens[id] = t
	return true, nil
}

func (v tokenView) Revoke(_ context.Context, id string, at time.Time) (bool, error) {
	v.s.mu.Lock()
	defer v.s.mu.Unlock()
	t, ok := v.s.tokens[id]
	if !ok || t.RevokedAt != nil {
		return false, nil
	}
	t.RevokedAt = &at
	v.s.tokens[id] = t
	return true, nil
}

func (v tokenView) RevokeAllForUser(_ context.Context, userID string, at time.Time) error {
	v.s.mu.Lock()
	defer v.s.mu.Unlock()
	for id, t := range v.s.tokens {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &at
			v.s.tokens[id] = t
		}
	}
	return nil
}

// PlainHasher is a fast, insecure hasher for tests only.
type PlainHasher struct{}

// Hash implements app.PasswordHasher.
func (PlainHasher) Hash(p string) (string, error) { return "plain:" + p, nil }

// Verify implements app.PasswordHasher.
func (PlainHasher) Verify(p, h string) bool { return h == "plain:"+p }
