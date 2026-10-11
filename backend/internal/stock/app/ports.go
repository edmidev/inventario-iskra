// Package app holds the stock use cases and the ports they depend on.
package app

import (
	"context"

	"github.com/edgar/inventario/backend/internal/stock/domain"
)

// DecideFunc turns the locked current item state into the outcome to persist.
type DecideFunc = func(domain.ItemState) (domain.Outcome, error)

// MovementRepository persists the ledger and the item state it changes.
type MovementRepository interface {
	// Record atomically locks the item, runs decide against its current
	// state (including its per-location stock), then writes the per-location
	// quantities, the item total and the movement. Nothing is written when
	// decide fails. It returns the movement and, for consumables, the
	// resulting per-location stock. Missing or soft-deleted items yield
	// ErrItemNotFound; a location deleted meanwhile yields ErrLocationNotFound.
	Record(ctx context.Context, itemID, userID string, decide DecideFunc) (domain.Movement, []domain.LocationStock, error)
	// List returns an item's movements, newest first, with the total count.
	// typ filters by movement type when non-empty. Missing or soft-deleted
	// items yield ErrItemNotFound.
	List(ctx context.Context, itemID string, typ domain.MovementType, p domain.Page) ([]domain.Movement, int, error)
	// Alerts returns consumables at or below their minimum, most severe first.
	Alerts(ctx context.Context, p domain.Page) ([]domain.Alert, int, error)
}
