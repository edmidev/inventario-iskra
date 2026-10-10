// Package app holds the dashboard use case: one read-only summary.
package app

import (
	"context"
	"time"

	"github.com/edgar/inventario/backend/internal/dashboard/domain"
)

// Repository computes the summary as of the given instant.
type Repository interface {
	Summary(ctx context.Context, now time.Time) (domain.Summary, error)
}

// Service serves the dashboard summary.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService wires the repository. now may be nil (defaults to time.Now).
func NewService(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// Summary returns the current dashboard snapshot.
func (s *Service) Summary(ctx context.Context) (domain.Summary, error) {
	now := s.now()
	sum, err := s.repo.Summary(ctx, now)
	if err != nil {
		return domain.Summary{}, err
	}
	sum.GeneratedAt = now
	return sum, nil
}
