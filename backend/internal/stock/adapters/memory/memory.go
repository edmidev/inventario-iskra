// Package memory is an in-memory stock repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/stock/domain"
)

// Item is a seeded item row.
type Item struct {
	ID           string
	Code         string
	Description  string
	Kind         domain.Kind
	Unit         string
	StockCurrent int
	StockMin     int
	LocationID   *int64
	Deleted      bool
}

// Store holds items, known locations and the ledger. Safe for concurrent use.
type Store struct {
	mu        sync.Mutex
	items     map[string]*Item
	locations map[int64]string
	moves     []domain.Movement
	users     map[string]string
	nextID    int64
	now       func() time.Time
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{items: map[string]*Item{}, locations: map[int64]string{}, users: map[string]string{},
		now: time.Now}
}

// SeedItem adds an item.
func (s *Store) SeedItem(it Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := it
	s.items[it.ID] = &cp
}

// SeedLocation registers a location id and name.
func (s *Store) SeedLocation(id int64, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locations[id] = name
}

// SeedUser registers a user name for history output.
func (s *Store) SeedUser(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[id] = name
}

// Item returns a copy of a seeded item.
func (s *Store) Item(id string) Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.items[id]
}

// Movements returns a copy of the ledger in insertion order.
func (s *Store) Movements() []domain.Movement {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Movement(nil), s.moves...)
}

// Record implements app.MovementRepository.
func (s *Store) Record(_ context.Context, itemID, userID string, decide func(domain.ItemState) (domain.Outcome, error)) (domain.Movement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[itemID]
	if !ok || it.Deleted {
		return domain.Movement{}, domain.ErrItemNotFound
	}
	out, err := decide(domain.ItemState{Kind: it.Kind, StockCurrent: it.StockCurrent, LocationID: it.LocationID})
	if err != nil {
		return domain.Movement{}, err
	}
	if to := out.Movement.ToLocationID; to != nil {
		if _, ok := s.locations[*to]; !ok {
			return domain.Movement{}, domain.ErrLocationNotFound
		}
	}
	it.StockCurrent, it.LocationID = out.StockCurrent, out.LocationID
	s.nextID++
	m := out.Movement
	m.ID, m.ItemID, m.CreatedAt = s.nextID, itemID, s.now()
	uid := userID
	m.UserID = &uid
	if name, ok := s.users[userID]; ok {
		m.UserName = &name
	}
	s.moves = append(s.moves, m)
	return m, nil
}

// List implements app.MovementRepository.
func (s *Store) List(_ context.Context, itemID string, typ domain.MovementType, p domain.Page) ([]domain.Movement, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.items[itemID]; !ok || it.Deleted {
		return nil, 0, domain.ErrItemNotFound
	}
	var all []domain.Movement
	for i := len(s.moves) - 1; i >= 0; i-- { // newest first
		m := s.moves[i]
		if m.ItemID == itemID && (typ == "" || m.Type == typ) {
			all = append(all, m)
		}
	}
	return page(all, p), len(all), nil
}

// Alerts implements app.MovementRepository.
func (s *Store) Alerts(_ context.Context, p domain.Page) ([]domain.Alert, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []domain.Alert
	for _, it := range s.items {
		if it.Deleted || it.Kind != domain.KindConsumable || it.StockMin <= 0 || it.StockCurrent > it.StockMin {
			continue
		}
		a := domain.Alert{ItemID: it.ID, Code: it.Code, Description: it.Description, Unit: it.Unit,
			StockCurrent: it.StockCurrent, StockMin: it.StockMin, LocationID: it.LocationID}
		if it.LocationID != nil {
			if n, ok := s.locations[*it.LocationID]; ok {
				a.LocationName = &n
			}
		}
		all = append(all, a)
	}
	sort.Slice(all, func(i, j int) bool {
		if si, sj := all[i].Shortage(), all[j].Shortage(); si != sj {
			return si < sj
		}
		return all[i].Code < all[j].Code
	})
	return page(all, p), len(all), nil
}

func page[T any](all []T, p domain.Page) []T {
	if p.Offset >= len(all) {
		return []T{}
	}
	end := min(p.Offset+p.Limit, len(all))
	return all[p.Offset:end]
}
