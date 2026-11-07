package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	qtyval "github.com/edgar/inventario/backend/internal/platform/quantity"

	"github.com/edgar/inventario/backend/internal/stock/domain"
)

// Service implements the stock use cases.
type Service struct{ repo MovementRepository }

type executionMaterialRepository interface {
	RecordExecutionMaterial(context.Context, string, string, int64, string, string, func(domain.ItemState) (domain.Outcome, error)) (domain.Movement, []domain.LocationStock, error)
}

// NewService wires the repository.
func NewService(r MovementRepository) *Service { return &Service{repo: r} }

// Result is a recorded movement and the item state it produced.
type Result struct {
	Movement     domain.Movement
	StockCurrent qtyval.Quantity
	// LocationID is an asset's new location; consumables have none.
	LocationID *int64
	// Stocks is a consumable's per-location stock after the movement (empty
	// for assets), ordered by location name.
	Stocks []domain.LocationStock
}

// Record applies req to the item on behalf of userID.
func (s *Service) Record(ctx context.Context, itemID, userID string, req domain.Request) (Result, error) {
	var outcome domain.Outcome
	m, stocks, err := s.repo.Record(ctx, itemID, userID, func(cur domain.ItemState) (domain.Outcome, error) {
		o, err := domain.Decide(cur, req)
		outcome = o
		return o, err
	})
	if errors.Is(err, domain.ErrLocationNotFound) {
		// Only reachable when a location was deleted after decide checked it.
		field := "location_id"
		if req.Type == domain.TypeTransfer {
			field = "to_location_id"
		}
		return Result{}, domain.FieldError(field, "location does not exist")
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Movement: m, StockCurrent: outcome.StockCurrent, LocationID: outcome.LocationID, Stocks: stocks}, nil
}

// RecordExecutionMaterial performs a stock movement against one execution's
// reservation. Ordinary movement endpoints cannot set this execution scope.
func (s *Service) RecordExecutionMaterial(ctx context.Context, itemID, userID string, executionID int64, requestID string, req domain.Request) (Result, error) {
	repo, ok := s.repo.(executionMaterialRepository)
	if !ok {
		return Result{}, domain.ErrInvalidInput
	}
	serialized, err := json.Marshal(struct {
		ExecutionID int64
		ItemID      string
		UserID      string
		Request     domain.Request
	}{executionID, itemID, userID, req})
	if err != nil {
		return Result{}, err
	}
	fingerprintBytes := sha256.Sum256(serialized)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	var outcome domain.Outcome
	m, stocks, err := repo.RecordExecutionMaterial(ctx, itemID, userID, executionID, requestID, fingerprint, func(cur domain.ItemState) (domain.Outcome, error) {
		o, decideErr := domain.Decide(cur, req)
		outcome = o
		return o, decideErr
	})
	if err != nil {
		return Result{}, err
	}
	if outcome.Movement.Type == "" {
		outcome.StockCurrent = m.StockAfter
	}
	return Result{Movement: m, StockCurrent: outcome.StockCurrent, LocationID: outcome.LocationID, Stocks: stocks}, nil
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
