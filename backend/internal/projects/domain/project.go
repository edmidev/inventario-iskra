// Package domain defines reusable manufacturing project templates.
package domain

import (
	"errors"
	"fmt"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	"strings"
	"time"
)

var ErrNotFound = errors.New("project not found")
var ErrInvalidInput = errors.New("invalid project input")
var ErrNameTaken = errors.New("project name already in use")

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string   { return "invalid project input" }
func (e *ValidationError) Is(t error) bool { return t == ErrInvalidInput }

// Material is a consumable quantity needed to manufacture one unit.
type Material struct {
	ItemID   string            `json:"item_id"`
	Quantity quantity.Quantity `json:"quantity"`
}

// Input is a complete editable template, never a live execution.
type Input struct {
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Characteristics []string   `json:"characteristics"`
	Consumables     []Material `json:"consumables"`
	Tools           []string   `json:"tools"`
	EstimatedDays   int        `json:"estimated_days"`
}

// Project retains references to the exact inventory entries selected by the author.
type Project struct {
	ID int64 `json:"id"`
	Input
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (in Input) Normalize() (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	f := map[string]string{}
	if in.Name == "" || len(in.Name) > 120 {
		f["name"] = "required, up to 120 characters"
	}
	if len(in.Description) > 4000 {
		f["description"] = "up to 4000 characters"
	}
	if in.EstimatedDays < 1 || in.EstimatedDays > 36500 {
		f["estimated_days"] = "must be between 1 and 36500 days"
	}
	in.Characteristics = append([]string{}, in.Characteristics...)
	for i, v := range in.Characteristics {
		v = strings.TrimSpace(v)
		if v == "" || len(v) > 500 {
			f[fmt.Sprintf("characteristics[%d]", i)] = "required, up to 500 characters"
		}
		in.Characteristics[i] = v
	}
	in.Consumables = append([]Material{}, in.Consumables...)
	seen := map[string]bool{}
	for i, m := range in.Consumables {
		key := fmt.Sprintf("consumables[%d]", i)
		if m.ItemID == "" || seen[m.ItemID] {
			f[key+".item_id"] = "must be a distinct inventory item"
		}
		seen[m.ItemID] = true
		if m.Quantity <= 0 {
			f[key+".quantity"] = "must be greater than zero"
		}
	}
	in.Tools = append([]string{}, in.Tools...)
	seen = map[string]bool{}
	for i, id := range in.Tools {
		if id == "" || seen[id] {
			f[fmt.Sprintf("tools[%d]", i)] = "must be a distinct inventory tool"
		}
		seen[id] = true
	}
	if len(f) > 0 {
		return Input{}, &ValidationError{f}
	}
	return in, nil
}

// Clone prevents a caller from mutating stored template slices.
func (p Project) Clone() Project {
	p.Characteristics = append([]string{}, p.Characteristics...)
	p.Tools = append([]string{}, p.Tools...)
	p.Consumables = append([]Material{}, p.Consumables...)
	return p
}

type Filter struct {
	Q             string
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
