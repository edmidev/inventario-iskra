// Package postgres wires the importer to the real services and repositories
// on one connection pool.
package postgres

import (
	"context"
	"errors"
	"fmt"

	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	identitypg "github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	identitydomain "github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/importer"
	maintenancepg "github.com/edgar/inventario/backend/internal/maintenance/adapters/postgres"
	maintenanceapp "github.com/edgar/inventario/backend/internal/maintenance/app"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	stockpg "github.com/edgar/inventario/backend/internal/stock/adapters/postgres"
	stockapp "github.com/edgar/inventario/backend/internal/stock/app"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openingBalanceHook mirrors the API wiring (cmd/api): a consumable created
// with stock gets its opening movement in the same transaction as the item,
// attributed to the acting user carried by the context.
func openingBalanceHook(ctx context.Context, tx pgx.Tx, it catalogdomain.Item) error {
	if it.Kind != catalogdomain.KindConsumable || len(it.Stocks) == 0 {
		return nil
	}
	userID := ""
	if p, ok := httpx.PrincipalFrom(ctx); ok {
		userID = p.UserID
	}
	stocks := make([]stockdomain.LocationStock, len(it.Stocks))
	for i, st := range it.Stocks {
		stocks[i] = stockdomain.LocationStock{LocationID: st.LocationID, LocationName: st.LocationName, Quantity: st.Quantity}
	}
	return stockpg.RecordOpenings(ctx, tx, it.ID, stocks, userID)
}

// NewDeps builds the importer's dependencies on pool: the same catalog and
// maintenance services the API uses.
func NewDeps(pool *pgxpool.Pool) importer.Deps {
	repo := catalogpg.New(pool, catalogpg.WithCreateHook(openingBalanceHook))
	return importer.Deps{
		Catalog:     catalogapp.NewService(repo.Categories(), repo.Locations(), repo.Items()),
		Maintenance: maintenanceapp.NewService(maintenancepg.New(pool), nil),
		Stock:       stockapp.NewService(stockpg.New(pool)),
		Lookup:      Lookup{pool: pool},
	}
}

// Lookup is the importer's read-only query for existing items. It is
// adapter-level SQL because no service searches by code list or provenance,
// and because soft-deleted items must be visible (their codes stay reserved).
type Lookup struct{ pool *pgxpool.Pool }

// Find implements importer.Lookup.
func (l Lookup) Find(ctx context.Context, codes, sources []string) (importer.Existing, error) {
	ex := importer.Existing{ByCode: map[string]importer.ExistingItem{}, BySource: map[string]importer.ExistingItem{}}
	rows, err := l.pool.Query(ctx, `
		SELECT id::text, code, description, COALESCE(attributes->>'import_source', ''), deleted_at IS NOT NULL
		FROM items
		WHERE code = ANY($1) OR attributes->>'import_source' = ANY($2)`, codes, sources)
	if err != nil {
		return ex, err
	}
	defer rows.Close()
	var found []importer.ExistingItem
	var codesOf []string
	for rows.Next() {
		var code string
		var e importer.ExistingItem
		if err := rows.Scan(&e.ID, &code, &e.Description, &e.ImportSource, &e.Deleted); err != nil {
			return ex, err
		}
		found, codesOf = append(found, e), append(codesOf, code)
	}
	if err := rows.Err(); err != nil {
		return ex, err
	}
	rows.Close()
	locs, err := l.locations(ctx, found)
	if err != nil {
		return ex, err
	}
	for i, e := range found {
		e.Locations = locs[e.ID]
		ex.ByCode[codesOf[i]] = e
		if e.ImportSource != "" {
			ex.BySource[e.ImportSource] = e
		}
	}
	return ex, nil
}

// locations returns, per item id, the names of the locations where the item
// has a stock row or ever had a stock movement.
func (l Lookup) locations(ctx context.Context, items []importer.ExistingItem) (map[string][]string, error) {
	out := map[string][]string{}
	if len(items) == 0 {
		return out, nil
	}
	ids := make([]string, len(items))
	for i, e := range items {
		ids[i] = e.ID
	}
	rows, err := l.pool.Query(ctx, `
		SELECT DISTINCT x.item_id::text, loc.name
		FROM (
			SELECT item_id, location_id FROM item_stocks
			UNION SELECT item_id, location_id FROM stock_movements WHERE location_id IS NOT NULL
			UNION SELECT item_id, to_location_id FROM stock_movements WHERE to_location_id IS NOT NULL
			UNION SELECT item_id, from_location_id FROM stock_movements WHERE from_location_id IS NOT NULL
		) x
		JOIN locations loc ON loc.id = x.location_id
		WHERE x.item_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = append(out[id], name)
	}
	return out, rows.Err()
}

// ResolveActor finds the active admin or operator the import acts as.
func ResolveActor(ctx context.Context, pool *pgxpool.Pool, email string) (importer.Actor, error) {
	norm, err := identitydomain.NormalizeEmail(email)
	if err != nil {
		return importer.Actor{}, fmt.Errorf("invalid --user-email: %w", err)
	}
	u, err := identitypg.New(pool).GetByEmail(ctx, norm)
	if errors.Is(err, identitydomain.ErrUserNotFound) {
		return importer.Actor{}, fmt.Errorf("no user with email %s", norm)
	}
	if err != nil {
		return importer.Actor{}, fmt.Errorf("look up user: %w", err)
	}
	if !u.Active {
		return importer.Actor{}, fmt.Errorf("user %s is inactive", norm)
	}
	if u.Role != identitydomain.RoleAdmin && u.Role != identitydomain.RoleOperator {
		return importer.Actor{}, fmt.Errorf("user %s has role %s; the import needs an admin or operator", norm, u.Role)
	}
	perms := make([]string, 0, 5)
	for _, p := range u.Role.Permissions() {
		perms = append(perms, string(p))
	}
	return importer.Actor{UserID: u.ID, Role: string(u.Role), Permissions: perms}, nil
}
