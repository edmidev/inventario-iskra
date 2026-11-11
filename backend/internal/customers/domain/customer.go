// Package domain defines customer identity and validation.
package domain

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidInput = errors.New("invalid input")
var ErrNotFound = errors.New("customer not found")
var ErrNameTaken = errors.New("customer name already in use")

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string        { return "invalid customer input" }
func (e *ValidationError) Is(target error) bool { return target == ErrInvalidInput }

type Input struct {
	Name, Identification, Contact, Notes string
	Active                               bool
}
type Customer struct {
	ID int64
	Input
	CreatedAt, UpdatedAt time.Time
}

func (in Input) Normalize() (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Identification = strings.TrimSpace(in.Identification)
	in.Contact = strings.TrimSpace(in.Contact)
	in.Notes = strings.TrimSpace(in.Notes)
	f := map[string]string{}
	if in.Name == "" || len(in.Name) > 120 {
		f["name"] = "required, up to 120 characters"
	}
	if len(in.Identification) > 120 {
		f["identification"] = "up to 120 characters"
	}
	if len(in.Contact) > 254 {
		f["contact"] = "up to 254 characters"
	}
	if len(in.Notes) > 1000 {
		f["notes"] = "up to 1000 characters"
	}
	if len(f) > 0 {
		return Input{}, &ValidationError{f}
	}
	return in, nil
}

type Filter struct {
	Q             string
	Active        *bool
	Limit, Offset int
}

func (f Filter) Normalize() (Filter, error) {
	if f.Limit == 0 {
		f.Limit = 25
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		return f, &ValidationError{map[string]string{"limit": "must be 1 to 100 and offset nonnegative"}}
	}
	f.Q = strings.TrimSpace(f.Q)
	return f, nil
}
