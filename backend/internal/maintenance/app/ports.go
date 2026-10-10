// Package app holds the maintenance use cases and the ports they depend on.
package app

import (
	"context"

	"github.com/edgar/inventario/backend/internal/maintenance/domain"
)

// Decide funcs turn the locked item state (and, for edits, the target
// record) into the outcome to persist.
type (
	ReportDecideFunc     = func(domain.ItemState) (domain.ReportOutcome, error)
	PreventiveDecideFunc = func(domain.ItemState) (domain.PreventiveOutcome, error)
	UpdateDecideFunc     = func(domain.ItemState, domain.Record) (domain.UpdateOutcome, error)
	CloseDecideFunc      = func(domain.ItemState, domain.Record) (domain.CloseOutcome, error)
)

// Repository persists maintenance records and the item changes they cause.
// Every write locks the item row and changes record and item in one
// transaction; nothing is written when decide fails. Missing or soft-deleted
// items yield ErrItemNotFound (ErrRecordNotFound when addressed by record id).
type Repository interface {
	// Report inserts an open corrective record and sets the item status. A
	// concurrent report for the same item yields ErrOpenFailureExists.
	Report(ctx context.Context, itemID, userID string, decide ReportDecideFunc) (domain.Record, error)
	// RegisterPreventive inserts a closed preventive record and updates the
	// item's last_maintenance_at without touching its status.
	RegisterPreventive(ctx context.Context, itemID, userID string, decide PreventiveDecideFunc) (domain.Record, error)
	// Update edits an open record and optionally moves the item between
	// inoperative and in_repair.
	Update(ctx context.Context, recordID int64, decide UpdateDecideFunc) (domain.Record, error)
	// Close closes an open record, sets the item status and, when the item is
	// functional again, its last_maintenance_at.
	Close(ctx context.Context, recordID int64, userID string, decide CloseDecideFunc) (domain.Record, error)
	// History returns an item's records, newest first, with the total count.
	History(ctx context.Context, itemID string, p domain.Page) ([]domain.Record, int, error)
	// List returns records across items: open first, then newest first.
	List(ctx context.Context, f domain.ListFilter, p domain.Page) ([]domain.Record, int, error)
	// Due returns assets whose maintenance is overdue or due within the
	// filter's window, most overdue first.
	Due(ctx context.Context, f domain.DueFilter, p domain.Page) ([]domain.DueItem, int, error)
}
