// Package memory provides a thread-safe template repository for tests.
package memory

import (
	"context"
	"github.com/edgar/inventario/backend/internal/projects/domain"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu   sync.Mutex
	next int64
	rows map[int64]domain.Project
}

func NewStore() *Store { return &Store{rows: map[int64]domain.Project{}} }
func (s *Store) nameTaken(name string, except int64) bool {
	for id, p := range s.rows {
		if id != except && strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}
func (s *Store) Create(_ context.Context, p *domain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nameTaken(p.Name, 0) {
		return domain.ErrNameTaken
	}
	s.next++
	p.ID = s.next
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = p.CreatedAt
	s.rows[p.ID] = p.Clone()
	return nil
}
func (s *Store) Get(_ context.Context, id int64) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.rows[id]
	if !ok {
		return p, domain.ErrNotFound
	}
	return p.Clone(), nil
}
func (s *Store) Update(_ context.Context, p *domain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.rows[p.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if s.nameTaken(p.Name, p.ID) {
		return domain.ErrNameTaken
	}
	p.CreatedAt = old.CreatedAt
	p.UpdatedAt = time.Now().UTC()
	s.rows[p.ID] = p.Clone()
	return nil
}
func (s *Store) Delete(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}
func (s *Store) List(_ context.Context, f domain.Filter) ([]domain.Project, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := []domain.Project{}
	for _, p := range s.rows {
		if strings.Contains(strings.ToLower(p.Name+" "+p.Description), strings.ToLower(f.Q)) {
			all = append(all, p.Clone())
		}
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := strings.ToLower(all[i].Name), strings.ToLower(all[j].Name)
		if a == b {
			return all[i].ID < all[j].ID
		}
		return a < b
	})
	n := len(all)
	if f.Offset >= n {
		return []domain.Project{}, n, nil
	}
	return all[f.Offset:min(f.Offset+f.Limit, n)], n, nil
}
