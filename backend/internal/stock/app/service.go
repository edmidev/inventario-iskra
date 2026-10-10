package app

import (
	"context"
	"errors"

	"github.com/edgar/inventario/backend/internal/stock/domain"
)

// Service implements the stock use cases.
type Service struct{ repo MovementRepository }

// NewService wires the repository.
func NewService(r MovementRepository) *Service { return &Service{repo: r} }

// Result is a recorded movement and the item state it produced.
type Result struct {
	Movement     domain.Movement
	StockCurrent int
	LocationID   *int64
}

// Record applies req to the item on behalf of userID.
func (s *Service) Record(ctx context.Context, itemID, userID string, req domain.Request) (Result, error) {
	var outcome domain.Outcome
	m, err := s.repo.Record(ctx, itemID, userID, func(cur domain.ItemState) (domain.Outcome, error) {
		o, err := domain.Decide(cur, req)
		outcome = o
		return o, err
	})
	if errors.Is(err, domain.ErrLocationNotFound) {
		return Result{}, domain.FieldError("to_location_id", "location does not exist")
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Movement: m, StockCurrent: outcome.StockCurrent, LocationID: outcome.LocationID}, nil
}

// History returns one page of an item's movements, newest first.
func (s *Service) History(ctx context.Context, itemID string, typ domain.MovementType, limit, offset int) ([]domain.Movement, int, domain.Page, error) {
	p, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return nil, 0, p, err
	}
	if typ != "" && !typ.Valid() {
		return nil, 0, p, domain.FieldError("type", "must be opening, in, out, adjust or transfer")
	}
	ms, total, err := s.repo.List(ctx, itemID, typ, p)
	return ms, total, p, err
}

// Alerts returns one page of low-stock consumables, most severe first.
func (s *Service) Alerts(ctx context.Context, limit, offset int) ([]domain.Alert, int, domain.Page, error) {
	p, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return nil, 0, p, err
	}
	as, total, err := s.repo.Alerts(ctx, p)
	return as, total, p, err
}
