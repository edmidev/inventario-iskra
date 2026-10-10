package app

import (
	"context"
	"strings"
	"time"

	"github.com/edgar/inventario/backend/internal/custody/domain"
)

// Service implements the custody use cases.
type Service struct {
	repo AssignmentRepository
	now  func() time.Time
}

// NewService wires the repository. now may be nil (defaults to time.Now).
func NewService(r AssignmentRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: r, now: now}
}

// Now returns the service clock, so adapters compute overdue consistently.
func (s *Service) Now() time.Time { return s.now() }

// Assign lends the item to the assignee on behalf of userID.
func (s *Service) Assign(ctx context.Context, itemID, userID string, req domain.AssignRequest) (domain.Assignment, error) {
	now := s.now()
	return s.repo.Assign(ctx, itemID, userID, req.AssigneeLocationID, func(cur domain.ItemState) (domain.AssignOutcome, error) {
		return domain.DecideAssign(cur, req, now)
	})
}

// Return closes the item's active assignment on behalf of userID.
func (s *Service) Return(ctx context.Context, itemID, userID string, req domain.ReturnRequest) (domain.Assignment, error) {
	now := s.now()
	return s.repo.Return(ctx, itemID, userID, req.ReturnLocationID, func(cur domain.ItemState) (domain.ReturnOutcome, error) {
		return domain.DecideReturn(cur, req, now)
	})
}

// History returns one page of an item's assignments, newest first.
func (s *Service) History(ctx context.Context, itemID string, limit, offset int) ([]domain.Assignment, int, domain.Page, error) {
	p, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return nil, 0, p, err
	}
	as, total, err := s.repo.History(ctx, itemID, p)
	return as, total, p, err
}

// Active returns one page of active loans, overdue first, then oldest first.
func (s *Service) Active(ctx context.Context, f domain.ActiveFilter, limit, offset int) ([]domain.Assignment, int, domain.Page, error) {
	p, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return nil, 0, p, err
	}
	f.Q = strings.TrimSpace(f.Q)
	f.Now = s.now()
	as, total, err := s.repo.Active(ctx, f, p)
	return as, total, p, err
}
