// Package domain holds the catalog entities (categories, locations, items)
// and their business rules.
package domain

import (
	"errors"
	"sort"
	"strings"
)

// Domain errors. The HTTP adapter maps each to a status and stable code.
var (
	ErrInvalidInput = errors.New("invalid input")

	ErrCategoryNotFound    = errors.New("category not found")
	ErrCategoryNameTaken   = errors.New("category name already in use")
	ErrCategoryPrefixTaken = errors.New("category code prefix already in use")
	ErrCategoryInUse       = errors.New("category still has items")

	ErrLocationNotFound = errors.New("location not found")
	ErrLocationCycle    = errors.New("location parent would create a cycle")
	ErrLocationInUse    = errors.New("location still has child locations or items")

	ErrItemNotFound = errors.New("item not found")
	ErrCodeTaken    = errors.New("item code already in use")
)

// ValidationError is an ErrInvalidInput carrying one message per offending
// field, so clients can highlight the exact inputs.
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

// fieldErrors collects field messages and yields a ValidationError, or nil.
type fieldErrors map[string]string

func (f fieldErrors) add(field, msg string) {
	if _, dup := f[field]; !dup {
		f[field] = msg
	}
}

func (f fieldErrors) err() error {
	if len(f) == 0 {
		return nil
	}
	return &ValidationError{Fields: f}
}

// FieldError builds a single-field ValidationError.
func FieldError(field, msg string) error {
	return &ValidationError{Fields: map[string]string{field: msg}}
}
