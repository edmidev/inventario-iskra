// Package app owns project-template use cases and integration ports.
package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/edgar/inventario/backend/internal/projects/domain"
)

// Repository persists mutable templates. Deletions must retain historical rows.
type Repository interface {
	Create(context.Context, *domain.Project) error
	Get(context.Context, int64) (domain.Project, error)
	Update(context.Context, *domain.Project) error
	Delete(context.Context, int64) error
	List(context.Context, domain.Filter) ([]domain.Project, int, error)
}

// ErrItemNotFound identifies a missing or deleted inventory reference.
var ErrItemNotFound = errors.New("inventory item not found")

// ItemCatalog returns the kind of a live inventory item, or an error when absent.
type ItemCatalog interface {
	Kind(context.Context, string) (string, error)
}
type Service struct {
	repo    Repository
	catalog ItemCatalog
}

func NewService(r Repository, c ItemCatalog) *Service { return &Service{r, c} }
func (s *Service) validate(ctx context.Context, in domain.Input) (domain.Input, error) {
	n, e := in.Normalize()
	if e != nil {
		return n, e
	}
	fields := map[string]string{}
	for i, m := range n.Consumables {
		k, e := s.catalog.Kind(ctx, m.ItemID)
		if e != nil && !errors.Is(e, ErrItemNotFound) {
			return domain.Input{}, e
		}
		if e != nil || k != "consumable" {
			fields[fmt.Sprintf("consumables[%d].item_id", i)] = "must reference an existing consumable"
		}
	}
	for i, id := range n.Tools {
		k, e := s.catalog.Kind(ctx, id)
		if e != nil && !errors.Is(e, ErrItemNotFound) {
			return domain.Input{}, e
		}
		if e != nil || k != "asset" {
			fields[fmt.Sprintf("tools[%d]", i)] = "must reference an existing asset"
		}
	}
	if len(fields) > 0 {
		return domain.Input{}, &domain.ValidationError{Fields: fields}
	}
	return n, nil
}
func (s *Service) Create(ctx context.Context, in domain.Input) (domain.Project, error) {
	n, e := s.validate(ctx, in)
	if e != nil {
		return domain.Project{}, e
	}
	p := domain.Project{Input: n}
	e = s.repo.Create(ctx, &p)
	return p, e
}
func (s *Service) Get(ctx context.Context, id int64) (domain.Project, error) {
	return s.repo.Get(ctx, id)
}

// Update replaces only the reusable template; existing execution snapshots are independent.
func (s *Service) Update(ctx context.Context, id int64, in domain.Input) (domain.Project, error) {
	p, e := s.repo.Get(ctx, id)
	if e != nil {
		return p, e
	}
	n, e := s.validate(ctx, in)
	if e != nil {
		return domain.Project{}, e
	}
	p.Input = n
	e = s.repo.Update(ctx, &p)
	return p, e
}
func (s *Service) Delete(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }
func (s *Service) List(ctx context.Context, f domain.Filter) ([]domain.Project, int, domain.Filter, error) {
	nf, e := f.Normalize()
	if e != nil {
		return nil, 0, nf, e
	}
	p, n, e := s.repo.List(ctx, nf)
	return p, n, nf, e
}
