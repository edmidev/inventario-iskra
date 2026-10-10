// Package app holds the catalog use cases and the ports they depend on.
package app

import (
	"context"

	"github.com/edgar/inventario/backend/internal/catalog/domain"
)

// CategoryRepository persists categories. Implementations return
// ErrCategoryNameTaken / ErrCategoryPrefixTaken on uniqueness violations and
// ErrCategoryInUse when Delete hits items.
type CategoryRepository interface {
	Create(ctx context.Context, c *domain.Category) error
	Get(ctx context.Context, id int64) (domain.Category, error)
	List(ctx context.Context) ([]domain.Category, error)
	Update(ctx context.Context, c *domain.Category) error
	Delete(ctx context.Context, id int64) error
}

// LocationRepository persists locations. Delete returns ErrLocationInUse when
// the location has children or items. Create/Update return ErrLocationNotFound
// when the parent does not exist.
type LocationRepository interface {
	Create(ctx context.Context, l *domain.Location) error
	Get(ctx context.Context, id int64) (domain.Location, error)
	List(ctx context.Context) ([]domain.Location, error)
	Update(ctx context.Context, l *domain.Location) error
	Delete(ctx context.Context, id int64) error
}

// ItemRepository persists items. Reads never return soft-deleted items; their
// codes stay reserved (Create reports ErrCodeTaken for them too). Create and
// Update report ErrCategoryNotFound / ErrLocationNotFound for dangling foreign
// keys. Update never touches code, kind or stock_current.
type ItemRepository interface {
	Create(ctx context.Context, it *domain.Item) error
	Get(ctx context.Context, id string) (domain.Item, error)
	GetByCode(ctx context.Context, code string) (domain.Item, error)
	Update(ctx context.Context, it *domain.Item) error
	SoftDelete(ctx context.Context, id string) error
	List(ctx context.Context, f domain.ItemFilter) (items []domain.Item, total int, err error)
	// NextSequence atomically increments and returns the counter for a code
	// prefix. It is not rolled back with any other operation, so values may
	// be skipped but are never repeated.
	NextSequence(ctx context.Context, prefix string) (int, error)
}
