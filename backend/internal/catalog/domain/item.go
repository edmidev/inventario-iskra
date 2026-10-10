package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Kind distinguishes unique assets from quantity-tracked consumables.
type Kind string

// Supported kinds.
const (
	KindAsset      Kind = "asset"
	KindConsumable Kind = "consumable"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k == KindAsset || k == KindConsumable }

// Status is the physical condition of an item.
type Status string

// Supported statuses.
const (
	StatusFunctional  Status = "functional"
	StatusInoperative Status = "inoperative"
	StatusInRepair    Status = "in_repair"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	return s == StatusFunctional || s == StatusInoperative || s == StatusInRepair
}

// Validation limits.
const (
	MaxDescriptionLen = 500
	MaxUnitLen        = 30
	MaxNotesLen       = 4000
	MaxCodeLen        = 64
	MaxAttributesSize = 8 * 1024 // bytes of marshalled JSON
	MaxIntervalDays   = 3650
	MaxStock          = 1_000_000_000
)

var codeRe = regexp.MustCompile(`^ISK-[A-Z0-9]+(-[A-Z0-9]+)*$`)

// ValidateCode checks a caller-supplied item code.
func ValidateCode(code string) error {
	if len(code) > MaxCodeLen || !codeRe.MatchString(code) {
		return FieldError("code", "must look like ISK-ABC-001: uppercase letters and digits in dash-separated groups, up to 64 characters")
	}
	return nil
}

// GenerateCode builds the code for sequence number seq under a category prefix.
func GenerateCode(prefix string, seq int) string {
	return fmt.Sprintf("ISK-%s-%03d", prefix, seq)
}

// Item is one inventory entry: a unique asset or a consumable stock line.
type Item struct {
	ID                      string
	Code                    string
	Description             string
	CategoryID              int64
	Kind                    Kind
	Status                  Status
	Unit                    *string
	StockCurrent            int
	StockMin                int
	LocationID              *int64
	LastMaintenanceAt       *time.Time
	MaintenanceIntervalDays *int
	Attributes              map[string]any
	Notes                   string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// LowStock reports whether a consumable is at or below a configured minimum.
func (i Item) LowStock() bool {
	return i.Kind == KindConsumable && i.StockMin > 0 && i.StockCurrent <= i.StockMin
}

// Availability says whether an item can be handed out right now.
type Availability string

// Derived availability values. Never stored.
const (
	Available   Availability = "available"
	Unavailable Availability = "unavailable"
)

// Valid reports whether a is a known availability.
func (a Availability) Valid() bool { return a == Available || a == Unavailable }

// AvailabilityInputs are the facts availability is derived from. Custody (T4)
// fills ActiveLoan; until then no item is ever on loan.
type AvailabilityInputs struct {
	Status     Status
	ActiveLoan bool
}

// DeriveAvailability is the single place availability is decided.
func DeriveAvailability(in AvailabilityInputs) Availability {
	if in.Status == StatusFunctional && !in.ActiveLoan {
		return Available
	}
	return Unavailable
}

// Availability derives the item's availability from its current facts.
func (i Item) Availability() Availability {
	return DeriveAvailability(AvailabilityInputs{Status: i.Status})
}

// StatusesFor returns the statuses compatible with availability a, so list
// queries filter with the same rule as DeriveAvailability. Custody extends
// this with a loan join; the API shape stays the same.
func StatusesFor(a Availability) []Status {
	if a == Available {
		return []Status{StatusFunctional}
	}
	return []Status{StatusInoperative, StatusInRepair}
}

// NewItemInput carries the creation fields (the code is decided by the app).
type NewItemInput struct {
	Description             string
	CategoryID              int64
	Kind                    Kind
	Status                  Status // empty means functional
	Unit                    *string
	StockCurrent            *int // opening balance, consumables only
	StockMin                int
	LocationID              *int64
	LastMaintenanceAt       *time.Time
	MaintenanceIntervalDays *int
	Attributes              map[string]any
	Notes                   string
}

// NewItem validates the input against the kind rules and builds an Item
// (without code or timestamps).
func NewItem(in NewItemInput) (Item, error) {
	fe := fieldErrors{}
	it := Item{
		Description: strings.TrimSpace(in.Description), CategoryID: in.CategoryID, Kind: in.Kind,
		Status: in.Status, StockMin: in.StockMin, LocationID: in.LocationID,
		LastMaintenanceAt: in.LastMaintenanceAt, MaintenanceIntervalDays: in.MaintenanceIntervalDays,
		Attributes: in.Attributes, Notes: strings.TrimSpace(in.Notes),
	}
	if it.Status == "" {
		it.Status = StatusFunctional
	}
	checkCommon(fe, &it)
	if in.CategoryID <= 0 {
		fe.add("category_id", "required")
	}
	switch in.Kind {
	case KindAsset:
		if in.Unit != nil && strings.TrimSpace(*in.Unit) != "" {
			fe.add("unit", "assets have no unit")
		}
		if in.StockCurrent != nil && *in.StockCurrent != 1 {
			fe.add("stock_current", "assets always have stock 1")
		}
		if in.StockMin != 0 {
			fe.add("stock_min", "assets have no minimum stock")
		}
		it.Unit, it.StockCurrent, it.StockMin = nil, 1, 0
	case KindConsumable:
		it.Unit = normalizeUnit(fe, in.Unit)
		if in.StockCurrent != nil {
			if *in.StockCurrent < 0 || *in.StockCurrent > MaxStock {
				fe.add("stock_current", "must be between 0 and 1000000000")
			}
			it.StockCurrent = *in.StockCurrent
		}
	default:
		fe.add("kind", "must be asset or consumable")
	}
	if err := fe.err(); err != nil {
		return Item{}, err
	}
	if it.Attributes == nil {
		it.Attributes = map[string]any{}
	}
	return it, nil
}

func normalizeUnit(fe fieldErrors, unit *string) *string {
	if unit == nil || strings.TrimSpace(*unit) == "" || len(strings.TrimSpace(*unit)) > MaxUnitLen {
		fe.add("unit", "required for consumables, up to 30 characters")
		return nil
	}
	u := strings.TrimSpace(*unit)
	return &u
}

// checkCommon validates the fields shared by create and update.
func checkCommon(fe fieldErrors, it *Item) {
	if it.Description == "" || len(it.Description) > MaxDescriptionLen {
		fe.add("description", "required, up to 500 characters")
	}
	if !it.Status.Valid() {
		fe.add("status", "must be functional, inoperative or in_repair")
	}
	if it.StockMin < 0 || it.StockMin > MaxStock {
		fe.add("stock_min", "must be between 0 and 1000000000")
	}
	if d := it.MaintenanceIntervalDays; d != nil && (*d <= 0 || *d > MaxIntervalDays) {
		fe.add("maintenance_interval_days", "must be between 1 and 3650")
	}
	if len(it.Notes) > MaxNotesLen {
		fe.add("notes", "up to 4000 characters")
	}
	if it.Attributes != nil {
		b, err := json.Marshal(it.Attributes)
		if err != nil || len(b) > MaxAttributesSize {
			fe.add("attributes", "must be a JSON object up to 8 KB")
		}
	}
}

// Nullable is an optional JSON field that distinguishes "absent" from
// "explicit null" (used to clear a value in a PATCH).
type Nullable[T any] struct {
	Set   bool
	Null  bool
	Value T
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Null = true
		return nil
	}
	return json.Unmarshal(b, &n.Value)
}

// ItemPatch is a partial update. Code, Kind and StockCurrent are accepted
// only so that attempts to change them are rejected with a clear message.
type ItemPatch struct {
	Description             *string
	CategoryID              *int64
	Status                  *Status
	Unit                    *string
	StockMin                *int
	LocationID              Nullable[int64]
	MaintenanceIntervalDays Nullable[int]
	Attributes              map[string]any // replaces the whole object when non-nil
	Notes                   *string

	Code         *string
	Kind         *Kind
	StockCurrent *int
}

// Apply validates the patch and returns the updated item; i is not modified.
func (i Item) Apply(p ItemPatch) (Item, error) {
	fe := fieldErrors{}
	if p.Code != nil && *p.Code != i.Code {
		fe.add("code", "immutable after creation")
	}
	if p.Kind != nil && *p.Kind != i.Kind {
		fe.add("kind", "immutable after creation")
	}
	if p.StockCurrent != nil {
		fe.add("stock_current", "cannot be edited here; use stock movements")
	}
	out := i
	if p.Description != nil {
		out.Description = strings.TrimSpace(*p.Description)
	}
	if p.CategoryID != nil {
		out.CategoryID = *p.CategoryID
		if *p.CategoryID <= 0 {
			fe.add("category_id", "invalid")
		}
	}
	if p.Status != nil {
		out.Status = *p.Status
	}
	if p.StockMin != nil {
		out.StockMin = *p.StockMin
	}
	if p.LocationID.Set {
		out.LocationID = nil
		if !p.LocationID.Null {
			v := p.LocationID.Value
			out.LocationID = &v
		}
	}
	if p.MaintenanceIntervalDays.Set {
		out.MaintenanceIntervalDays = nil
		if !p.MaintenanceIntervalDays.Null {
			v := p.MaintenanceIntervalDays.Value
			out.MaintenanceIntervalDays = &v
		}
	}
	if p.Attributes != nil {
		out.Attributes = p.Attributes
	}
	if p.Notes != nil {
		out.Notes = strings.TrimSpace(*p.Notes)
	}
	checkCommon(fe, &out)
	switch i.Kind {
	case KindAsset:
		if p.Unit != nil && strings.TrimSpace(*p.Unit) != "" {
			fe.add("unit", "assets have no unit")
		}
		if p.StockMin != nil && *p.StockMin != 0 {
			fe.add("stock_min", "assets have no minimum stock")
		}
	case KindConsumable:
		if p.Unit != nil {
			out.Unit = normalizeUnit(fe, p.Unit)
		}
	}
	if err := fe.err(); err != nil {
		return Item{}, err
	}
	return out, nil
}
