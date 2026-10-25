// Package memory is an in-memory supplier repository for tests.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/suppliers/domain"
)

// Store keeps suppliers in a map; deleted ones are removed.
type Store struct {
	mu     sync.Mutex
	nextID int64
	rows   map[int64]domain.Supplier
}

// NewStore returns an empty Store.
func NewStore() *Store { return &Store{rows: map[int64]domain.Supplier{}} }

func (s *Store) nameTaken(name string, except int64) bool {
	for id, r := range s.rows {
		if id != except && strings.EqualFold(r.Name, name) {
			return true
		}
	}
	return false
}

// Create implements app.Repository.
func (s *Store) Create(_ context.Context, sup *domain.Supplier) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nameTaken(sup.Name, 0) {
		return domain.ErrNameTaken
	}
	s.nextID++
	sup.ID = s.nextID
	sup.CreatedAt = time.Now().UTC()
	sup.UpdatedAt = sup.CreatedAt
	s.rows[sup.ID] = *sup
	return nil
}

// Get implements app.Repository.
func (s *Store) Get(_ context.Context, id int64) (domain.Supplier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok {
		return domain.Supplier{}, domain.ErrNotFound
	}
	return r, nil
}

// Update implements app.Repository.
func (s *Store) Update(_ context.Context, sup *domain.Supplier) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.rows[sup.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if s.nameTaken(sup.Name, sup.ID) {
		return domain.ErrNameTaken
	}
	sup.CreatedAt = cur.CreatedAt
	sup.UpdatedAt = time.Now().UTC()
	s.rows[sup.ID] = *sup
	return nil
}

// Delete implements app.Repository.
func (s *Store) Delete(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}

// List implements app.Repository.
func (s *Store) List(_ context.Context, f domain.Filter) ([]domain.Supplier, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.ToLower(f.Q)
	var all []domain.Supplier
	for _, r := range s.rows {
		if f.Active != nil && r.Active != *f.Active {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.Name+"\x00"+r.ContactName+"\x00"+r.Email), q) {
			continue
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := strings.ToLower(all[i].Name), strings.ToLower(all[j].Name)
		if a != b {
			return a < b
		}
		return all[i].ID < all[j].ID
	})
	total := len(all)
	if f.Offset >= total {
		return []domain.Supplier{}, total, nil
	}
	end := min(f.Offset+f.Limit, total)
	return all[f.Offset:end], total, nil
}
