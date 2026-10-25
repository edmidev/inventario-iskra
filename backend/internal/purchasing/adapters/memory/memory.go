// Package memory is an in-memory purchase order repository for tests.
package memory

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/purchasing/domain"
)

// Store keeps orders in a map. Mutate holds the store lock for the whole
// callback, which gives the same serialization the postgres row lock does.
type Store struct {
	mu          sync.Mutex
	nextOrder   int64
	nextLine    int64
	nextReceipt int64
	rows        map[int64]domain.Order
	suppliers   map[int64]string
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{rows: map[int64]domain.Order{}, suppliers: map[int64]string{}}
}

// SetSupplierName tells the store the name to show (and search) for a supplier,
// standing in for the join the postgres adapter does.
func (s *Store) SetSupplierName(id int64, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suppliers[id] = name
}

func clone(o domain.Order) domain.Order {
	c := o
	c.Lines = make([]domain.Line, len(o.Lines))
	for i, l := range o.Lines {
		if l.NewItem != nil {
			n := *l.NewItem
			l.NewItem = &n
		}
		c.Lines[i] = l
	}
	c.Receipts = make([]domain.Receipt, len(o.Receipts))
	for i, r := range o.Receipts {
		r.Lines = append([]domain.ReceiptLine(nil), r.Lines...)
		for j := range r.Lines {
			r.Lines[j].Items = append([]domain.ReceiptItem(nil), r.Lines[j].Items...)
		}
		c.Receipts[i] = r
	}
	return c
}

// store assigns ids to new lines and receipts and saves o.
func (s *Store) store(o *domain.Order) {
	for i := range o.Lines {
		if o.Lines[i].ID == 0 {
			s.nextLine++
			o.Lines[i].ID = s.nextLine
		}
	}
	for i := range o.Receipts {
		if o.Receipts[i].ID == 0 {
			s.nextReceipt++
			o.Receipts[i].ID = s.nextReceipt
		}
	}
	o.LinesDirty = false
	s.rows[o.ID] = clone(*o)
}

func (s *Store) view(o domain.Order) domain.Order {
	c := clone(o)
	c.SupplierName = s.suppliers[c.SupplierID]
	return c
}

// Create implements app.Repository.
func (s *Store) Create(_ context.Context, o *domain.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextOrder++
	o.ID = s.nextOrder
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	o.UpdatedAt = o.CreatedAt
	s.store(o)
	return nil
}

// Get implements app.Repository.
func (s *Store) Get(_ context.Context, id int64) (domain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.rows[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	return s.view(o), nil
}

// Mutate implements app.Repository. The callback works on a copy that is saved
// only when it succeeds.
func (s *Store) Mutate(ctx context.Context, id int64, fn func(context.Context, *domain.Order) error) (domain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.rows[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	work := s.view(cur)
	if err := fn(ctx, &work); err != nil {
		if err == domain.ErrNoChange {
			return s.view(cur), nil
		}
		return domain.Order{}, err
	}
	s.store(&work)
	return s.view(s.rows[id]), nil
}

// List implements app.Repository: newest first.
func (s *Store) List(_ context.Context, f domain.Filter) ([]domain.Order, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.TrimPrefix(strings.TrimSpace(f.Q), "#")
	var all []domain.Order
	for _, o := range s.rows {
		if f.Status != "" && o.Status != f.Status {
			continue
		}
		if f.SupplierID != 0 && o.SupplierID != f.SupplierID {
			continue
		}
		if q != "" && strconv.FormatInt(o.ID, 10) != q &&
			!strings.Contains(strings.ToLower(s.suppliers[o.SupplierID]), strings.ToLower(q)) {
			continue
		}
		all = append(all, o)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := len(all)
	lo := min(f.Offset, total)
	hi := min(lo+f.Limit, total)
	out := make([]domain.Order, 0, hi-lo)
	for _, o := range all[lo:hi] {
		out = append(out, s.view(o))
	}
	return out, total, nil
}
