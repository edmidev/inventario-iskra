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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openingBalanceHook mirrors the API wiring (cmd/api): a consumable created
// with stock gets its opening movement in the same transaction as the item,
// attributed to the acting user carried by the context.
func openingBalanceHook(ctx context.Context, tx pgx.Tx, it catalogdomain.Item) error {
	if it.Kind != catalogdomain.KindConsumable || it.StockCurrent <= 0 {
		return nil
	}
	userID := ""
	if p, ok := httpx.PrincipalFrom(ctx); ok {
		userID = p.UserID
	}
	return stockpg.RecordOpening(ctx, tx, it.ID, it.StockCurrent, it.LocationID, userID)
}

// NewDeps builds the importer's dependencies on pool: the same catalog and
// maintenance services the API uses.
func NewDeps(pool *pgxpool.Pool) importer.Deps {
	repo := catalogpg.New(pool, catalogpg.WithCreateHook(openingBalanceHook))
	return importer.Deps{
		Catalog:     catalogapp.NewService(repo.Categories(), repo.Locations(), repo.Items()),
		Maintenance: maintenanceapp.NewService(maintenancepg.New(pool), nil),
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
	for rows.Next() {
		var code string
		var e importer.ExistingItem
		if err := rows.Scan(&e.ID, &code, &e.Description, &e.ImportSource, &e.Deleted); err != nil {
			return ex, err
		}
		ex.ByCode[code] = e
		if e.ImportSource != "" {
			ex.BySource[e.ImportSource] = e
		}
	}
	return ex, rows.Err()
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
