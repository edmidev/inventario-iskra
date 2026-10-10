// Package postgres implements the catalog repositories on PostgreSQL via pgx.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateHook runs inside the transaction that inserts a new item, after the
// insert. Returning an error rolls the whole creation back. It lets another
// module (stock) write its own rows atomically with the item without catalog
// importing that module.
type CreateHook func(ctx context.Context, tx pgx.Tx, it domain.Item) error

// Option customizes a Repo.
type Option func(*Repo)

// WithCreateHook registers a hook run in the item-creation transaction.
func WithCreateHook(h CreateHook) Option { return func(r *Repo) { r.onCreate = h } }

// Repo implements the catalog repository ports on one pool.
type Repo struct {
	pool     *pgxpool.Pool
	onCreate CreateHook
}

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool, opts ...Option) *Repo {
	r := &Repo{pool: pool}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Categories returns the category repository view.
func (r *Repo) Categories() *Categories { return &Categories{r.pool} }

// Locations returns the location repository view.
func (r *Repo) Locations() *Locations { return &Locations{r.pool} }

// Items returns the item repository view.
func (r *Repo) Items() *Items { return &Items{pool: r.pool, onCreate: r.onCreate} }

// Postgres error codes and constraint names this package maps to domain errors.
const (
	pgUnique     = "23505"
	pgForeignKey = "23503"
)

func pgError(err error) (code, constraint string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.ConstraintName
	}
	return "", ""
}

// ---- categories ----

// Categories implements app.CategoryRepository.
type Categories struct{ pool *pgxpool.Pool }

func mapCategoryErr(err error) error {
	code, constraint := pgError(err)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.ErrCategoryNotFound
	case code == pgUnique && constraint == "categories_name_lower_key":
		return domain.ErrCategoryNameTaken
	case code == pgUnique && constraint == "categories_code_prefix_key":
		return domain.ErrCategoryPrefixTaken
	case code == pgForeignKey && constraint == "items_category_id_fkey":
		return domain.ErrCategoryInUse
	}
	return err
}

// Create implements app.CategoryRepository.
func (c *Categories) Create(ctx context.Context, cat *domain.Category) error {
	err := c.pool.QueryRow(ctx,
		`INSERT INTO categories (name, code_prefix) VALUES ($1, $2) RETURNING id`,
		cat.Name, cat.CodePrefix).Scan(&cat.ID)
	if err != nil {
		return fmt.Errorf("create category: %w", mapCategoryErr(err))
	}
	return nil
}

// Get implements app.CategoryRepository.
func (c *Categories) Get(ctx context.Context, id int64) (domain.Category, error) {
	var cat domain.Category
	err := c.pool.QueryRow(ctx, `SELECT id, name, code_prefix FROM categories WHERE id = $1`, id).
		Scan(&cat.ID, &cat.Name, &cat.CodePrefix)
	if err != nil {
		return domain.Category{}, wrapQuery("get category", mapCategoryErr(err))
	}
	return cat, nil
}

// List implements app.CategoryRepository.
func (c *Categories) List(ctx context.Context) ([]domain.Category, error) {
	rows, err := c.pool.Query(ctx, `SELECT id, name, code_prefix FROM categories ORDER BY lower(name), id`)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	out := []domain.Category{}
	for rows.Next() {
		var cat domain.Category
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.CodePrefix); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		out = append(out, cat)
	}
	return out, rows.Err()
}

// Update implements app.CategoryRepository.
func (c *Categories) Update(ctx context.Context, cat *domain.Category) error {
	tag, err := c.pool.Exec(ctx, `UPDATE categories SET name = $2, code_prefix = $3 WHERE id = $1`,
		cat.ID, cat.Name, cat.CodePrefix)
	if err != nil {
		return wrapQuery("update category", mapCategoryErr(err))
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrCategoryNotFound
	}
	return nil
}

// Delete implements app.CategoryRepository. Items, including soft-deleted
// ones, keep a category alive through the foreign key.
func (c *Categories) Delete(ctx context.Context, id int64) error {
	tag, err := c.pool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, id)
	if err != nil {
		return wrapQuery("delete category", mapCategoryErr(err))
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrCategoryNotFound
	}
	return nil
}

// wrapQuery keeps domain errors unwrapped (so errors.Is works either way) and
// adds context to unexpected failures.
func wrapQuery(op string, err error) error {
	for _, d := range []error{
		domain.ErrCategoryNotFound, domain.ErrCategoryNameTaken, domain.ErrCategoryPrefixTaken, domain.ErrCategoryInUse,
		domain.ErrLocationNotFound, domain.ErrLocationInUse, domain.ErrItemNotFound, domain.ErrCodeTaken,
	} {
		if errors.Is(err, d) {
			return err
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}

// ---- locations ----

// Locations implements app.LocationRepository.
type Locations struct{ pool *pgxpool.Pool }

func mapLocationErr(err error) error {
	code, constraint := pgError(err)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.ErrLocationNotFound
	case code == pgForeignKey && constraint == "locations_parent_id_fkey":
		// Insert/update with a missing parent, or delete of a parent: the
		// caller distinguishes by operation.
		return err
	case code == pgForeignKey && constraint == "items_location_id_fkey":
		return domain.ErrLocationInUse
	}
	return err
}

// Create implements app.LocationRepository.
func (l *Locations) Create(ctx context.Context, loc *domain.Location) error {
	err := l.pool.QueryRow(ctx,
		`INSERT INTO locations (name, kind, parent_id) VALUES ($1, $2, $3) RETURNING id`,
		loc.Name, string(loc.Kind), loc.ParentID).Scan(&loc.ID)
	if code, c := pgError(err); code == pgForeignKey && c == "locations_parent_id_fkey" {
		return domain.ErrLocationNotFound
	}
	if err != nil {
		return fmt.Errorf("create location: %w", err)
	}
	return nil
}

// Get implements app.LocationRepository.
func (l *Locations) Get(ctx context.Context, id int64) (domain.Location, error) {
	var loc domain.Location
	var kind string
	err := l.pool.QueryRow(ctx, `SELECT id, name, kind, parent_id FROM locations WHERE id = $1`, id).
		Scan(&loc.ID, &loc.Name, &kind, &loc.ParentID)
	loc.Kind = domain.LocationKind(kind)
	if err != nil {
		return domain.Location{}, wrapQuery("get location", mapLocationErr(err))
	}
	return loc, nil
}

// List implements app.LocationRepository.
func (l *Locations) List(ctx context.Context) ([]domain.Location, error) {
	rows, err := l.pool.Query(ctx, `SELECT id, name, kind, parent_id FROM locations ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	defer rows.Close()
	out := []domain.Location{}
	for rows.Next() {
		var loc domain.Location
		var kind string
		if err := rows.Scan(&loc.ID, &loc.Name, &kind, &loc.ParentID); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		loc.Kind = domain.LocationKind(kind)
		out = append(out, loc)
	}
	return out, rows.Err()
}

// Update implements app.LocationRepository.
func (l *Locations) Update(ctx context.Context, loc *domain.Location) error {
	tag, err := l.pool.Exec(ctx, `UPDATE locations SET name = $2, kind = $3, parent_id = $4 WHERE id = $1`,
		loc.ID, loc.Name, string(loc.Kind), loc.ParentID)
	if code, c := pgError(err); code == pgForeignKey && c == "locations_parent_id_fkey" {
		return domain.ErrLocationNotFound
	}
	if err != nil {
		return fmt.Errorf("update location: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrLocationNotFound
	}
	return nil
}

// Delete implements app.LocationRepository.
func (l *Locations) Delete(ctx context.Context, id int64) error {
	tag, err := l.pool.Exec(ctx, `DELETE FROM locations WHERE id = $1`, id)
	if code, c := pgError(err); code == pgForeignKey && (c == "locations_parent_id_fkey" || c == "items_location_id_fkey") {
		return domain.ErrLocationInUse
	}
	if err != nil {
		return fmt.Errorf("delete location: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrLocationNotFound
	}
	return nil
}

// ---- items ----

// Items implements app.ItemRepository.
type Items struct {
	pool     *pgxpool.Pool
	onCreate CreateHook
}

const itemCols = `id::text, code, description, category_id, kind, status, unit, stock_current, stock_min,
	location_id, last_maintenance_at, maintenance_interval_days, attributes, notes, created_at, updated_at`

func scanItem(row pgx.Row) (domain.Item, error) {
	var it domain.Item
	var kind, status string
	var attrs []byte
	err := row.Scan(&it.ID, &it.Code, &it.Description, &it.CategoryID, &kind, &status, &it.Unit,
		&it.StockCurrent, &it.StockMin, &it.LocationID, &it.LastMaintenanceAt, &it.MaintenanceIntervalDays,
		&attrs, &it.Notes, &it.CreatedAt, &it.UpdatedAt)
	if err != nil {
		return domain.Item{}, err
	}
	it.Kind, it.Status = domain.Kind(kind), domain.Status(status)
	it.Attributes = map[string]any{}
	if err := json.Unmarshal(attrs, &it.Attributes); err != nil {
		return domain.Item{}, fmt.Errorf("decode attributes: %w", err)
	}
	return it, nil
}

func mapItemWriteErr(err error) error {
	code, constraint := pgError(err)
	switch {
	case code == pgUnique && constraint == "items_code_key":
		return domain.ErrCodeTaken
	case code == pgForeignKey && constraint == "items_category_id_fkey":
		return domain.ErrCategoryNotFound
	case code == pgForeignKey && constraint == "items_location_id_fkey":
		return domain.ErrLocationNotFound
	}
	return err
}

// Create implements app.ItemRepository.
func (r *Items) Create(ctx context.Context, it *domain.Item) error {
	attrs, err := json.Marshal(it.Attributes)
	if err != nil {
		return fmt.Errorf("encode attributes: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create item: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanItem(tx.QueryRow(ctx, `
		INSERT INTO items (code, description, category_id, kind, status, unit, stock_current, stock_min,
			location_id, last_maintenance_at, maintenance_interval_days, attributes, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13)
		RETURNING `+itemCols,
		it.Code, it.Description, it.CategoryID, string(it.Kind), string(it.Status), it.Unit, it.StockCurrent,
		it.StockMin, it.LocationID, it.LastMaintenanceAt, it.MaintenanceIntervalDays, string(attrs), it.Notes))
	if err != nil {
		return wrapQuery("create item", mapItemWriteErr(err))
	}
	if r.onCreate != nil {
		if err := r.onCreate(ctx, tx, created); err != nil {
			return fmt.Errorf("create item hook: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create item: %w", err)
	}
	*it = created
	return nil
}

func validUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

func itemNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrItemNotFound
	}
	return err
}

// Get implements app.ItemRepository.
func (r *Items) Get(ctx context.Context, id string) (domain.Item, error) {
	if !validUUID(id) {
		return domain.Item{}, domain.ErrItemNotFound
	}
	it, err := scanItem(r.pool.QueryRow(ctx,
		`SELECT `+itemCols+` FROM items WHERE id = $1::uuid AND deleted_at IS NULL`, id))
	if err != nil {
		return domain.Item{}, wrapQuery("get item", itemNotFound(err))
	}
	return it, nil
}

// GetByCode implements app.ItemRepository.
func (r *Items) GetByCode(ctx context.Context, code string) (domain.Item, error) {
	it, err := scanItem(r.pool.QueryRow(ctx,
		`SELECT `+itemCols+` FROM items WHERE code = $1 AND deleted_at IS NULL`, code))
	if err != nil {
		return domain.Item{}, wrapQuery("get item by code", itemNotFound(err))
	}
	return it, nil
}

// Update implements app.ItemRepository. code, kind and stock_current are never written.
func (r *Items) Update(ctx context.Context, it *domain.Item) error {
	attrs, err := json.Marshal(it.Attributes)
	if err != nil {
		return fmt.Errorf("encode attributes: %w", err)
	}
	updated, err := scanItem(r.pool.QueryRow(ctx, `
		UPDATE items SET description = $2, category_id = $3, status = $4, unit = $5, stock_min = $6,
			location_id = $7, last_maintenance_at = $8, maintenance_interval_days = $9,
			attributes = $10::jsonb, notes = $11, updated_at = now()
		WHERE id = $1::uuid AND deleted_at IS NULL
		RETURNING `+itemCols,
		it.ID, it.Description, it.CategoryID, string(it.Status), it.Unit, it.StockMin, it.LocationID,
		it.LastMaintenanceAt, it.MaintenanceIntervalDays, string(attrs), it.Notes))
	if err != nil {
		return wrapQuery("update item", itemNotFound(mapItemWriteErr(err)))
	}
	*it = updated
	return nil
}

// SoftDelete implements app.ItemRepository.
func (r *Items) SoftDelete(ctx context.Context, id string) error {
	if !validUUID(id) {
		return domain.ErrItemNotFound
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE items SET deleted_at = now(), updated_at = now() WHERE id = $1::uuid AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("soft delete item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrItemNotFound
	}
	return nil
}

// escapeLike escapes LIKE wildcards in user input.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// whereClause mirrors domain.ItemFilter.Matches in SQL.
func whereClause(f domain.ItemFilter) (string, []any) {
	conds := []string{"deleted_at IS NULL"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.Q != "" {
		p := arg(escapeLike(f.Q))
		conds = append(conds, fmt.Sprintf(
			`(code ILIKE '%%' || %s || '%%' ESCAPE '\' OR description ILIKE '%%' || %s || '%%' ESCAPE '\')`, p, p))
	}
	if f.CategoryID != nil {
		conds = append(conds, "category_id = "+arg(*f.CategoryID))
	}
	if f.Status != "" {
		conds = append(conds, "status = "+arg(string(f.Status)))
	}
	if f.Kind != "" {
		conds = append(conds, "kind = "+arg(string(f.Kind)))
	}
	if f.Availability != "" {
		statuses := domain.StatusesFor(f.Availability)
		names := make([]string, len(statuses))
		for i, s := range statuses {
			names[i] = string(s)
		}
		conds = append(conds, "status = ANY("+arg(names)+")")
	}
	if f.LocationID != nil {
		conds = append(conds, "location_id = "+arg(*f.LocationID))
	}
	if f.LowStock {
		conds = append(conds, "kind = 'consumable' AND stock_min > 0 AND stock_current <= stock_min")
	}
	return strings.Join(conds, " AND "), args
}

// List implements app.ItemRepository.
func (r *Items) List(ctx context.Context, f domain.ItemFilter) ([]domain.Item, int, error) {
	where, args := whereClause(f)
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM items WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count items: %w", err)
	}

	dir := "ASC"
	if f.Desc {
		dir = "DESC"
	}
	var order string
	switch f.Sort {
	case domain.SortDescription:
		order = "lower(description) " + dir + ", code " + dir
	case domain.SortUpdatedAt:
		order = "updated_at " + dir + ", code " + dir
	default:
		order = "code " + dir
	}
	limit := f.Limit
	if limit <= 0 {
		limit = domain.DefaultLimit
	}
	args = append(args, limit, f.Offset)
	q := fmt.Sprintf(`SELECT %s FROM items WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		itemCols, where, order, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	out := []domain.Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// NextSequence implements app.ItemRepository. The upsert is atomic per prefix
// and runs outside any other transaction, so concurrent callers never see
// the same value.
func (r *Items) NextSequence(ctx context.Context, prefix string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO code_sequences (prefix, last_value) VALUES ($1, 1)
		ON CONFLICT (prefix) DO UPDATE SET last_value = code_sequences.last_value + 1
		RETURNING last_value`, prefix).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("next sequence: %w", err)
	}
	return n, nil
}
