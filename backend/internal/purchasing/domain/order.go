// Package domain holds the purchase order aggregate and its business rules.
// It is pure: no I/O, no knowledge of the catalog, stock or suppliers modules.
package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Limits.
const (
	// MaxAssetUnitsPerLine caps how many assets one line may create on receipt.
	MaxAssetUnitsPerLine = 100
	MaxLines             = 200
	MaxQuantity          = 100_000
	// MaxUnitCostMinor is 1e9 currency units in minor units; with MaxQuantity
	// and MaxLines it keeps an order total far from int64 overflow.
	MaxUnitCostMinor  = 100_000_000_000
	MaxRate           = 1_000_000_000
	MaxNotesLen       = 1000
	MaxReceiptNote    = 400
	MaxClientKeyLen   = 100
	MaxDescriptionLen = 500

	DefaultLimit = 25
	MaxLimit     = 100
)

// Status is the lifecycle state of an order.
type Status string

// Order states.
const (
	StatusDraft             Status = "draft"
	StatusOrdered           Status = "ordered"
	StatusPartiallyReceived Status = "partially_received"
	StatusReceived          Status = "received"
	StatusCancelled         Status = "cancelled"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusOrdered, StatusPartiallyReceived, StatusReceived, StatusCancelled:
		return true
	}
	return false
}

// Currency is the ISO code of the currency an order is priced in.
type Currency string

// Supported currencies.
const (
	USD Currency = "USD"
	VES Currency = "VES"
)

// Valid reports whether c is supported.
func (c Currency) Valid() bool { return c == USD || c == VES }

// Item kinds a new_item line may carry (the catalog owns their meaning).
const (
	KindAsset      = "asset"
	KindConsumable = "consumable"
)

// Error codes carried by a ValidationError when a rule has its own stable code.
const (
	CodeValidation       = "validation_failed"
	CodeOverReceipt      = "over_receipt"
	CodeSupplierInactive = "supplier_inactive"
)

// Domain errors. The HTTP adapter maps each to a status and stable code.
var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("purchase order not found")
	ErrInvalidState = errors.New("invalid state")
	// ErrNoChange is returned by a Mutate callback that decided nothing must be
	// stored (an idempotent replay); the repository returns the order untouched.
	ErrNoChange = errors.New("no change")
)

// ValidationError is an ErrInvalidInput carrying one message per offending
// field and, for a few rules, its own stable error code.
type ValidationError struct {
	Code    string // defaults to validation_failed
	Message string // optional human text for the envelope
	Fields  map[string]string
}

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
func FieldError(field, msg string) *ValidationError {
	return &ValidationError{Fields: map[string]string{field: msg}}
}

// InvalidStateError says an action is not allowed in the order's current status.
type InvalidStateError struct {
	Status Status
	Action string
}

// Error implements error.
func (e *InvalidStateError) Error() string {
	return fmt.Sprintf("cannot %s an order that is %s", e.Action, e.Status)
}

// Is makes errors.Is(err, ErrInvalidState) true.
func (e *InvalidStateError) Is(target error) bool { return target == ErrInvalidState }

// NewItem is the definition of an item to be created when its line is first
// received. The catalog validates it in full (through a port); the domain only
// needs its kind to apply the asset rules.
type NewItem struct {
	Description string         `json:"description"`
	CategoryID  int64          `json:"category_id"`
	Kind        string         `json:"kind"`
	Unit        string         `json:"unit,omitempty"`
	StockMin    int            `json:"stock_min,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	Notes       string         `json:"notes,omitempty"`
}

// Line is one ordered position.
type Line struct {
	ID       int64
	Position int
	// ItemID is the existing item ordered, or, for a new_item line, the first
	// item created for it once something was received.
	ItemID        string
	NewItem       *NewItem
	Description   string // snapshot taken when the line was saved
	Quantity      int
	ReceivedQty   int
	UnitCostMinor int64
}

// Remaining is the quantity still to receive.
func (l Line) Remaining() int { return l.Quantity - l.ReceivedQty }

// IsAsset reports whether receiving the line creates one asset per unit.
func (l Line) IsAsset() bool { return l.NewItem != nil && l.NewItem.Kind == KindAsset }

// ReceiptItem is an item a receipt line brought in.
type ReceiptItem struct {
	ItemID   string
	Code     string // read-side convenience; not stored by purchasing
	Quantity int
}

// ReceiptLine is the part of one order line a receipt covers.
type ReceiptLine struct {
	LineID   int64
	Quantity int
	Items    []ReceiptItem
}

// Receipt is one delivery registered against an order.
type Receipt struct {
	ID         int64 // 0 until stored
	LocationID int64
	ReceivedBy string
	ReceivedAt time.Time
	Note       string
	ClientKey  string
	Lines      []ReceiptLine
}

// Order is a purchase order with its lines and receipts.
type Order struct {
	ID           int64
	SupplierID   int64
	SupplierName string // filled by the repository on read
	Status       Status
	Currency     Currency
	ExchangeRate *float64 // VES per USD, reference only
	ExpectedAt   *time.Time
	Notes        string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	OrderedAt    *time.Time
	CancelledAt  *time.Time
	Lines        []Line
	Receipts     []Receipt

	// LinesDirty tells the repository the line set was replaced and must be
	// rewritten. Otherwise only received quantities and item ids are updated.
	LinesDirty bool
}

// Total is the order value in minor units of its currency.
func (o Order) Total() int64 {
	var t int64
	for _, l := range o.Lines {
		t += int64(l.Quantity) * l.UnitCostMinor
	}
	return t
}

// ReceivedAnything reports whether any unit was received.
func (o Order) ReceivedAnything() bool {
	for _, l := range o.Lines {
		if l.ReceivedQty > 0 {
			return true
		}
	}
	return false
}

// ReceiptByKey returns the receipt registered with a client key.
func (o Order) ReceiptByKey(key string) (Receipt, bool) {
	if key == "" {
		return Receipt{}, false
	}
	for _, r := range o.Receipts {
		if r.ClientKey == key {
			return r, true
		}
	}
	return Receipt{}, false
}

// LineInput is the editable part of a line.
type LineInput struct {
	ItemID        string
	NewItem       *NewItem
	Description   string // snapshot; the app fills it before building the order
	Quantity      int
	UnitCostMinor int64
}

// Input is the editable part of an order.
type Input struct {
	SupplierID   int64
	Currency     Currency
	ExchangeRate *float64
	ExpectedAt   *time.Time
	Notes        string
	// Lines replaces the whole line set when non-nil.
	Lines []LineInput
}

// Normalize trims text and validates the order shape. It does not touch other
// modules: whether the supplier exists or an item is a consumable is checked
// by the application through ports.
func (in Input) Normalize() (Input, error) {
	out := in
	out.Notes = strings.TrimSpace(in.Notes)
	fe := map[string]string{}
	if in.SupplierID <= 0 {
		fe["supplier_id"] = "required"
	}
	if !in.Currency.Valid() {
		fe["currency"] = "must be USD or VES"
	}
	if r := in.ExchangeRate; r != nil && (*r <= 0 || *r > MaxRate) {
		fe["exchange_rate"] = "must be greater than 0 (VES per USD)"
	}
	if len(out.Notes) > MaxNotesLen {
		fe["notes"] = "up to 1000 characters"
	}
	if in.Lines != nil {
		if len(in.Lines) > MaxLines {
			fe["lines"] = "up to 200 lines"
		}
		out.Lines = make([]LineInput, len(in.Lines))
		for i, l := range in.Lines {
			nl, lfe := normalizeLine(i, l)
			out.Lines[i] = nl
			for k, v := range lfe {
				fe[k] = v
			}
		}
	}
	if len(fe) > 0 {
		return Input{}, &ValidationError{Fields: fe}
	}
	return out, nil
}

func normalizeLine(i int, l LineInput) (LineInput, map[string]string) {
	p := fmt.Sprintf("lines[%d].", i)
	fe := map[string]string{}
	l.ItemID = strings.TrimSpace(l.ItemID)
	switch {
	case l.ItemID != "" && l.NewItem != nil:
		fe[p+"item_id"] = "provide either item_id or new_item, not both"
	case l.ItemID == "" && l.NewItem == nil:
		fe[p+"item_id"] = "provide item_id or new_item"
	}
	if l.Quantity < 1 || l.Quantity > MaxQuantity {
		fe[p+"quantity"] = "must be between 1 and 100000"
	}
	if l.UnitCostMinor < 0 || l.UnitCostMinor > MaxUnitCostMinor {
		fe[p+"unit_cost_minor"] = "must be between 0 and 100000000000 (minor units)"
	}
	if ni := l.NewItem; ni != nil {
		c := *ni
		c.Description = strings.TrimSpace(c.Description)
		c.Kind = strings.TrimSpace(c.Kind)
		l.NewItem = &c
		if c.Description == "" || len(c.Description) > MaxDescriptionLen {
			fe[p+"new_item.description"] = "required, up to 500 characters"
		}
		switch c.Kind {
		case KindAsset:
			if l.Quantity > MaxAssetUnitsPerLine {
				fe[p+"quantity"] = fmt.Sprintf("asset lines create one item per unit: up to %d", MaxAssetUnitsPerLine)
			}
		case KindConsumable:
		default:
			fe[p+"new_item.kind"] = "must be asset or consumable"
		}
		l.Description = c.Description
	}
	return l, fe
}

// NewDraft builds a draft order from validated input.
func NewDraft(in Input, createdBy string, now time.Time) Order {
	o := Order{
		SupplierID: in.SupplierID, Status: StatusDraft, Currency: in.Currency,
		ExchangeRate: in.ExchangeRate, ExpectedAt: in.ExpectedAt, Notes: in.Notes,
		CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now, LinesDirty: true,
	}
	o.Lines = buildLines(in.Lines)
	return o
}

func buildLines(in []LineInput) []Line {
	out := make([]Line, len(in))
	for i, l := range in {
		out[i] = Line{
			Position: i + 1, ItemID: l.ItemID, NewItem: l.NewItem, Description: l.Description,
			Quantity: l.Quantity, UnitCostMinor: l.UnitCostMinor,
		}
	}
	return out
}

// Edit replaces the editable fields. Only a draft can be edited.
func (o *Order) Edit(in Input, now time.Time) error {
	if o.Status != StatusDraft {
		return &InvalidStateError{Status: o.Status, Action: "edit"}
	}
	o.SupplierID, o.Currency, o.ExchangeRate = in.SupplierID, in.Currency, in.ExchangeRate
	o.ExpectedAt, o.Notes = in.ExpectedAt, in.Notes
	if in.Lines != nil {
		o.Lines = buildLines(in.Lines)
		o.LinesDirty = true
	}
	o.UpdatedAt = now
	return nil
}

// Place moves a draft to ordered. It needs at least one line.
func (o *Order) Place(now time.Time) error {
	if o.Status != StatusDraft {
		return &InvalidStateError{Status: o.Status, Action: "place"}
	}
	if len(o.Lines) == 0 {
		return FieldError("lines", "at least one line is required to place the order")
	}
	o.Status, o.OrderedAt, o.UpdatedAt = StatusOrdered, &now, now
	return nil
}

// Cancel moves a draft or an ordered order with nothing received to cancelled.
func (o *Order) Cancel(now time.Time) error {
	if o.Status != StatusDraft && o.Status != StatusOrdered {
		return &InvalidStateError{Status: o.Status, Action: "cancel"}
	}
	if o.ReceivedAnything() {
		return &InvalidStateError{Status: o.Status, Action: "cancel"}
	}
	o.Status, o.CancelledAt, o.UpdatedAt = StatusCancelled, &now, now
	return nil
}

// Filter narrows an order listing.
type Filter struct {
	Status     Status
	SupplierID int64
	// Q matches the supplier name (substring, case-insensitive) or the order
	// id as text (exact, an optional leading # ignored).
	Q      string
	Limit  int
	Offset int
}

// Normalize applies pagination defaults and validates bounds.
func (f Filter) Normalize() (Filter, error) {
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	fe := map[string]string{}
	if f.Limit < 0 || f.Limit > MaxLimit {
		fe["limit"] = "must be between 1 and 100"
	}
	if f.Offset < 0 {
		fe["offset"] = "must be 0 or greater"
	}
	if f.Status != "" && !f.Status.Valid() {
		fe["status"] = "must be draft, ordered, partially_received, received or cancelled"
	}
	if f.SupplierID < 0 {
		fe["supplier_id"] = "must be a positive integer"
	}
	if len(fe) > 0 {
		return Filter{}, &ValidationError{Fields: fe}
	}
	f.Q = strings.TrimSpace(f.Q)
	return f, nil
}
