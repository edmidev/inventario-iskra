// Package memory provides an in-memory custody repository. It backs unit
// tests of the use cases and the HTTP adapter; it is not used in production.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/edgar/inventario/backend/internal/custody/domain"
)

// Item is a seeded item row.
type Item struct {
	ID          string
	Code        string
	Description string
	Kind        domain.Kind
	Status      domain.Status
	LocationID  *int64
	Deleted     bool
}

// Store implements app.AssignmentRepository.
type Store struct {
	mu     sync.Mutex
	items  map[string]*Item
	places map[int64]domain.Place
	users  map[string]string
	rows   []domain.Assignment
	nextID int64
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{items: map[string]*Item{}, places: map[int64]domain.Place{}, users: map[string]string{}}
}

// SeedPlace adds a location.
func (s *Store) SeedPlace(id int64, name string, kind domain.PlaceKind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.places[id] = domain.Place{ID: id, Name: name, Kind: kind}
}

// SeedUser adds a user name for display.
func (s *Store) SeedUser(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[id] = name
}

// SeedItem adds an item. Empty Status defaults to functional.
func (s *Store) SeedItem(it Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it.Status == "" {
		it.Status = domain.StatusFunctional
	}
	s.items[it.ID] = &it
}

// SetStatus changes an item's status (test helper).
func (s *Store) SetStatus(id string, st domain.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id].Status = st
}

// ItemLocation returns an item's current location (test helper).
func (s *Store) ItemLocation(id string) *int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[id].LocationID
}

// Rows returns every stored assignment (test helper).
func (s *Store) Rows() []domain.Assignment {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Assignment(nil), s.rows...)
}

func (s *Store) activeIndex(itemID string) int {
	for i, a := range s.rows {
		if a.ItemID == itemID && a.Active() {
			return i
		}
	}
	return -1
}

func (s *Store) place(id int64) *domain.Place {
	if p, ok := s.places[id]; ok {
		return &p
	}
	return nil
}

func (s *Store) name(userID string) string { return s.users[userID] }

func (s *Store) hydrate(a domain.Assignment) domain.Assignment {
	it := s.items[a.ItemID]
	a.ItemCode, a.ItemDescription = it.Code, it.Description
	if p := s.place(a.AssigneeLocationID); p != nil {
		a.AssigneeName, a.AssigneeKind = p.Name, p.Kind
	}
	if a.OriginLocationID != nil {
		if p := s.place(*a.OriginLocationID); p != nil {
			a.OriginName = &p.Name
		}
	}
	if a.ReturnedToID != nil {
		if p := s.place(*a.ReturnedToID); p != nil {
			a.ReturnedToName = &p.Name
		}
	}
	a.AssignedByName = s.name(a.AssignedBy)
	if a.ReturnedBy != nil {
		n := s.name(*a.ReturnedBy)
		a.ReturnedByName = &n
	}
	return a
}

func (s *Store) lock(itemID string) (*Item, error) {
	it, ok := s.items[itemID]
	if !ok || it.Deleted {
		return nil, domain.ErrItemNotFound
	}
	return it, nil
}

func (s *Store) state(it *Item) domain.ItemState {
	st := domain.ItemState{Kind: it.Kind, Status: it.Status, LocationID: it.LocationID}
	if i := s.activeIndex(it.ID); i >= 0 {
		a := s.rows[i]
		st.Active = &a
	}
	return st
}

// Assign implements app.AssignmentRepository.
func (s *Store) Assign(_ context.Context, itemID, userID string, assigneeID int64, decide func(domain.ItemState) (domain.AssignOutcome, error)) (domain.Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.lock(itemID)
	if err != nil {
		return domain.Assignment{}, err
	}
	st := s.state(it)
	st.Assignee = s.place(assigneeID)
	out, err := decide(st)
	if err != nil {
		return domain.Assignment{}, err
	}
	s.nextID++
	a := out.Assignment
	a.ID, a.ItemID, a.AssignedBy, a.CreatedAt = s.nextID, itemID, userID, a.AssignedAt
	s.rows = append(s.rows, a)
	it.LocationID = out.LocationID
	return s.hydrate(a), nil
}

// Return implements app.AssignmentRepository.
func (s *Store) Return(_ context.Context, itemID, userID string, returnLocationID *int64, decide func(domain.ItemState) (domain.ReturnOutcome, error)) (domain.Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.lock(itemID)
	if err != nil {
		return domain.Assignment{}, err
	}
	st := s.state(it)
	if returnLocationID != nil {
		st.ReturnTo = s.place(*returnLocationID)
	}
	out, err := decide(st)
	if err != nil {
		return domain.Assignment{}, err
	}
	i := s.activeIndex(itemID)
	at := out.ReturnedAt
	s.rows[i].ReturnedAt, s.rows[i].ReturnedToID = &at, out.ReturnedTo
	s.rows[i].ReturnedBy, s.rows[i].ReturnNote = &userID, out.ReturnNote
	it.LocationID = out.ReturnedTo
	return s.hydrate(s.rows[i]), nil
}

func page[T any](all []T, p domain.Page) []T {
	if p.Offset >= len(all) {
		return []T{}
	}
	end := p.Offset + p.Limit
	if end > len(all) {
		end = len(all)
	}
	return all[p.Offset:end]
}

// History implements app.AssignmentRepository.
func (s *Store) History(_ context.Context, itemID string, p domain.Page) ([]domain.Assignment, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.lock(itemID); err != nil {
		return nil, 0, err
	}
	var all []domain.Assignment
	for _, a := range s.rows {
		if a.ItemID == itemID {
			all = append(all, s.hydrate(a))
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].AssignedAt.Equal(all[j].AssignedAt) {
			return all[i].AssignedAt.After(all[j].AssignedAt)
		}
		return all[i].ID > all[j].ID
	})
	return page(all, p), len(all), nil
}

// Active implements app.AssignmentRepository.
func (s *Store) Active(_ context.Context, f domain.ActiveFilter, p domain.Page) ([]domain.Assignment, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.ToLower(f.Q)
	var all []domain.Assignment
	for _, a := range s.rows {
		if !a.Active() {
			continue
		}
		a = s.hydrate(a)
		if f.AssigneeLocationID != nil && a.AssigneeLocationID != *f.AssigneeLocationID {
			continue
		}
		if f.OverdueOnly && !a.Overdue(f.Now) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(a.ItemCode), q) && !strings.Contains(strings.ToLower(a.ItemDescription), q) {
			continue
		}
		all = append(all, a)
	}
	sort.SliceStable(all, func(i, j int) bool {
		oi, oj := all[i].Overdue(f.Now), all[j].Overdue(f.Now)
		if oi != oj {
			return oi
		}
		if !all[i].AssignedAt.Equal(all[j].AssignedAt) {
			return all[i].AssignedAt.Before(all[j].AssignedAt)
		}
		return all[i].ID < all[j].ID
	})
	return page(all, p), len(all), nil
}
