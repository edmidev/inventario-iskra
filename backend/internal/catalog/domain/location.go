package domain

import "strings"

// LocationKind says what a location physically is.
type LocationKind string

// Supported location kinds.
const (
	LocationStorage   LocationKind = "storage"
	LocationContainer LocationKind = "container"
	LocationPerson    LocationKind = "person"
)

// Valid reports whether k is a known kind.
func (k LocationKind) Valid() bool {
	return k == LocationStorage || k == LocationContainer || k == LocationPerson
}

// Location is a place where items live: storage room, container (a case) or
// a person holding them.
type Location struct {
	ID       int64
	Name     string
	Kind     LocationKind
	ParentID *int64
}

// NormalizeLocation validates name and kind.
func NormalizeLocation(name string, kind LocationKind) (string, error) {
	fe := fieldErrors{}
	n := strings.TrimSpace(name)
	if n == "" || len(n) > MaxNameLen {
		fe.add("name", "required, up to 120 characters")
	}
	if !kind.Valid() {
		fe.add("kind", "must be one of storage, container, person")
	}
	if err := fe.err(); err != nil {
		return "", err
	}
	return n, nil
}

// CheckNoCycle verifies that making newParent the parent of id creates no
// cycle, given every location's parent (parents maps id -> parent id).
func CheckNoCycle(id int64, newParent *int64, parents map[int64]*int64) error {
	if newParent == nil {
		return nil
	}
	seen := map[int64]bool{}
	for cur := newParent; cur != nil; cur = parents[*cur] {
		if *cur == id || seen[*cur] {
			return ErrLocationCycle
		}
		seen[*cur] = true
	}
	return nil
}
