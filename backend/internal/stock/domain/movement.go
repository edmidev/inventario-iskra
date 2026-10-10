// Package domain contains the stock rules: the movement types, how each one
// changes an item, and the errors the adapters translate to HTTP.
package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MovementType is the kind of change a movement records.
type MovementType string

// Movement types. Opening is written by the system when a consumable is
// created with stock; clients cannot request it.
const (
	TypeOpening  MovementType = "opening"
	TypeIn       MovementType = "in"
	TypeOut      MovementType = "out"
	TypeAdjust   MovementType = "adjust"
	TypeTransfer MovementType = "transfer"
)

// Valid reports whether t is a stored movement type.
func (t MovementType) Valid() bool {
	switch t {
	case TypeOpening, TypeIn, TypeOut, TypeAdjust, TypeTransfer:
		return true
	}
	return false
}

// Kind mirrors the catalog item kind; stock keeps its own copy so it does not
// import the catalog domain.
type Kind string

// Item kinds.
const (
	KindAsset      Kind = "asset"
	KindConsumable Kind = "consumable"
)

// Limits.
const (
	MaxStock   = 1_000_000_000
	MaxNoteLen = 500
)

// Domain errors the HTTP adapter maps to statuses and stable codes.
var (
	ErrItemNotFound      = errors.New("item not found")
	ErrLocationNotFound  = errors.New("location not found")
	ErrNotApplicable     = errors.New("movement not applicable to this item kind")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrInvalidInput      = errors.New("invalid input")
	ErrItemOnLoan        = errors.New("item is on loan; custody owns its location")
)

// InsufficientStockError carries the numbers behind ErrInsufficientStock.
type InsufficientStockError struct{ Available, Requested int }

// Error implements error.
func (e *InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock: %d available, %d requested", e.Available, e.Requested)
}

// Is makes errors.Is(err, ErrInsufficientStock) true.
func (e *InsufficientStockError) Is(target error) bool { return target == ErrInsufficientStock }

// ValidationError is an ErrInvalidInput with one message per offending field.
type ValidationError struct{ Fields map[string]string }

// Error implements error.
func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + e.Fields[k]
	}
	return "invalid input: " + strings.Join(parts, "; ")
}

// Is makes errors.Is(err, ErrInvalidInput) true.
func (e *ValidationError) Is(target error) bool { return target == ErrInvalidInput }

// FieldError builds a single-field ValidationError.
func FieldError(field, msg string) error {
	return &ValidationError{Fields: map[string]string{field: msg}}
}

// Movement is one immutable ledger row. Qty is the signed delta actually
// applied to stock (0 for transfers).
type Movement struct {
	ID             int64
	ItemID         string
	Type           MovementType
	Qty            int
	StockBefore    int
	StockAfter     int
	FromLocationID *int64
	ToLocationID   *int64
	UserID         *string // nil for system rows (migration backfill, imports)
	UserName       *string
	Note           string
	CreatedAt      time.Time
}

// ItemState is what the rules need to know about the locked item.
type ItemState struct {
	Kind         Kind
	StockCurrent int
	LocationID   *int64
	// OnLoan is true while custody has an active assignment for the item.
	// Stock only reads this flag; custody owns the loan itself.
	OnLoan bool
}

// Request is a client-requested movement. For adjust, Qty is the absolute
// counted quantity; for in and out it is the amount moved; for transfer it
// must be zero.
type Request struct {
	Type         MovementType
	Qty          int
	ToLocationID *int64
	Note         string
}

// Outcome is the decision for one request: the movement to record and the
// resulting item state to persist.
type Outcome struct {
	Movement     Movement // ID, ItemID, user and timestamp are filled by the repository
	StockCurrent int
	LocationID   *int64
}

// Decide applies the stock rules to the current item state. It never mutates
// anything; the repository persists the Outcome atomically under a row lock.
func Decide(cur ItemState, req Request) (Outcome, error) {
	note := strings.TrimSpace(req.Note)
	switch req.Type {
	case TypeIn, TypeOut, TypeAdjust, TypeTransfer:
	default:
		return Outcome{}, FieldError("type", "must be in, out, adjust or transfer")
	}
	if len(note) > MaxNoteLen {
		return Outcome{}, FieldError("note", "up to 500 characters")
	}
	if req.Type != TypeTransfer && cur.Kind != KindConsumable {
		return Outcome{}, ErrNotApplicable
	}
	if req.Type != TypeTransfer && req.ToLocationID != nil {
		return Outcome{}, FieldError("to_location_id", "only allowed for transfer")
	}

	out := Outcome{StockCurrent: cur.StockCurrent, LocationID: cur.LocationID}
	m := Movement{Type: req.Type, StockBefore: cur.StockCurrent, StockAfter: cur.StockCurrent, Note: note}

	switch req.Type {
	case TypeIn:
		if req.Qty <= 0 || req.Qty > MaxStock {
			return Outcome{}, FieldError("qty", "must be between 1 and 1000000000")
		}
		if cur.StockCurrent+req.Qty > MaxStock {
			return Outcome{}, FieldError("qty", "resulting stock would exceed 1000000000")
		}
		m.Qty = req.Qty
	case TypeOut:
		if req.Qty <= 0 || req.Qty > MaxStock {
			return Outcome{}, FieldError("qty", "must be between 1 and 1000000000")
		}
		if req.Qty > cur.StockCurrent {
			return Outcome{}, &InsufficientStockError{Available: cur.StockCurrent, Requested: req.Qty}
		}
		m.Qty = -req.Qty
	case TypeAdjust:
		fe := map[string]string{}
		if req.Qty < 0 || req.Qty > MaxStock {
			fe["qty"] = "counted quantity must be between 0 and 1000000000"
		}
		if note == "" {
			fe["note"] = "required for adjustments"
		}
		if len(fe) > 0 {
			return Outcome{}, &ValidationError{Fields: fe}
		}
		m.Qty = req.Qty - cur.StockCurrent
	case TypeTransfer:
		if cur.OnLoan {
			return Outcome{}, ErrItemOnLoan
		}
		fe := map[string]string{}
		if req.Qty != 0 {
			fe["qty"] = "must be omitted or 0 for transfers"
		}
		switch {
		case req.ToLocationID == nil:
			fe["to_location_id"] = "required"
		case cur.LocationID != nil && *cur.LocationID == *req.ToLocationID:
			fe["to_location_id"] = "item is already in this location"
		}
		if len(fe) > 0 {
			return Outcome{}, &ValidationError{Fields: fe}
		}
		m.FromLocationID = cur.LocationID
		m.ToLocationID = req.ToLocationID
		out.LocationID = req.ToLocationID
	}
	m.StockAfter = m.StockBefore + m.Qty
	out.StockCurrent = m.StockAfter
	out.Movement = m
	return out, nil
}

// Alert is a consumable at or below its minimum stock.
type Alert struct {
	ItemID       string
	Code         string
	Description  string
	Unit         string
	StockCurrent int
	StockMin     int
	LocationID   *int64
	LocationName *string
}

// Shortage is stock_current - stock_min: the lower (more negative), the more severe.
func (a Alert) Shortage() int { return a.StockCurrent - a.StockMin }

// Paging limits shared by listings.
const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// Page is a validated limit/offset pair.
type Page struct{ Limit, Offset int }

// NormalizePage applies defaults and validates the bounds.
func NormalizePage(limit, offset int) (Page, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	fe := map[string]string{}
	if limit < 0 || limit > MaxLimit {
		fe["limit"] = "must be between 1 and 100"
	}
	if offset < 0 {
		fe["offset"] = "must be 0 or greater"
	}
	if len(fe) > 0 {
		return Page{}, &ValidationError{Fields: fe}
	}
	return Page{Limit: limit, Offset: offset}, nil
}
