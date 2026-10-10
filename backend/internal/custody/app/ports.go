// Package app holds the custody use cases and the ports they depend on.
package app

import (
	"context"

	"github.com/edgar/inventario/backend/internal/custody/domain"
)

// AssignDecideFunc turns the locked item state into the loan to persist.
type AssignDecideFunc = func(domain.ItemState) (domain.AssignOutcome, error)

// ReturnDecideFunc turns the locked item state into the return to persist.
type ReturnDecideFunc = func(domain.ItemState) (domain.ReturnOutcome, error)

// AssignmentRepository persists loans and the item location they change.
type AssignmentRepository interface {
	// Assign atomically locks the item, loads the assignee location, runs
	// decide, then inserts the assignment and moves the item. Nothing is
	// written when decide fails. Missing or soft-deleted items yield
	// ErrItemNotFound; a concurrent loan of the same item yields
	// ErrAlreadyAssigned. The returned assignment carries display names.
	Assign(ctx context.Context, itemID, userID string, assigneeID int64, decide AssignDecideFunc) (domain.Assignment, error)
	// Return atomically locks the item, loads the optional return location,
	// runs decide, closes the active assignment and moves the item.
	Return(ctx context.Context, itemID, userID string, returnLocationID *int64, decide ReturnDecideFunc) (domain.Assignment, error)
	// History returns an item's assignments, newest first, with the total
	// count. Missing or soft-deleted items yield ErrItemNotFound.
	History(ctx context.Context, itemID string, p domain.Page) ([]domain.Assignment, int, error)
	// Active lists active loans: overdue first, then oldest first.
	Active(ctx context.Context, f domain.ActiveFilter, p domain.Page) ([]domain.Assignment, int, error)
}
