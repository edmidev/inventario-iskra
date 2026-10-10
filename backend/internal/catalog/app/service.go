package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/edgar/inventario/backend/internal/catalog/domain"
)

// maxCodeAttempts bounds retries when a generated code collides with a code
// that was supplied manually (legacy or imported).
const maxCodeAttempts = 50

// Service implements the catalog use cases.
type Service struct {
	cats CategoryRepository
	locs LocationRepository
	its  ItemRepository
}

// NewService wires the repositories.
func NewService(c CategoryRepository, l LocationRepository, i ItemRepository) *Service {
	return &Service{cats: c, locs: l, its: i}
}

// ---- categories ----

// ListCategories returns all categories.
func (s *Service) ListCategories(ctx context.Context) ([]domain.Category, error) {
	return s.cats.List(ctx)
}

// CreateCategory validates and stores a category.
func (s *Service) CreateCategory(ctx context.Context, name, prefix string) (domain.Category, error) {
	n, p, err := domain.NormalizeCategory(name, prefix)
	if err != nil {
		return domain.Category{}, err
	}
	c := domain.Category{Name: n, CodePrefix: p}
	if err := s.cats.Create(ctx, &c); err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

// UpdateCategory changes name and/or prefix. A new prefix only affects codes
// generated from now on.
func (s *Service) UpdateCategory(ctx context.Context, id int64, name, prefix *string) (domain.Category, error) {
	c, err := s.cats.Get(ctx, id)
	if err != nil {
		return domain.Category{}, err
	}
	newName, newPrefix := c.Name, c.CodePrefix
	if name != nil {
		newName = *name
	}
	if prefix != nil {
		newPrefix = *prefix
	}
	c.Name, c.CodePrefix, err = domain.NormalizeCategory(newName, newPrefix)
	if err != nil {
		return domain.Category{}, err
	}
	if err := s.cats.Update(ctx, &c); err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

// DeleteCategory removes a category that has no items.
func (s *Service) DeleteCategory(ctx context.Context, id int64) error {
	return s.cats.Delete(ctx, id)
}

// ---- locations ----

// ListLocations returns all locations.
func (s *Service) ListLocations(ctx context.Context) ([]domain.Location, error) {
	return s.locs.List(ctx)
}

// CreateLocation validates and stores a location.
func (s *Service) CreateLocation(ctx context.Context, name string, kind domain.LocationKind, parentID *int64) (domain.Location, error) {
	n, err := domain.NormalizeLocation(name, kind)
	if err != nil {
		return domain.Location{}, err
	}
	if err := s.checkParent(ctx, parentID); err != nil {
		return domain.Location{}, err
	}
	l := domain.Location{Name: n, Kind: kind, ParentID: parentID}
	if err := s.locs.Create(ctx, &l); err != nil {
		return domain.Location{}, mapParentMissing(err)
	}
	return l, nil
}

// LocationPatch is a partial location update.
type LocationPatch struct {
	Name     *string
	Kind     *domain.LocationKind
	ParentID domain.Nullable[int64]
}

// UpdateLocation applies a patch, rejecting parent cycles.
func (s *Service) UpdateLocation(ctx context.Context, id int64, p LocationPatch) (domain.Location, error) {
	l, err := s.locs.Get(ctx, id)
	if err != nil {
		return domain.Location{}, err
	}
	if p.Name != nil {
		l.Name = *p.Name
	}
	if p.Kind != nil {
		l.Kind = *p.Kind
	}
	if l.Name, err = domain.NormalizeLocation(l.Name, l.Kind); err != nil {
		return domain.Location{}, err
	}
	if p.ParentID.Set {
		l.ParentID = nil
		if !p.ParentID.Null {
			v := p.ParentID.Value
			l.ParentID = &v
		}
		if err := s.checkParent(ctx, l.ParentID); err != nil {
			return domain.Location{}, err
		}
		all, err := s.locs.List(ctx)
		if err != nil {
			return domain.Location{}, err
		}
		parents := make(map[int64]*int64, len(all))
		for _, o := range all {
			parents[o.ID] = o.ParentID
		}
		if err := domain.CheckNoCycle(id, l.ParentID, parents); err != nil {
			return domain.Location{}, err
		}
	}
	if err := s.locs.Update(ctx, &l); err != nil {
		return domain.Location{}, mapParentMissing(err)
	}
	return l, nil
}

// DeleteLocation removes a location without children or items.
func (s *Service) DeleteLocation(ctx context.Context, id int64) error {
	return s.locs.Delete(ctx, id)
}

func (s *Service) checkParent(ctx context.Context, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	if _, err := s.locs.Get(ctx, *parentID); err != nil {
		return mapParentMissing(err)
	}
	return nil
}

func mapParentMissing(err error) error {
	if errors.Is(err, domain.ErrLocationNotFound) {
		return domain.FieldError("parent_id", "location does not exist")
	}
	return err
}

// ---- items ----

// CreateItemInput is a new item. Code is optional: leave empty to generate
// one (callers must be allowed to supply codes, e.g. admin or importer).
type CreateItemInput struct {
	Code string
	domain.NewItemInput
}

// CreateItem creates an item, generating its code atomically when none is given.
func (s *Service) CreateItem(ctx context.Context, in CreateItemInput) (domain.Item, error) {
	it, err := domain.NewItem(in.NewItemInput)
	if err != nil {
		return domain.Item{}, err
	}
	cat, err := s.cats.Get(ctx, it.CategoryID)
	if errors.Is(err, domain.ErrCategoryNotFound) {
		return domain.Item{}, domain.FieldError("category_id", "category does not exist")
	}
	if err != nil {
		return domain.Item{}, err
	}
	if it.LocationID != nil {
		if _, err := s.locs.Get(ctx, *it.LocationID); errors.Is(err, domain.ErrLocationNotFound) {
			return domain.Item{}, domain.FieldError("location_id", "location does not exist")
		} else if err != nil {
			return domain.Item{}, err
		}
	}

	if in.Code != "" {
		if err := domain.ValidateCode(in.Code); err != nil {
			return domain.Item{}, err
		}
		it.Code = in.Code
		if err := s.its.Create(ctx, &it); err != nil {
			return domain.Item{}, mapItemFK(err)
		}
		return it, nil
	}

	for attempt := 0; attempt < maxCodeAttempts; attempt++ {
		seq, err := s.its.NextSequence(ctx, cat.CodePrefix)
		if err != nil {
			return domain.Item{}, fmt.Errorf("next code sequence: %w", err)
		}
		it.Code = domain.GenerateCode(cat.CodePrefix, seq)
		err = s.its.Create(ctx, &it)
		if errors.Is(err, domain.ErrCodeTaken) {
			continue // collided with a manually supplied code; take the next number
		}
		if err != nil {
			return domain.Item{}, mapItemFK(err)
		}
		return it, nil
	}
	return domain.Item{}, fmt.Errorf("could not allocate a free code for prefix %s", cat.CodePrefix)
}

func mapItemFK(err error) error {
	switch {
	case errors.Is(err, domain.ErrCategoryNotFound):
		return domain.FieldError("category_id", "category does not exist")
	case errors.Is(err, domain.ErrLocationNotFound):
		return domain.FieldError("location_id", "location does not exist")
	}
	return err
}

// GetItem returns an item by id.
func (s *Service) GetItem(ctx context.Context, id string) (domain.Item, error) {
	return s.its.Get(ctx, id)
}

// GetItemByCode returns an item by its code (used when scanning QR labels).
func (s *Service) GetItemByCode(ctx context.Context, code string) (domain.Item, error) {
	return s.its.GetByCode(ctx, code)
}

// UpdateItem applies a partial update. Kind, code and stock are immutable here.
func (s *Service) UpdateItem(ctx context.Context, id string, p domain.ItemPatch) (domain.Item, error) {
	it, err := s.its.Get(ctx, id)
	if err != nil {
		return domain.Item{}, err
	}
	updated, err := it.Apply(p)
	if err != nil {
		return domain.Item{}, err
	}
	if err := s.its.Update(ctx, &updated); err != nil {
		return domain.Item{}, mapItemFK(err)
	}
	return updated, nil
}

// DeleteItem soft-deletes an item; its code stays reserved.
func (s *Service) DeleteItem(ctx context.Context, id string) error {
	return s.its.SoftDelete(ctx, id)
}

// ListItems returns one page of items plus the total number of matches.
func (s *Service) ListItems(ctx context.Context, f domain.ItemFilter) ([]domain.Item, int, domain.ItemFilter, error) {
	f, err := f.Normalize()
	if err != nil {
		return nil, 0, f, err
	}
	items, total, err := s.its.List(ctx, f)
	return items, total, f, err
}
