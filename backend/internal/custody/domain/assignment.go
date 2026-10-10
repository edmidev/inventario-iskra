// Package domain contains the custody rules: who may hold an asset, what a
// loan and a return change, and the errors the adapters translate to HTTP.
package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// Domain errors the HTTP adapter maps to statuses and stable codes.
var (
	ErrItemNotFound     = errors.New("item not found")
	ErrNotApplicable    = errors.New("custody only applies to assets")
	ErrItemNotAvailable = errors.New("item is not functional")
	ErrAlreadyAssigned  = errors.New("item already has an active assignment")
	ErrNotAssigned      = errors.New("item has no active assignment")
	ErrInvalidInput     = errors.New("invalid input")
)

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

// Kind mirrors the catalog item kind; custody keeps its own copy so it does
// not import the catalog domain.
type Kind string

// Item kinds.
const (
	KindAsset      Kind = "asset"
	KindConsumable Kind = "consumable"
)

// Status mirrors the catalog item status.
type Status string

// StatusFunctional is the only status an item may be lent in.
const StatusFunctional Status = "functional"

// PlaceKind mirrors the catalog location kind.
type PlaceKind string

// Location kinds. Only person and container can hold a loan.
const (
	PlaceStorage   PlaceKind = "storage"
	PlaceContainer PlaceKind = "container"
	PlacePerson    PlaceKind = "person"
)

// CanHold reports whether a location of this kind can be a loan assignee.
func (k PlaceKind) CanHold() bool { return k == PlacePerson || k == PlaceContainer }

// Place is a location as custody sees it.
type Place struct {
	ID   int64
	Name string
	Kind PlaceKind
}

// Limits.
const MaxNoteLen = 500

// Assignment is one loan of an asset. It is active while ReturnedAt is nil.
type Assignment struct {
	ID                 int64
	ItemID             string
	ItemCode           string
	ItemDescription    string
	AssigneeLocationID int64
	AssigneeName       string
	AssigneeKind       PlaceKind
	OriginLocationID   *int64
	OriginName         *string
	AssignedAt         time.Time
	DueAt              *time.Time
	ReturnedAt         *time.Time
	ReturnedToID       *int64
	ReturnedToName     *string
	AssignedBy         string
	AssignedByName     string
	ReturnedBy         *string
	ReturnedByName     *string
	AssignNote         string
	ReturnNote         string
	CreatedAt          time.Time
}

// Active reports whether the item is still out.
func (a Assignment) Active() bool { return a.ReturnedAt == nil }

// Overdue reports whether an active assignment is past its due date.
func (a Assignment) Overdue(now time.Time) bool {
	return a.Active() && a.DueAt != nil && a.DueAt.Before(now)
}

// ItemState is what the rules need to know about the locked item.
type ItemState struct {
	Kind       Kind
	Status     Status
	LocationID *int64
	Active     *Assignment // the item's active assignment, if any
	Assignee   *Place      // the requested assignee, nil when it does not exist
	ReturnTo   *Place      // the requested return location, nil when absent or missing
}

// AssignRequest is a client-requested loan.
type AssignRequest struct {
	AssigneeLocationID int64
	DueAt              *time.Time
	Note               string
}

// AssignOutcome is what to persist for an accepted loan.
type AssignOutcome struct {
	Assignment Assignment // item, assignee, origin, times and note; ID and users are filled by the repository
	LocationID *int64     // the item's new location
}

// DecideAssign applies the loan rules. It never mutates anything; the
// repository persists the outcome atomically under a row lock.
func DecideAssign(cur ItemState, req AssignRequest, now time.Time) (AssignOutcome, error) {
	if cur.Kind != KindAsset {
		return AssignOutcome{}, ErrNotApplicable
	}
	fe := map[string]string{}
	note := strings.TrimSpace(req.Note)
	if len(note) > MaxNoteLen {
		fe["note"] = "up to 500 characters"
	}
	if req.DueAt != nil && !req.DueAt.After(now) {
		fe["due_at"] = "must be in the future"
	}
	switch {
	case req.AssigneeLocationID <= 0:
		fe["assignee_location_id"] = "required"
	case cur.Assignee == nil:
		fe["assignee_location_id"] = "location does not exist"
	case !cur.Assignee.Kind.CanHold():
		fe["assignee_location_id"] = "must be a person or a container"
	}
	if len(fe) > 0 {
		return AssignOutcome{}, &ValidationError{Fields: fe}
	}
	if cur.Active != nil {
		return AssignOutcome{}, ErrAlreadyAssigned
	}
	if cur.Status != StatusFunctional {
		return AssignOutcome{}, ErrItemNotAvailable
	}
	a := Assignment{
		AssigneeLocationID: cur.Assignee.ID, AssigneeName: cur.Assignee.Name, AssigneeKind: cur.Assignee.Kind,
		OriginLocationID: cur.LocationID, AssignedAt: now, DueAt: req.DueAt, AssignNote: note,
	}
	loc := cur.Assignee.ID
	return AssignOutcome{Assignment: a, LocationID: &loc}, nil
}

// ReturnRequest is a client-requested return. ReturnLocationID nil means the
// location the item had when it was lent out.
type ReturnRequest struct {
	ReturnLocationID *int64
	Note             string
}

// ReturnOutcome is what to persist for an accepted return.
type ReturnOutcome struct {
	AssignmentID int64
	ReturnedAt   time.Time
	ReturnedTo   *int64 // where the item goes back to; nil leaves it without a location
	ReturnNote   string
}

// DecideReturn applies the return rules.
func DecideReturn(cur ItemState, req ReturnRequest, now time.Time) (ReturnOutcome, error) {
	if cur.Kind != KindAsset {
		return ReturnOutcome{}, ErrNotApplicable
	}
	fe := map[string]string{}
	note := strings.TrimSpace(req.Note)
	if len(note) > MaxNoteLen {
		fe["note"] = "up to 500 characters"
	}
	if req.ReturnLocationID != nil && cur.ReturnTo == nil {
		fe["return_location_id"] = "location does not exist"
	}
	if len(fe) > 0 {
		return ReturnOutcome{}, &ValidationError{Fields: fe}
	}
	if cur.Active == nil {
		return ReturnOutcome{}, ErrNotAssigned
	}
	out := ReturnOutcome{AssignmentID: cur.Active.ID, ReturnedAt: now, ReturnNote: note}
	if now.Before(cur.Active.AssignedAt) { // clock skew between app instances
		out.ReturnedAt = cur.Active.AssignedAt
	}
	switch {
	case req.ReturnLocationID != nil:
		out.ReturnedTo = req.ReturnLocationID
	case cur.Active.OriginLocationID != nil:
		out.ReturnedTo = cur.Active.OriginLocationID
	}
	return out, nil
}

// ActiveFilter selects active loans.
type ActiveFilter struct {
	AssigneeLocationID *int64
	OverdueOnly        bool
	Q                  string // case-insensitive substring over item code and description
	Now                time.Time
}

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
