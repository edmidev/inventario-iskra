// Package domain holds the supplier entity and its business rules.
package domain

import (
	"errors"
	"net/mail"
	"sort"
	"strings"
	"time"
)

// Limits for supplier fields.
const (
	MaxNameLen    = 120
	MaxContactLen = 120
	MaxEmailLen   = 254
	MaxPhoneLen   = 40
	MaxNotesLen   = 1000

	DefaultLimit = 25
	MaxLimit     = 100
)

// Domain errors. The HTTP adapter maps each to a status and stable code.
var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("supplier not found")
	ErrNameTaken    = errors.New("supplier name already in use")
)

// ValidationError is an ErrInvalidInput carrying one message per offending field.
type ValidationError struct {
	Fields map[string]string
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

// Supplier is a company or person items are bought from.
type Supplier struct {
	ID          int64
	Name        string
	ContactName string
	Email       string
	Phone       string
	Notes       string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Input is the editable part of a supplier.
type Input struct {
	Name        string
	ContactName string
	Email       string
	Phone       string
	Notes       string
	Active      bool
}

// Normalize trims every field and validates the result.
func (in Input) Normalize() (Input, error) {
	out := Input{
		Name:        strings.TrimSpace(in.Name),
		ContactName: strings.TrimSpace(in.ContactName),
		Email:       strings.TrimSpace(in.Email),
		Phone:       strings.TrimSpace(in.Phone),
		Notes:       strings.TrimSpace(in.Notes),
		Active:      in.Active,
	}
	fe := map[string]string{}
	if out.Name == "" || len(out.Name) > MaxNameLen {
		fe["name"] = "required, up to 120 characters"
	}
	if len(out.ContactName) > MaxContactLen {
		fe["contact_name"] = "up to 120 characters"
	}
	if out.Email != "" {
		if _, err := mail.ParseAddress(out.Email); err != nil || len(out.Email) > MaxEmailLen {
			fe["email"] = "must be a valid email address"
		}
	}
	if len(out.Phone) > MaxPhoneLen {
		fe["phone"] = "up to 40 characters"
	}
	if len(out.Notes) > MaxNotesLen {
		fe["notes"] = "up to 1000 characters"
	}
	if len(fe) > 0 {
		return Input{}, &ValidationError{Fields: fe}
	}
	return out, nil
}

// Filter narrows a supplier listing.
type Filter struct {
	// Q matches the name, contact name or email, case-insensitively.
	Q string
	// Active, when set, keeps only active or only inactive suppliers.
	Active *bool
	Limit  int
	Offset int
}

// Normalize applies the pagination defaults and validates the bounds.
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
	if len(fe) > 0 {
		return Filter{}, &ValidationError{Fields: fe}
	}
	f.Q = strings.TrimSpace(f.Q)
	return f, nil
}
