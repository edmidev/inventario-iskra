// Package domain contains the maintenance rules: how a failure is reported,
// repaired and closed, how a preventive service is registered, and the errors
// the adapters translate to HTTP.
package domain

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

// Domain errors the HTTP adapter maps to statuses and stable codes.
var (
	ErrItemNotFound      = errors.New("item not found")
	ErrRecordNotFound    = errors.New("maintenance record not found")
	ErrNotApplicable     = errors.New("maintenance only applies to assets")
	ErrOpenFailureExists = errors.New("item already has an open failure")
	ErrRecordClosed      = errors.New("maintenance record is closed")
	ErrInvalidInput      = errors.New("invalid input")
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

// Kind is the type of maintenance event.
type Kind string

// Record kinds.
const (
	KindPreventive Kind = "preventive"
	KindCorrective Kind = "corrective"
)

// Valid reports whether k is a stored kind.
func (k Kind) Valid() bool { return k == KindPreventive || k == KindCorrective }

// Status is the lifecycle state of a record.
type Status string

// Record statuses.
const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
)

// Valid reports whether s is a stored status.
func (s Status) Valid() bool { return s == StatusOpen || s == StatusClosed }

// ItemKind mirrors the catalog item kind; maintenance keeps its own copy so
// it does not import the catalog domain.
type ItemKind string

// Item kinds.
const (
	ItemAsset      ItemKind = "asset"
	ItemConsumable ItemKind = "consumable"
)

// ItemStatus mirrors the catalog item status.
type ItemStatus string

// Item statuses.
const (
	ItemFunctional  ItemStatus = "functional"
	ItemInoperative ItemStatus = "inoperative"
	ItemInRepair    ItemStatus = "in_repair"
)

// Limits.
const (
	MaxDescriptionLen = 1000
	MaxDestinationLen = 120
	MaxNoteLen        = 1000
	MaxCost           = 9_999_999_999.99 // NUMERIC(12,2)
	MaxDueWithinDays  = 365
)

// Record is one maintenance event of an asset. A corrective record is open
// from the failure report until it is closed; a preventive record is created
// closed.
type Record struct {
	ID              int64
	ItemID          string
	ItemCode        string
	ItemDescription string
	ItemStatus      ItemStatus // the item's current status
	Kind            Kind
	Status          Status
	ReportedAt      time.Time // failure date (corrective) or date performed (preventive)
	Description     string
	Destination     *string
	Cost            *float64
	NextDueAt       *time.Time
	ClosedAt        *time.Time
	ResolutionNote  *string
	ReportedBy      string
	ReportedByName  string
	ClosedBy        *string
	ClosedByName    *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ItemState is what the rules need to know about the locked item.
type ItemState struct {
	Kind              ItemKind
	Status            ItemStatus
	IntervalDays      *int       // maintenance_interval_days
	LastMaintenanceAt *time.Time // items.last_maintenance_at
	Open              *Record    // the item's open corrective record, if any
}

// AddDays adds whole 24-hour days; the SQL side uses the same arithmetic.
func AddDays(t time.Time, days int) time.Time { return t.Add(time.Duration(days) * 24 * time.Hour) }

func laterOf(cur *time.Time, t time.Time) time.Time {
	if cur != nil && cur.After(t) {
		return *cur
	}
	return t
}

// normText trims s and reports whether anything is left.
func normText(s string) (string, bool) {
	s = strings.TrimSpace(s)
	return s, s != ""
}

// normDestination trims the destination; blank means none.
func normDestination(d *string, fe map[string]string) *string {
	if d == nil {
		return nil
	}
	v, ok := normText(*d)
	if !ok {
		return nil
	}
	if len([]rune(v)) > MaxDestinationLen {
		fe["destination"] = "up to 120 characters"
		return nil
	}
	return &v
}

func checkDescription(s string, fe map[string]string) string {
	v, ok := normText(s)
	switch {
	case !ok:
		fe["description"] = "required"
	case len([]rune(v)) > MaxDescriptionLen:
		fe["description"] = "up to 1000 characters"
	}
	return v
}

func checkCost(c *float64, fe map[string]string) *float64 {
	if c == nil {
		return nil
	}
	if math.IsNaN(*c) || math.IsInf(*c, 0) || *c < 0 || *c > MaxCost {
		fe["cost"] = "must be between 0 and 9999999999.99"
		return nil
	}
	v := math.Round(*c*100) / 100
	return &v
}

// CreateRequest is the client payload of POST /items/{id}/maintenance. Kind
// selects between reporting a failure and registering a preventive service;
// fields that belong to the other kind are rejected.
type CreateRequest struct {
	Kind        Kind
	Description string
	ReportedAt  *time.Time // corrective: failure date
	Destination *string    // corrective
	ItemStatus  ItemStatus // corrective: inoperative (default) or in_repair
	PerformedAt *time.Time // preventive: date performed
	Cost        *float64   // preventive
	NextDueAt   *time.Time // preventive
}

// Split validates Kind and the kind-specific fields and returns the matching
// typed request.
func (c CreateRequest) Split() (ReportRequest, PreventiveRequest, error) {
	fe := map[string]string{}
	reject := func(name string, present bool) {
		if present {
			fe[name] = "not allowed for kind " + string(c.Kind)
		}
	}
	switch c.Kind {
	case KindCorrective:
		reject("performed_at", c.PerformedAt != nil)
		reject("cost", c.Cost != nil)
		reject("next_due_at", c.NextDueAt != nil)
	case KindPreventive:
		reject("reported_at", c.ReportedAt != nil)
		reject("destination", c.Destination != nil)
		reject("item_status", c.ItemStatus != "")
	default:
		return ReportRequest{}, PreventiveRequest{}, FieldError("kind", "must be preventive or corrective")
	}
	if len(fe) > 0 {
		return ReportRequest{}, PreventiveRequest{}, &ValidationError{Fields: fe}
	}
	return ReportRequest{ReportedAt: c.ReportedAt, Description: c.Description, Destination: c.Destination, ItemStatus: c.ItemStatus},
		PreventiveRequest{PerformedAt: c.PerformedAt, Description: c.Description, Cost: c.Cost, NextDueAt: c.NextDueAt}, nil
}

// ReportRequest is a failure report. ItemStatus is the status the item takes:
// inoperative (default) or in_repair.
type ReportRequest struct {
	ReportedAt  *time.Time
	Description string
	Destination *string
	ItemStatus  ItemStatus
}

// ReportOutcome is what to persist for an accepted failure report.
type ReportOutcome struct {
	Record     Record // item, kind, status, times, text; ID and users are filled by the repository
	ItemStatus ItemStatus
}

// DecideReport applies the failure-report rules. It never mutates anything;
// the repository persists the outcome atomically under the item row lock.
func DecideReport(cur ItemState, req ReportRequest, now time.Time) (ReportOutcome, error) {
	if cur.Kind != ItemAsset {
		return ReportOutcome{}, ErrNotApplicable
	}
	fe := map[string]string{}
	desc := checkDescription(req.Description, fe)
	dest := normDestination(req.Destination, fe)
	reportedAt := now
	if req.ReportedAt != nil {
		reportedAt = *req.ReportedAt
		if reportedAt.After(now) {
			fe["reported_at"] = "cannot be in the future"
		}
	}
	status := req.ItemStatus
	switch status {
	case "":
		status = ItemInoperative
	case ItemInoperative, ItemInRepair:
	default:
		fe["item_status"] = "must be inoperative or in_repair"
	}
	if len(fe) > 0 {
		return ReportOutcome{}, &ValidationError{Fields: fe}
	}
	if cur.Open != nil {
		return ReportOutcome{}, ErrOpenFailureExists
	}
	return ReportOutcome{
		Record:     Record{Kind: KindCorrective, Status: StatusOpen, ReportedAt: reportedAt, Description: desc, Destination: dest},
		ItemStatus: status,
	}, nil
}

// PreventiveRequest registers a preventive service already performed.
// PerformedAt defaults to now. NextDueAt defaults to PerformedAt plus the
// item's maintenance interval, when it has one.
type PreventiveRequest struct {
	PerformedAt *time.Time
	Description string
	Cost        *float64
	NextDueAt   *time.Time
}

// PreventiveOutcome is what to persist for an accepted preventive service.
// The item status is deliberately untouched.
type PreventiveOutcome struct {
	Record            Record
	LastMaintenanceAt time.Time // the item's new last_maintenance_at (never moves backwards)
}

// DecidePreventive applies the preventive-registration rules.
func DecidePreventive(cur ItemState, req PreventiveRequest, now time.Time) (PreventiveOutcome, error) {
	if cur.Kind != ItemAsset {
		return PreventiveOutcome{}, ErrNotApplicable
	}
	fe := map[string]string{}
	desc := checkDescription(req.Description, fe)
	cost := checkCost(req.Cost, fe)
	performed := now
	if req.PerformedAt != nil {
		performed = *req.PerformedAt
		if performed.After(now) {
			fe["performed_at"] = "cannot be in the future"
		}
	}
	next := req.NextDueAt
	switch {
	case next != nil && !next.After(performed):
		fe["next_due_at"] = "must be after performed_at"
	case next == nil && cur.IntervalDays != nil:
		d := AddDays(performed, *cur.IntervalDays)
		next = &d
	}
	if len(fe) > 0 {
		return PreventiveOutcome{}, &ValidationError{Fields: fe}
	}
	return PreventiveOutcome{
		Record: Record{Kind: KindPreventive, Status: StatusClosed, ReportedAt: performed, ClosedAt: &performed,
			Description: desc, Cost: cost, NextDueAt: next},
		LastMaintenanceAt: laterOf(cur.LastMaintenanceAt, performed),
	}, nil
}

// UpdateRequest edits an open record. Nil / unset fields stay as they are;
// SetDestination with a nil or blank Destination clears it.
type UpdateRequest struct {
	Description    *string
	SetDestination bool
	Destination    *string
	ItemStatus     ItemStatus // inoperative or in_repair; empty keeps the item as is
}

// UpdateOutcome is what to persist for an accepted edit.
type UpdateOutcome struct {
	Record     Record     // the record with the edits applied
	ItemStatus ItemStatus // empty when the item status is untouched
}

// DecideUpdate applies the edit rules to an open record.
func DecideUpdate(cur ItemState, rec Record, req UpdateRequest) (UpdateOutcome, error) {
	if cur.Kind != ItemAsset {
		return UpdateOutcome{}, ErrNotApplicable
	}
	fe := map[string]string{}
	if req.Description == nil && !req.SetDestination && req.ItemStatus == "" {
		fe["body"] = "at least one of description, destination or item_status is required"
	}
	out := UpdateOutcome{Record: rec}
	if req.Description != nil {
		out.Record.Description = checkDescription(*req.Description, fe)
	}
	if req.SetDestination {
		out.Record.Destination = normDestination(req.Destination, fe)
	}
	switch req.ItemStatus {
	case "":
	case ItemInoperative, ItemInRepair:
		out.ItemStatus = req.ItemStatus
	default:
		fe["item_status"] = "must be inoperative or in_repair"
	}
	if len(fe) > 0 {
		return UpdateOutcome{}, &ValidationError{Fields: fe}
	}
	if rec.Status != StatusOpen {
		return UpdateOutcome{}, ErrRecordClosed
	}
	return out, nil
}

// CloseRequest closes an open corrective record. ClosedAt defaults to now;
// ItemStatusAfter defaults to functional.
type CloseRequest struct {
	ClosedAt        *time.Time
	Cost            *float64
	ResolutionNote  string
	ItemStatusAfter ItemStatus // functional (default) or inoperative
}

// CloseOutcome is what to persist for an accepted close.
type CloseOutcome struct {
	Record            Record
	ItemStatus        ItemStatus
	LastMaintenanceAt *time.Time // set only when the item comes back functional
}

// DecideClose applies the close rules.
func DecideClose(cur ItemState, rec Record, req CloseRequest, now time.Time) (CloseOutcome, error) {
	if cur.Kind != ItemAsset {
		return CloseOutcome{}, ErrNotApplicable
	}
	fe := map[string]string{}
	cost := checkCost(req.Cost, fe)
	note, hasNote := normText(req.ResolutionNote)
	if len([]rune(note)) > MaxNoteLen {
		fe["resolution_note"] = "up to 1000 characters"
	}
	closedAt := now
	if now.Before(rec.ReportedAt) { // clock skew between app instances
		closedAt = rec.ReportedAt
	}
	if req.ClosedAt != nil {
		closedAt = *req.ClosedAt
		switch {
		case closedAt.After(now):
			fe["closed_at"] = "cannot be in the future"
		case closedAt.Before(rec.ReportedAt):
			fe["closed_at"] = "cannot be before the failure was reported"
		}
	}
	after := req.ItemStatusAfter
	switch after {
	case "":
		after = ItemFunctional
	case ItemFunctional, ItemInoperative:
	default:
		fe["item_status_after"] = "must be functional or inoperative"
	}
	if len(fe) > 0 {
		return CloseOutcome{}, &ValidationError{Fields: fe}
	}
	if rec.Status != StatusOpen {
		return CloseOutcome{}, ErrRecordClosed
	}
	out := CloseOutcome{Record: rec, ItemStatus: after}
	out.Record.Status = StatusClosed
	out.Record.ClosedAt = &closedAt
	out.Record.Cost = cost
	if hasNote {
		out.Record.ResolutionNote = &note
	}
	if after == ItemFunctional {
		t := laterOf(cur.LastMaintenanceAt, closedAt)
		out.LastMaintenanceAt = &t
	}
	return out, nil
}

// ListFilter selects records for the global listing.
type ListFilter struct {
	Status      Status
	Kind        Kind
	Destination string // exact, case-insensitive
	ItemID      string
	Q           string // case-insensitive substring over item code, item description and record description
}

// DueFilter selects assets whose preventive maintenance is overdue or due
// within WithinDays of Now.
type DueFilter struct {
	CategoryID *int64
	Q          string
	WithinDays int
	Now        time.Time
}

// DueItem is an asset with a maintenance interval, with its next due date.
type DueItem struct {
	ItemID            string
	Code              string
	Description       string
	CategoryID        int64
	CategoryName      string
	Status            ItemStatus
	LocationID        *int64
	LocationName      *string
	LastMaintenanceAt *time.Time
	IntervalDays      int
	DueAt             time.Time
}

// Overdue reports whether the maintenance date has passed at now.
func (d DueItem) Overdue(now time.Time) bool { return d.DueAt.Before(now) }

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
