// Package memory provides in-memory catalog repositories. They back unit tests
// of the use cases and the HTTP adapter; they are not used in production.
package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/catalog/domain"
)

// Store implements app.CategoryRepository, app.LocationRepository and
// app.ItemRepository.
type Store struct {
	mu      sync.Mutex
	catSeq  int64
	locSeq  int64
	itemSeq int
	cats    map[int64]domain.Category
	locs    map[int64]domain.Location
	items   map[string]storedItem
	seqs    map[string]int
	clock   func() time.Time
}

type storedItem struct {
	domain.Item
	deleted bool
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		cats: map[int64]domain.Category{}, locs: map[int64]domain.Location{},
		items: map[string]storedItem{}, seqs: map[string]int{}, clock: time.Now,
	}
}

// Categories returns the category repository view.
func (s *Store) Categories() *Categories { return &Categories{s} }

// Locations returns the location repository view.
func (s *Store) Locations() *Locations { return &Locations{s} }

// Items returns the item repository view.
func (s *Store) Items() *Items { return &Items{s} }

// SeedSequence sets the last used number for a prefix (test helper).
func (s *Store) SeedSequence(prefix string, last int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seqs[prefix] = last
}

// ---- categories ----

// Categories is the category repository.
type Categories struct{ s *Store }

// Create implements app.CategoryRepository.
func (c *Categories) Create(_ context.Context, cat *domain.Category) error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if err := c.s.checkCategoryUnique(*cat); err != nil {
		return err
	}
	c.s.catSeq++
	cat.ID = c.s.catSeq
	c.s.cats[cat.ID] = *cat
	return nil
}

func (s *Store) checkCategoryUnique(cat domain.Category) error {
	for id, e := range s.cats {
		if id == cat.ID {
			continue
		}
		if strings.EqualFold(e.Name, cat.Name) {
			return domain.ErrCategoryNameTaken
		}
		if e.CodePrefix == cat.CodePrefix {
			return domain.ErrCategoryPrefixTaken
		}
	}
	return nil
}

// Get implements app.CategoryRepository.
func (c *Categories) Get(_ context.Context, id int64) (domain.Category, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	cat, ok := c.s.cats[id]
	if !ok {
		return domain.Category{}, domain.ErrCategoryNotFound
	}
	return cat, nil
}

// List implements app.CategoryRepository.
func (c *Categories) List(context.Context) ([]domain.Category, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	out := make([]domain.Category, 0, len(c.s.cats))
	for _, cat := range c.s.cats {
		out = append(out, cat)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// Update implements app.CategoryRepository.
func (c *Categories) Update(_ context.Context, cat *domain.Category) error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if _, ok := c.s.cats[cat.ID]; !ok {
		return domain.ErrCategoryNotFound
	}
	if err := c.s.checkCategoryUnique(*cat); err != nil {
		return err
	}
	c.s.cats[cat.ID] = *cat
	return nil
}

// Delete implements app.CategoryRepository.
func (c *Categories) Delete(_ context.Context, id int64) error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if _, ok := c.s.cats[id]; !ok {
		return domain.ErrCategoryNotFound
	}
	for _, it := range c.s.items {
		if it.CategoryID == id { // soft-deleted items still reference the category
			return domain.ErrCategoryInUse
		}
	}
	delete(c.s.cats, id)
	return nil
}

// ---- locations ----

// Locations is the location repository.
type Locations struct{ s *Store }

// Create implements app.LocationRepository.
func (l *Locations) Create(_ context.Context, loc *domain.Location) error {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if loc.ParentID != nil {
		if _, ok := l.s.locs[*loc.ParentID]; !ok {
			return domain.ErrLocationNotFound
		}
	}
	l.s.locSeq++
	loc.ID = l.s.locSeq
	l.s.locs[loc.ID] = *loc
	return nil
}

// Get implements app.LocationRepository.
func (l *Locations) Get(_ context.Context, id int64) (domain.Location, error) {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	loc, ok := l.s.locs[id]
	if !ok {
		return domain.Location{}, domain.ErrLocationNotFound
	}
	return loc, nil
}

// List implements app.LocationRepository.
func (l *Locations) List(context.Context) ([]domain.Location, error) {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	out := make([]domain.Location, 0, len(l.s.locs))
	for _, loc := range l.s.locs {
		out = append(out, loc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Update implements app.LocationRepository.
func (l *Locations) Update(_ context.Context, loc *domain.Location) error {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if _, ok := l.s.locs[loc.ID]; !ok {
		return domain.ErrLocationNotFound
	}
	if loc.ParentID != nil {
		if _, ok := l.s.locs[*loc.ParentID]; !ok {
			return domain.ErrLocationNotFound
		}
	}
	l.s.locs[loc.ID] = *loc
	return nil
}

// Delete implements app.LocationRepository.
func (l *Locations) Delete(_ context.Context, id int64) error {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if _, ok := l.s.locs[id]; !ok {
		return domain.ErrLocationNotFound
	}
	for _, o := range l.s.locs {
		if o.ParentID != nil && *o.ParentID == id {
			return domain.ErrLocationInUse
		}
	}
	for _, it := range l.s.items {
		if it.LocationID != nil && *it.LocationID == id {
			return domain.ErrLocationInUse
		}
	}
	delete(l.s.locs, id)
	return nil
}

// ---- items ----

// Items is the item repository.
type Items struct{ s *Store }

// Create implements app.ItemRepository.
func (r *Items) Create(_ context.Context, it *domain.Item) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.cats[it.CategoryID]; !ok {
		return domain.ErrCategoryNotFound
	}
	if it.LocationID != nil {
		if _, ok := r.s.locs[*it.LocationID]; !ok {
			return domain.ErrLocationNotFound
		}
	}
	for _, e := range r.s.items {
		if e.Code == it.Code { // deleted items keep their code reserved
			return domain.ErrCodeTaken
		}
	}
	r.s.itemSeq++
	it.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", r.s.itemSeq)
	now := r.s.clock()
	it.CreatedAt, it.UpdatedAt = now, now
	r.s.items[it.ID] = storedItem{Item: *it}
	return nil
}

// Get implements app.ItemRepository.
func (r *Items) Get(_ context.Context, id string) (domain.Item, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	e, ok := r.s.items[id]
	if !ok || e.deleted {
		return domain.Item{}, domain.ErrItemNotFound
	}
	return e.Item, nil
}

// GetByCode implements app.ItemRepository.
func (r *Items) GetByCode(_ context.Context, code string) (domain.Item, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, e := range r.s.items {
		if e.Code == code && !e.deleted {
			return e.Item, nil
		}
	}
	return domain.Item{}, domain.ErrItemNotFound
}

// Update implements app.ItemRepository.
func (r *Items) Update(_ context.Context, it *domain.Item) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	e, ok := r.s.items[it.ID]
	if !ok || e.deleted {
		return domain.ErrItemNotFound
	}
	if _, ok := r.s.cats[it.CategoryID]; !ok {
		return domain.ErrCategoryNotFound
	}
	if it.LocationID != nil {
		if _, ok := r.s.locs[*it.LocationID]; !ok {
			return domain.ErrLocationNotFound
		}
	}
	// Like the SQL repository: code, kind and stock_current are not written.
	it.Code, it.Kind, it.StockCurrent = e.Code, e.Kind, e.StockCurrent
	it.CreatedAt, it.UpdatedAt = e.CreatedAt, r.s.clock()
	r.s.items[it.ID] = storedItem{Item: *it}
	return nil
}

// SoftDelete implements app.ItemRepository.
func (r *Items) SoftDelete(_ context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	e, ok := r.s.items[id]
	if !ok || e.deleted {
		return domain.ErrItemNotFound
	}
	e.deleted = true
	r.s.items[id] = e
	return nil
}

// List implements app.ItemRepository.
func (r *Items) List(_ context.Context, f domain.ItemFilter) ([]domain.Item, int, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var all []domain.Item
	for _, e := range r.s.items {
		if !e.deleted && f.Matches(e.Item) {
			all = append(all, e.Item)
		}
	}
	less := func(a, b domain.Item) bool {
		switch f.Sort {
		case domain.SortDescription:
			if la, lb := strings.ToLower(a.Description), strings.ToLower(b.Description); la != lb {
				return la < lb
			}
		case domain.SortUpdatedAt:
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.Before(b.UpdatedAt)
			}
		}
		return a.Code < b.Code
	}
	sort.Slice(all, func(i, j int) bool {
		if f.Desc {
			return less(all[j], all[i])
		}
		return less(all[i], all[j])
	})
	total := len(all)
	if f.Offset >= total {
		return []domain.Item{}, total, nil
	}
	end := f.Offset + f.Limit
	if f.Limit <= 0 || end > total {
		end = total
	}
	return all[f.Offset:end], total, nil
}

// NextSequence implements app.ItemRepository.
func (r *Items) NextSequence(_ context.Context, prefix string) (int, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.seqs[prefix]++
	return r.s.seqs[prefix], nil
}
