// Package app holds the purchase order use cases and the ports they depend on.
//
// The catalog, stock and suppliers modules are reached only through the ports
// below; cmd/api implements them on top of those modules' services, so the
// purchasing module imports none of them.
package app

import (
	"context"
	"errors"

	"github.com/edgar/inventario/backend/internal/purchasing/domain"
)

// Port errors an implementation returns so the use cases can react without
// knowing the other modules.
var (
	ErrItemNotFound     = errors.New("item not found")
	ErrSupplierNotFound = errors.New("supplier not found")
)

// Repository persists orders. Mutations run under a row lock so concurrent
// requests on one order serialize.
type Repository interface {
	// Create stores a new draft and fills its ID, line IDs and timestamps.
	Create(ctx context.Context, o *domain.Order) error
	// Get returns an order with its lines and receipts, or ErrNotFound.
	Get(ctx context.Context, id int64) (domain.Order, error)
	// List returns a page of orders (newest first) with lines, and the total.
	List(ctx context.Context, f domain.Filter) ([]domain.Order, int, error)
	// Mutate locks the order, loads it, runs fn and stores what fn changed,
	// all in one transaction. The context given to fn carries that
	// transaction: ports called with it (catalog item creation, stock
	// movements) join it, so fn's side effects commit or roll back together
	// with the order. fn returning an error rolls everything back and Mutate
	// returns that error; fn returning domain.ErrNoChange stores nothing and
	// Mutate returns the order as loaded with a nil error. A missing order
	// yields ErrNotFound. Receipts with ID 0 are inserted.
	Mutate(ctx context.Context, id int64, fn func(ctx context.Context, o *domain.Order) error) (domain.Order, error)
}

// NewItemSpec is the definition of an item the catalog must create.
type NewItemSpec = domain.NewItem

// CreatedItem is an item the catalog just created.
type CreatedItem struct {
	ID   string
	Code string
}

// ItemInfo is what an order needs to know about an existing item.
type ItemInfo struct {
	Kind        string // "asset" or "consumable"
	Description string
}

// ItemCatalog is the purchasing view of the catalog module.
type ItemCatalog interface {
	// ValidateNewItem checks spec without storing anything. A
	// *domain.ValidationError's field names are relative to the spec
	// ("description", "category_id"...); the use case prefixes them.
	ValidateNewItem(ctx context.Context, spec NewItemSpec) error
	// CreateItem creates one item from spec, with a generated code. Assets are
	// placed at locationID in functional status; consumables are created with
	// no stock. It joins the transaction carried by ctx.
	CreateItem(ctx context.Context, spec NewItemSpec, locationID int64) (CreatedItem, error)
	// ItemInfo describes an existing item, or ErrItemNotFound.
	ItemInfo(ctx context.Context, itemID string) (ItemInfo, error)
	// LocationExists reports whether a location with that id exists.
	LocationExists(ctx context.Context, id int64) (bool, error)
}

// StockReceiver records goods coming in through the stock module.
type StockReceiver interface {
	// ReceiveIn records an `in` movement of qty at the location for a
	// consumable, attributed to userID. It joins the transaction carried by ctx.
	ReceiveIn(ctx context.Context, itemID, userID string, locationID int64, qty int, note string) error
}

// Suppliers looks suppliers up.
type Suppliers interface {
	// Get returns a live supplier's name and whether it is active, or
	// ErrSupplierNotFound when it does not exist or was deleted.
	Get(ctx context.Context, id int64) (name string, active bool, err error)
}
