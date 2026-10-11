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
	// ErrLocationRequired is returned when a consumable is stocked in several
	// locations and the request did not say which one it applies to.
	ErrLocationRequired = errors.New("location required")
)

// InsufficientStockError carries the numbers behind ErrInsufficientStock.
// LocationID and LocationName are set when the shortfall is at one location.
type InsufficientStockError struct {
	Available, Requested int
	LocationID           *int64
	LocationName         string
}

// Error implements error.
func (e *InsufficientStockError) Error() string {
	if e.LocationName != "" {
		return fmt.Sprintf("insufficient stock at %s: %d available, %d requested", e.LocationName, e.Available, e.Requested)
	}
	return fmt.Sprintf("insufficient stock: %d available, %d requested", e.Available, e.Requested)
}

// Is makes errors.Is(err, ErrInsufficientStock) true.
func (e *InsufficientStockError) Is(target error) bool { return target == ErrInsufficientStock }

// ValidationError is an ErrInvalidInput with one message per offending field.
type ValidationError struct{ Fields map[string]string }

// Error implements error.
func (e *ValidationError) Error() string { return "invalid input: " + joinFields(e.Fields) }

// Is makes errors.Is(err, ErrInvalidInput) true.
func (e *ValidationError) Is(target error) bool { return target == ErrInvalidInput }

// LocationRequiredError is an ErrLocationRequired naming the offending field.
type LocationRequiredError struct{ Fields map[string]string }

// Error implements error.
func (e *LocationRequiredError) Error() string { return "location required: " + joinFields(e.Fields) }

// Is makes errors.Is(err, ErrLocationRequired) true.
func (e *LocationRequiredError) Is(target error) bool { return target == ErrLocationRequired }

func joinFields(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + fields[k]
	}
	return strings.Join(parts, "; ")
}

// FieldError builds a single-field ValidationError.
func FieldError(field, msg string) error {
	return &ValidationError{Fields: map[string]string{field: msg}}
}

// Movement is one immutable ledger row. Qty is the signed delta actually
// applied to the item's total stock (0 for transfers). For consumables
// LocationID is the location an in/out/adjust/opening affected; a transfer
// carries From/To plus MovedQty, the quantity moved between them. Legacy rows
// written before stock per location may have no location.
type Movement struct {
	ID             int64
	ItemID         string
	Type           MovementType
	Qty            int
	StockBefore    int
	StockAfter     int
	LocationID     *int64
	FromLocationID *int64
	ToLocationID   *int64
	MovedQty       int
	UserID         *string // nil for system rows (migration backfill, imports)
	UserName       *string
	Note           string
	CreatedAt      time.Time
}

// LocationStock is the quantity of a consumable held at one location.
type LocationStock struct {
	LocationID   int64
	LocationName string
	Quantity     int
}

// ItemState is what the rules need to know about the locked item.
type ItemState struct {
	Kind         Kind
	StockCurrent int
	// LocationID is an asset's location; consumables have none (see Stocks).
	LocationID *int64
	// OnLoan is true while custody has an active assignment for the item.
	// Stock only reads this flag; custody owns the loan itself.
	OnLoan bool
	// Stocks is a consumable's per-location quantities (only locations with
	// stock), ordered by location name.
	Stocks []LocationStock
	// Locations maps every existing location id to its name.
	Locations map[int64]string
	// LastLocationID is the location of the consumable's latest movement, if
	// any; used to place stock coming back after the item ran out.
	LastLocationID *int64
}

func (s ItemState) quantityAt(id int64) int {
	for _, st := range s.Stocks {
		if st.LocationID == id {
			return st.Quantity
		}
	}
	return 0
}

// Request is a client-requested movement. For adjust, Qty is the absolute
// counted quantity at the location; for in and out it is the amount moved;
// for a consumable transfer it is the quantity to move; for an asset transfer
// it must be zero.
type Request struct {
	Type MovementType
	Qty  int
	// LocationID is where a consumable in/out/adjust applies. When omitted it
	// is inferred from the item's stock (see Decide).
	LocationID *int64
	// FromLocationID is the source of a consumable transfer (inferred when the
	// item is stocked in exactly one location).
	FromLocationID *int64
	// ToLocationID is the destination of a transfer.
	ToLocationID *int64
	Note         string
}

// StockChange sets the absolute quantity of a consumable at one location. A
// zero quantity removes the location's row. LocationID 0 stands for the
// default location, which the repository resolves (see Outcome.DefaultLocation).
type StockChange struct {
	LocationID int64
	Quantity   int
}

// Outcome is the decision for one request: the movement to record and the
// resulting item state to persist.
type Outcome struct {
	Movement     Movement // ID, ItemID, user and timestamp are filled by the repository
	StockCurrent int
	// LocationID is an asset's new location.
	LocationID *int64
	// Changes are the per-location quantities to write for a consumable.
	Changes []StockChange
	// DefaultLocation is true when the stock goes to "Sin ubicación": the
	// repository finds or creates it and substitutes its id for 0 in
	// Changes and Movement.LocationID.
	DefaultLocation bool
}

// Decide applies the stock rules to the current item state. It never mutates
// anything; the repository persists the Outcome atomically under a row lock.
//
// Consumables are stocked per location. in/out/adjust apply to
// req.LocationID; when omitted it is inferred: the only location holding
// stock, else (nothing in stock) the location of the latest movement or the
// default location for in/adjust, else ErrLocationRequired. adjust sets the
// counted quantity at that location. transfer moves Qty between
// FromLocationID and ToLocationID without changing the total. Assets keep a
// single location and can only be transferred.
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
	if req.Type != TypeTransfer {
		if req.ToLocationID != nil {
			return Outcome{}, FieldError("to_location_id", "only allowed for transfer")
		}
		if req.FromLocationID != nil {
			return Outcome{}, FieldError("from_location_id", "only allowed for transfer")
		}
	}
	m := Movement{Type: req.Type, StockBefore: cur.StockCurrent, StockAfter: cur.StockCurrent, Note: note}
	if req.Type == TypeTransfer {
		if cur.Kind == KindAsset {
			return decideAssetTransfer(cur, req, m)
		}
		return decideConsumableTransfer(cur, req, m)
	}
	return decideConsumable(cur, req, m, note)
}

func decideAssetTransfer(cur ItemState, req Request, m Movement) (Outcome, error) {
	if cur.OnLoan {
		return Outcome{}, ErrItemOnLoan
	}
	fe := map[string]string{}
	if req.Qty != 0 {
		fe["qty"] = "must be omitted or 0 for transfers"
	}
	if req.LocationID != nil {
		fe["location_id"] = "not allowed for transfer; use to_location_id"
	}
	if req.FromLocationID != nil {
		fe["from_location_id"] = "only allowed for consumables"
	}
	switch {
	case req.ToLocationID == nil:
		fe["to_location_id"] = "required"
	case !cur.locationExists(*req.ToLocationID):
		fe["to_location_id"] = "location does not exist"
	case cur.LocationID != nil && *cur.LocationID == *req.ToLocationID:
		fe["to_location_id"] = "item is already in this location"
	}
	if len(fe) > 0 {
		return Outcome{}, &ValidationError{Fields: fe}
	}
	m.FromLocationID = cur.LocationID
	m.ToLocationID = req.ToLocationID
	return Outcome{Movement: m, StockCurrent: cur.StockCurrent, LocationID: req.ToLocationID}, nil
}

func (s ItemState) locationExists(id int64) bool {
	_, ok := s.Locations[id]
	return ok
}

func decideConsumableTransfer(cur ItemState, req Request, m Movement) (Outcome, error) {
	fe := map[string]string{}
	if req.LocationID != nil {
		fe["location_id"] = "not allowed for transfer; use from_location_id and to_location_id"
	}
	if req.Qty <= 0 || req.Qty > MaxStock {
		fe["qty"] = "quantity to move must be between 1 and 1000000000"
	}
	if req.ToLocationID == nil {
		fe["to_location_id"] = "required"
	} else if !cur.locationExists(*req.ToLocationID) {
		fe["to_location_id"] = "location does not exist"
	}
	var from *int64
	switch {
	case req.FromLocationID != nil:
		from = req.FromLocationID
		if !cur.locationExists(*from) {
			fe["from_location_id"] = "location does not exist"
		}
	case len(cur.Stocks) == 1:
		id := cur.Stocks[0].LocationID
		from = &id
	case len(cur.Stocks) > 1:
		return Outcome{}, &LocationRequiredError{Fields: map[string]string{
			"from_location_id": "required: the item is stocked in several locations"}}
	default:
		fe["from_location_id"] = "item has no stock to transfer"
	}
	if from != nil && req.ToLocationID != nil && *from == *req.ToLocationID {
		fe["to_location_id"] = "must differ from the source location"
	}
	if len(fe) > 0 {
		return Outcome{}, &ValidationError{Fields: fe}
	}
	have := cur.quantityAt(*from)
	if req.Qty > have {
		return Outcome{}, &InsufficientStockError{Available: have, Requested: req.Qty,
			LocationID: from, LocationName: cur.Locations[*from]}
	}
	m.FromLocationID, m.ToLocationID, m.MovedQty = from, req.ToLocationID, req.Qty
	return Outcome{
		Movement: m, StockCurrent: cur.StockCurrent,
		Changes: []StockChange{
			{LocationID: *from, Quantity: have - req.Qty},
			{LocationID: *req.ToLocationID, Quantity: cur.quantityAt(*req.ToLocationID) + req.Qty},
		},
	}, nil
}

func decideConsumable(cur ItemState, req Request, m Movement, note string) (Outcome, error) {
	switch req.Type {
	case TypeIn, TypeOut:
		if req.Qty <= 0 || req.Qty > MaxStock {
			return Outcome{}, FieldError("qty", "must be between 1 and 1000000000")
		}
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
	}

	loc, useDefault, err := cur.resolveLocation(req)
	if err != nil {
		return Outcome{}, err
	}
	var at int64
	have := 0
	if loc != nil {
		at = *loc
		have = cur.quantityAt(at)
		m.LocationID = &at
	}

	var newQty int
	switch req.Type {
	case TypeIn:
		if cur.StockCurrent+req.Qty > MaxStock {
			return Outcome{}, FieldError("qty", "resulting stock would exceed 1000000000")
		}
		m.Qty, newQty = req.Qty, have+req.Qty
	case TypeOut:
		if req.Qty > have {
			ie := &InsufficientStockError{Available: have, Requested: req.Qty}
			if loc != nil {
				ie.LocationID, ie.LocationName = loc, cur.Locations[at]
			}
			return Outcome{}, ie
		}
		m.Qty, newQty = -req.Qty, have-req.Qty
	case TypeAdjust:
		if cur.StockCurrent-have+req.Qty > MaxStock {
			return Outcome{}, FieldError("qty", "resulting stock would exceed 1000000000")
		}
		m.Qty, newQty = req.Qty-have, req.Qty
	}
	m.StockAfter = m.StockBefore + m.Qty
	return Outcome{
		Movement: m, StockCurrent: m.StockAfter, DefaultLocation: useDefault,
		Changes: []StockChange{{LocationID: at, Quantity: newQty}},
	}, nil
}

// resolveLocation picks the location an in/out/adjust applies to. useDefault
// is true when the stock belongs to the default location.
func (s ItemState) resolveLocation(req Request) (loc *int64, useDefault bool, err error) {
	if req.LocationID != nil {
		if !s.locationExists(*req.LocationID) {
			return nil, false, FieldError("location_id", "location does not exist")
		}
		return req.LocationID, false, nil
	}
	switch len(s.Stocks) {
	case 1:
		id := s.Stocks[0].LocationID
		return &id, false, nil
	case 0:
		if req.Type == TypeOut {
			return nil, false, nil // nothing anywhere: reported as insufficient stock
		}
		if s.LastLocationID != nil && s.locationExists(*s.LastLocationID) {
			return s.LastLocationID, false, nil
		}
		return nil, true, nil
	}
	return nil, false, &LocationRequiredError{Fields: map[string]string{
		"location_id": "required: the item is stocked in several locations"}}
}

// Alert is a consumable at or below its minimum stock.
type Alert struct {
	ItemID       string
	Code         string
	Description  string
	Unit         string
	StockCurrent int
	StockMin     int
	// LocationID/LocationName are set only when all the stock sits in one
	// location; Stocks is the full per-location breakdown.
	LocationID   *int64
	LocationName *string
	Stocks       []LocationStock
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
