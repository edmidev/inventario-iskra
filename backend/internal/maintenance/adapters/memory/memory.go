// Package memory is an in-memory maintenance repository for tests. It models
// the same transactional behavior as the PostgreSQL adapter: one lock covers
// the item and its records.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/maintenance/domain"
)

// Item is a seeded item row.
type Item struct {
	ID                string
	Code              string
	Description       string
	CategoryID        int64
	CategoryName      string
	Kind              domain.ItemKind
	Status            domain.ItemStatus
	IntervalDays      *int
	LastMaintenanceAt *time.Time
	LocationID        *int64
	LocationName      *string
	CreatedAt         time.Time
	Deleted           bool
}

// Store holds items and records. Safe for concurrent use.
type Store struct {
	mu      sync.Mutex
	items   map[string]*Item
	users   map[string]string
	records []domain.Record
	nextID  int64
	now     func() time.Time
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{items: map[string]*Item{}, users: map[string]string{}, now: time.Now}
}

// SeedItem adds an item; a zero CreatedAt defaults to the Unix epoch plus one day.
func (s *Store) SeedItem(it Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it.Status == "" {
		it.Status = domain.ItemFunctional
	}
	cp := it
	s.items[it.ID] = &cp
}

// SeedUser registers a user name.
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

// Records returns a copy of every record in insertion order.
func (s *Store) Records() []domain.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Record(nil), s.records...)
}

func (s *Store) state(it *Item) domain.ItemState {
	st := domain.ItemState{Kind: it.Kind, Status: it.Status, IntervalDays: it.IntervalDays, LastMaintenanceAt: it.LastMaintenanceAt}
	for i := range s.records {
		if r := s.records[i]; r.ItemID == it.ID && r.Kind == domain.KindCorrective && r.Status == domain.StatusOpen {
			cp := r
			st.Open = &cp
		}
	}
	return st
}

// view fills the joined display fields of a record.
func (s *Store) view(r domain.Record) domain.Record {
	it := s.items[r.ItemID]
	r.ItemCode, r.ItemDescription, r.ItemStatus = it.Code, it.Description, it.Status
	r.ReportedByName = s.users[r.ReportedBy]
	if r.ClosedBy != nil {
		if n, ok := s.users[*r.ClosedBy]; ok {
			r.ClosedByName = &n
		}
	}
	return r
}

func (s *Store) live(itemID string) (*Item, error) {
	it, ok := s.items[itemID]
	if !ok || it.Deleted {
		return nil, domain.ErrItemNotFound
	}
	return it, nil
}

func (s *Store) insert(it *Item, userID string, r domain.Record) domain.Record {
	s.nextID++
	t := s.now()
	r.ID, r.ItemID, r.ReportedBy, r.CreatedAt, r.UpdatedAt = s.nextID, it.ID, userID, t, t
	if r.Status == domain.StatusClosed {
		uid := userID
		r.ClosedBy = &uid
	}
	s.records = append(s.records, r)
	return s.view(r)
}

// Report implements app.Repository.
func (s *Store) Report(_ context.Context, itemID, userID string, decide func(domain.ItemState) (domain.ReportOutcome, error)) (domain.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.live(itemID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(s.state(it))
	if err != nil {
		return domain.Record{}, err
	}
	it.Status = out.ItemStatus
	return s.insert(it, userID, out.Record), nil
}

// RegisterPreventive implements app.Repository.
func (s *Store) RegisterPreventive(_ context.Context, itemID, userID string, decide func(domain.ItemState) (domain.PreventiveOutcome, error)) (domain.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.live(itemID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(s.state(it))
	if err != nil {
		return domain.Record{}, err
	}
	t := out.LastMaintenanceAt
	it.LastMaintenanceAt = &t
	return s.insert(it, userID, out.Record), nil
}

func (s *Store) find(recordID int64) (int, *Item, error) {
	for i := range s.records {
		if s.records[i].ID == recordID {
			it, err := s.live(s.records[i].ItemID)
			if err != nil {
				return 0, nil, domain.ErrRecordNotFound
			}
			return i, it, nil
		}
	}
	return 0, nil, domain.ErrRecordNotFound
}

// Update implements app.Repository.
func (s *Store) Update(_ context.Context, recordID int64, decide func(domain.ItemState, domain.Record) (domain.UpdateOutcome, error)) (domain.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, it, err := s.find(recordID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(s.state(it), s.records[i])
	if err != nil {
		return domain.Record{}, err
	}
	if out.ItemStatus != "" {
		it.Status = out.ItemStatus
	}
	out.Record.UpdatedAt = s.now()
	s.records[i] = out.Record
	return s.view(out.Record), nil
}

// Close implements app.Repository.
func (s *Store) Close(_ context.Context, recordID int64, userID string, decide func(domain.ItemState, domain.Record) (domain.CloseOutcome, error)) (domain.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, it, err := s.find(recordID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(s.state(it), s.records[i])
	if err != nil {
		return domain.Record{}, err
	}
	it.Status = out.ItemStatus
	if out.LastMaintenanceAt != nil {
		t := *out.LastMaintenanceAt
		it.LastMaintenanceAt = &t
	}
	uid := userID
	out.Record.ClosedBy = &uid
	out.Record.UpdatedAt = s.now()
	s.records[i] = out.Record
	return s.view(out.Record), nil
}

func newestFirst(rs []domain.Record) {
	sort.SliceStable(rs, func(a, b int) bool {
		if !rs[a].ReportedAt.Equal(rs[b].ReportedAt) {
			return rs[a].ReportedAt.After(rs[b].ReportedAt)
		}
		return rs[a].ID > rs[b].ID
	})
}

// History implements app.Repository.
func (s *Store) History(_ context.Context, itemID string, p domain.Page) ([]domain.Record, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.live(itemID); err != nil {
		return nil, 0, err
	}
	var all []domain.Record
	for _, r := range s.records {
		if r.ItemID == itemID {
			all = append(all, s.view(r))
		}
	}
	newestFirst(all)
	return page(all, p), len(all), nil
}

// List implements app.Repository.
func (s *Store) List(_ context.Context, f domain.ListFilter, p domain.Page) ([]domain.Record, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []domain.Record
	for _, r := range s.records {
		it := s.items[r.ItemID]
		if it.Deleted || (f.Status != "" && r.Status != f.Status) || (f.Kind != "" && r.Kind != f.Kind) ||
			(f.ItemID != "" && r.ItemID != f.ItemID) {
			continue
		}
		if f.Destination != "" && (r.Destination == nil || !strings.EqualFold(*r.Destination, f.Destination)) {
			continue
		}
		if q := strings.ToLower(f.Q); q != "" &&
			!strings.Contains(strings.ToLower(it.Code), q) && !strings.Contains(strings.ToLower(it.Description), q) &&
			!strings.Contains(strings.ToLower(r.Description), q) {
			continue
		}
		all = append(all, s.view(r))
	}
	newestFirst(all)
	sort.SliceStable(all, func(a, b int) bool { return all[a].Status == domain.StatusOpen && all[b].Status != domain.StatusOpen })
	return page(all, p), len(all), nil
}

// Due implements app.Repository.
func (s *Store) Due(_ context.Context, f domain.DueFilter, p domain.Page) ([]domain.DueItem, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := domain.AddDays(f.Now, f.WithinDays)
	var all []domain.DueItem
	for _, it := range s.items {
		if it.Deleted || it.Kind != domain.ItemAsset || it.IntervalDays == nil {
			continue
		}
		if f.CategoryID != nil && it.CategoryID != *f.CategoryID {
			continue
		}
		if q := strings.ToLower(f.Q); q != "" && !strings.Contains(strings.ToLower(it.Code), q) &&
			!strings.Contains(strings.ToLower(it.Description), q) {
			continue
		}
		base := it.CreatedAt
		if it.LastMaintenanceAt != nil {
			base = *it.LastMaintenanceAt
		}
		due := domain.AddDays(base, *it.IntervalDays)
		if due.After(cutoff) {
			continue
		}
		all = append(all, domain.DueItem{ItemID: it.ID, Code: it.Code, Description: it.Description, CategoryID: it.CategoryID,
			CategoryName: it.CategoryName, Status: it.Status, LocationID: it.LocationID, LocationName: it.LocationName,
			LastMaintenanceAt: it.LastMaintenanceAt, IntervalDays: *it.IntervalDays, DueAt: due})
	}
	sort.Slice(all, func(a, b int) bool {
		if !all[a].DueAt.Equal(all[b].DueAt) {
			return all[a].DueAt.Before(all[b].DueAt)
		}
		return all[a].Code < all[b].Code
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
