package domain

import "strings"

// Sort fields accepted by item lists.
const (
	SortCode        = "code"
	SortDescription = "description"
	SortUpdatedAt   = "updated_at"
)

// Pagination bounds.
const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// ItemFilter selects and orders a page of items. Deleted items are never listed.
type ItemFilter struct {
	Q            string // case-insensitive substring over code and description
	CategoryID   *int64
	Status       Status
	Kind         Kind
	Availability Availability
	LocationID   *int64
	LowStock     bool
	Sort         string // code (default), description, updated_at
	Desc         bool
	Limit        int
	Offset       int
}

// Normalize validates the filter and applies defaults.
func (f ItemFilter) Normalize() (ItemFilter, error) {
	fe := fieldErrors{}
	f.Q = strings.TrimSpace(f.Q)
	if f.Status != "" && !f.Status.Valid() {
		fe.add("status", "must be functional, inoperative or in_repair")
	}
	if f.Kind != "" && !f.Kind.Valid() {
		fe.add("kind", "must be asset or consumable")
	}
	if f.Availability != "" && !f.Availability.Valid() {
		fe.add("availability", "must be available or unavailable")
	}
	switch f.Sort {
	case "":
		f.Sort = SortCode
	case SortCode, SortDescription, SortUpdatedAt:
	default:
		fe.add("sort", "must be code, description or updated_at")
	}
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit < 0 || f.Limit > MaxLimit {
		fe.add("limit", "must be between 1 and 100")
	}
	if f.Offset < 0 {
		fe.add("offset", "must be 0 or greater")
	}
	if err := fe.err(); err != nil {
		return ItemFilter{}, err
	}
	return f, nil
}

// Matches reports whether it satisfies the filter's predicates (not sorting
// or paging). In-memory repositories use it; the SQL repository mirrors it.
func (f ItemFilter) Matches(it Item) bool {
	if q := strings.ToLower(f.Q); q != "" &&
		!strings.Contains(strings.ToLower(it.Code), q) && !strings.Contains(strings.ToLower(it.Description), q) {
		return false
	}
	if f.CategoryID != nil && it.CategoryID != *f.CategoryID {
		return false
	}
	if f.Status != "" && it.Status != f.Status {
		return false
	}
	if f.Kind != "" && it.Kind != f.Kind {
		return false
	}
	if f.Availability != "" && it.Availability() != f.Availability {
		return false
	}
	if f.LocationID != nil && !it.InLocation(*f.LocationID) {
		return false
	}
	if f.LowStock && !it.LowStock() {
		return false
	}
	return true
}
