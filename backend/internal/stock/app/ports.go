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
	// state, then updates stock/location and appends the movement. Nothing is
	// written when decide fails. Missing or soft-deleted items yield
	// ErrItemNotFound; a dangling destination yields ErrLocationNotFound.
	Record(ctx context.Context, itemID, userID string, decide DecideFunc) (domain.Movement, error)
	// List returns an item's movements, newest first, with the total count.
	// typ filters by movement type when non-empty. Missing or soft-deleted
	// items yield ErrItemNotFound.
	List(ctx context.Context, itemID string, typ domain.MovementType, p domain.Page) ([]domain.Movement, int, error)
	// Alerts returns consumables at or below their minimum, most severe first.
	Alerts(ctx context.Context, p domain.Page) ([]domain.Alert, int, error)
}
