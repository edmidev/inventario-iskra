// Package app holds the supplier use cases and the port they depend on.
package app

import (
	"context"

	"github.com/edgar/inventario/backend/internal/suppliers/domain"
)

// Repository persists suppliers. Soft-deleted suppliers behave as missing.
type Repository interface {
	// Create stores s and fills its ID and timestamps. A name already used by
	// a live supplier (case-insensitive) yields ErrNameTaken.
	Create(ctx context.Context, s *domain.Supplier) error
	// Get returns a live supplier or ErrNotFound.
	Get(ctx context.Context, id int64) (domain.Supplier, error)
	// Update stores the editable fields of s and refreshes UpdatedAt.
	Update(ctx context.Context, s *domain.Supplier) error
	// Delete soft-deletes a live supplier or yields ErrNotFound.
	Delete(ctx context.Context, id int64) error
	// List returns a page of live suppliers ordered by name and the total count.
	List(ctx context.Context, f domain.Filter) ([]domain.Supplier, int, error)
}

// Service implements the supplier use cases.
type Service struct {
	repo Repository
}

// NewService wires the repository.
func NewService(r Repository) *Service { return &Service{repo: r} }

// Create validates and stores a supplier.
func (s *Service) Create(ctx context.Context, in domain.Input) (domain.Supplier, error) {
	n, err := in.Normalize()
	if err != nil {
		return domain.Supplier{}, err
	}
	sup := domain.Supplier{
		Name: n.Name, ContactName: n.ContactName, Email: n.Email,
		Phone: n.Phone, Notes: n.Notes, Active: n.Active,
	}
	if err := s.repo.Create(ctx, &sup); err != nil {
		return domain.Supplier{}, err
	}
	return sup, nil
}

// Get returns one supplier.
func (s *Service) Get(ctx context.Context, id int64) (domain.Supplier, error) {
	return s.repo.Get(ctx, id)
}

// Patch is a partial supplier update; nil fields are left unchanged.
type Patch struct {
	Name        *string
	ContactName *string
	Email       *string
	Phone       *string
	Notes       *string
	Active      *bool
}

// Update applies a patch and revalidates the whole supplier.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (domain.Supplier, error) {
	cur, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Supplier{}, err
	}
	in := domain.Input{
		Name: cur.Name, ContactName: cur.ContactName, Email: cur.Email,
		Phone: cur.Phone, Notes: cur.Notes, Active: cur.Active,
	}
	if p.Name != nil {
		in.Name = *p.Name
	}
	if p.ContactName != nil {
		in.ContactName = *p.ContactName
	}
	if p.Email != nil {
		in.Email = *p.Email
	}
	if p.Phone != nil {
		in.Phone = *p.Phone
	}
	if p.Notes != nil {
		in.Notes = *p.Notes
	}
	if p.Active != nil {
		in.Active = *p.Active
	}
	n, err := in.Normalize()
	if err != nil {
		return domain.Supplier{}, err
	}
	cur.Name, cur.ContactName, cur.Email = n.Name, n.ContactName, n.Email
	cur.Phone, cur.Notes, cur.Active = n.Phone, n.Notes, n.Active
	if err := s.repo.Update(ctx, &cur); err != nil {
		return domain.Supplier{}, err
	}
	return cur, nil
}

// Delete soft-deletes a supplier.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// List returns a filtered page of suppliers, the total and the normalized filter.
func (s *Service) List(ctx context.Context, f domain.Filter) ([]domain.Supplier, int, domain.Filter, error) {
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
