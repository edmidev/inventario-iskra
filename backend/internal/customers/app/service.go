// Package app holds the customer use cases and the port they depend on.
package app

import (
	"context"

	"github.com/edgar/inventario/backend/internal/customers/domain"
)

// Repository persists customers. Soft-deleted customers behave as missing.
type Repository interface {
	// Create stores s and fills its ID and timestamps. A name already used by
	// a live customer (case-insensitive) yields ErrNameTaken.
	Create(ctx context.Context, s *domain.Customer) error
	// Get returns a live customer or ErrNotFound.
	Get(ctx context.Context, id int64) (domain.Customer, error)
	// Update stores the editable fields of s and refreshes UpdatedAt.
	Update(ctx context.Context, s *domain.Customer) error
	// Delete soft-deletes a live customer or yields ErrNotFound.
	Delete(ctx context.Context, id int64) error
	// List returns a page of live customers ordered by name and the total count.
	List(ctx context.Context, f domain.Filter) ([]domain.Customer, int, error)
}

// Service implements the customer use cases.
type Service struct {
	repo Repository
}

// NewService wires the repository.
func NewService(r Repository) *Service { return &Service{repo: r} }

// Create validates and stores a customer.
func (s *Service) Create(ctx context.Context, in domain.Input) (domain.Customer, error) {
	n, err := in.Normalize()
	if err != nil {
		return domain.Customer{}, err
	}
	sup := domain.Customer{Input: n}
	if err := s.repo.Create(ctx, &sup); err != nil {
		return domain.Customer{}, err
	}
	return sup, nil
}

// Get returns one customer.
func (s *Service) Get(ctx context.Context, id int64) (domain.Customer, error) {
	return s.repo.Get(ctx, id)
}

// Patch is a partial customer update; nil fields are left unchanged.
type Patch struct {
	Name           *string
	Identification *string
	Contact        *string
	Notes          *string
	Active         *bool
}

// Update applies a patch and revalidates the whole customer.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (domain.Customer, error) {
	cur, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Customer{}, err
	}
	in := domain.Input{
		Name: cur.Name, Identification: cur.Identification, Contact: cur.Contact,
		Notes: cur.Notes, Active: cur.Active,
	}
	if p.Name != nil {
		in.Name = *p.Name
	}
	if p.Identification != nil {
		in.Identification = *p.Identification
	}
	if p.Contact != nil {
		in.Contact = *p.Contact
	}
	if p.Notes != nil {
		in.Notes = *p.Notes
	}
	if p.Active != nil {
		in.Active = *p.Active
	}
	n, err := in.Normalize()
	if err != nil {
		return domain.Customer{}, err
	}
	cur.Name, cur.Identification, cur.Contact = n.Name, n.Identification, n.Contact
	cur.Notes, cur.Active = n.Notes, n.Active
	if err := s.repo.Update(ctx, &cur); err != nil {
		return domain.Customer{}, err
	}
	return cur, nil
}

// Delete soft-deletes a customer.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// List returns a filtered page of customers, the total and the normalized filter.
func (s *Service) List(ctx context.Context, f domain.Filter) ([]domain.Customer, int, domain.Filter, error) {
	nf, err := f.Normalize()
	if err != nil {
		return nil, 0, domain.Filter{}, err
	}
	out, total, err := s.repo.List(ctx, nf)
	if err != nil {
		return nil, 0, domain.Filter{}, err
	}
	return out, total, nf, nil
}
