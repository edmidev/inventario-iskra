// Package memory is an in-memory stock repository for tests.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/stock/domain"
)

// defaultLocationID and defaultLocationName stand in for the "Sin ubicación"
// location the SQL repository creates on demand.
const (
	defaultLocationID   int64 = 1_000_000
	defaultLocationName       = "Sin ubicación"
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
	// LocationID is an asset's location. For a consumable seeded with stock
	// and no Stocks it is shorthand for one stock row at that location.
	LocationID *int64
	// Stocks is a consumable's per-location quantity by location id.
	Stocks  map[int64]int
	Deleted bool
	OnLoan  bool // active custody assignment
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
	cp.Stocks = map[int64]int{}
	for k, v := range it.Stocks {
		cp.Stocks[k] = v
	}
	if cp.Kind == domain.KindConsumable {
		if len(cp.Stocks) == 0 && cp.StockCurrent > 0 && cp.LocationID != nil {
			cp.Stocks[*cp.LocationID] = cp.StockCurrent
		}
		cp.LocationID = nil
		total := 0
		for _, q := range cp.Stocks {
			total += q
		}
		if len(cp.Stocks) > 0 {
			cp.StockCurrent = total
		}
	}
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
	cp := *s.items[id]
	cp.Stocks = map[int64]int{}
	for k, v := range s.items[id].Stocks {
		cp.Stocks[k] = v
	}
	return cp
}

// Movements returns a copy of the ledger in insertion order.
func (s *Store) Movements() []domain.Movement {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Movement(nil), s.moves...)
}

// stocksLocked returns an item's stock rows ordered by location name, then id.
func (s *Store) stocksLocked(it *Item) []domain.LocationStock {
	out := make([]domain.LocationStock, 0, len(it.Stocks))
	for id, q := range it.Stocks {
		out = append(out, domain.LocationStock{LocationID: id, LocationName: s.locations[id], Quantity: q})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].LocationName), strings.ToLower(out[j].LocationName)
		if a != b {
			return a < b
		}
		return out[i].LocationID < out[j].LocationID
	})
	return out
}

// Record implements app.MovementRepository.
func (s *Store) Record(_ context.Context, itemID, userID string, decide func(domain.ItemState) (domain.Outcome, error)) (domain.Movement, []domain.LocationStock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[itemID]
	if !ok || it.Deleted {
		return domain.Movement{}, nil, domain.ErrItemNotFound
	}
	locs := make(map[int64]string, len(s.locations))
	for id, n := range s.locations {
		locs[id] = n
	}
	st := domain.ItemState{Kind: it.Kind, StockCurrent: it.StockCurrent, LocationID: it.LocationID, OnLoan: it.OnLoan,
		Stocks: s.stocksLocked(it), Locations: locs}
	for i := len(s.moves) - 1; i >= 0 && st.LastLocationID == nil; i-- {
		if m := s.moves[i]; m.ItemID == itemID {
			if m.LocationID != nil {
				st.LastLocationID = m.LocationID
			} else if m.ToLocationID != nil {
				st.LastLocationID = m.ToLocationID
			}
		}
	}
	out, err := decide(st)
	if err != nil {
		return domain.Movement{}, nil, err
	}
	m := out.Movement
	if out.DefaultLocation {
		if _, ok := s.locations[defaultLocationID]; !ok {
			s.locations[defaultLocationID] = defaultLocationName
		}
		id := defaultLocationID
		m.LocationID = &id
	}
	for _, c := range out.Changes {
		id := c.LocationID
		if out.DefaultLocation && id == 0 {
			id = defaultLocationID
		}
		if c.Quantity == 0 {
			delete(it.Stocks, id)
		} else {
			it.Stocks[id] = c.Quantity
		}
	}
	it.StockCurrent = out.StockCurrent
	if it.Kind == domain.KindAsset {
		it.LocationID = out.LocationID
	}
	s.nextID++
	m.ID, m.ItemID, m.CreatedAt = s.nextID, itemID, s.now()
	uid := userID
	m.UserID = &uid
	if name, ok := s.users[userID]; ok {
		m.UserName = &name
	}
	s.moves = append(s.moves, m)
	if it.Kind != domain.KindConsumable {
		return m, nil, nil
	}
	return m, s.stocksLocked(it), nil
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
			StockCurrent: it.StockCurrent, StockMin: it.StockMin, Stocks: s.stocksLocked(it)}
		if len(a.Stocks) == 1 {
			id, n := a.Stocks[0].LocationID, a.Stocks[0].LocationName
			a.LocationID, a.LocationName = &id, &n
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
