package main

import (
	"context"
	"time"

	catalogapi "github.com/edgar/inventario/backend/internal/catalog/adapters/httpapi"
	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	custodyapi "github.com/edgar/inventario/backend/internal/custody/adapters/httpapi"
	custodypg "github.com/edgar/inventario/backend/internal/custody/adapters/postgres"
	custodyapp "github.com/edgar/inventario/backend/internal/custody/app"
	"github.com/edgar/inventario/backend/internal/identity/adapters"
	"github.com/edgar/inventario/backend/internal/identity/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	maintenanceapi "github.com/edgar/inventario/backend/internal/maintenance/adapters/httpapi"
	maintenancepg "github.com/edgar/inventario/backend/internal/maintenance/adapters/postgres"
	maintenanceapp "github.com/edgar/inventario/backend/internal/maintenance/app"
	"github.com/edgar/inventario/backend/internal/platform/config"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	stockapi "github.com/edgar/inventario/backend/internal/stock/adapters/httpapi"
	stockpg "github.com/edgar/inventario/backend/internal/stock/adapters/postgres"
	stockapp "github.com/edgar/inventario/backend/internal/stock/app"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Login throttling: 5 failures per client+email within 15 minutes.
const (
	loginMaxFails = 5
	loginWindow   = 15 * time.Minute
)

func wireIdentity(pool *pgxpool.Pool, cfg config.Config) (*httpapi.Handler, *app.Service, error) {
	issuer, err := adapters.NewJWTIssuer(cfg.JWTSecret, cfg.AccessTTL)
	if err != nil {
		return nil, nil, err
	}
	repo := postgres.New(pool)
	svc := app.NewService(app.Deps{
		Users:      repo,
		Tokens:     repo.Tokens(),
		Hasher:     adapters.DefaultArgon2idHasher(),
		Issuer:     issuer,
		Limiter:    app.NewMemoryLoginLimiter(loginMaxFails, loginWindow, nil),
		RefreshTTL: cfg.RefreshTTL,
	})
	return httpapi.New(svc, cfg.CookieSecure), svc, nil
}

// openingBalanceHook is the only place catalog creation and stock meet: a
// consumable created with stock gets its opening movement inside the same
// transaction as the item. The acting user comes from the request principal.
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

// wireCatalog builds the catalog handler, reusing identity's bearer-token
// middleware and permission names.
func wireCatalog(pool *pgxpool.Pool, identity *httpapi.Handler) *catalogapi.Handler {
	repo := catalogpg.New(pool, catalogpg.WithCreateHook(openingBalanceHook))
	svc := catalogapp.NewService(repo.Categories(), repo.Locations(), repo.Items())
	return catalogapi.New(svc, identity.Authenticate(), catalogapi.Permissions{
		Read:           string(domain.PermRead),
		WriteItems:     string(domain.PermWriteItems),
		ManageCatalogs: string(domain.PermManageCatalogs),
	})
}

// wireStock builds the stock handler with the same authentication and
// permission names as the catalog.
func wireStock(pool *pgxpool.Pool, identity *httpapi.Handler) *stockapi.Handler {
	svc := stockapp.NewService(stockpg.New(pool))
	return stockapi.New(svc, identity.Authenticate(), stockapi.Permissions{
		Read:       string(domain.PermRead),
		WriteItems: string(domain.PermWriteItems),
	})
}

// wireCustody builds the custody handler with the same authentication and
// permission names as the catalog.
func wireCustody(pool *pgxpool.Pool, identity *httpapi.Handler) *custodyapi.Handler {
	svc := custodyapp.NewService(custodypg.New(pool), nil)
	return custodyapi.New(svc, identity.Authenticate(), custodyapi.Permissions{
		Read:       string(domain.PermRead),
		WriteItems: string(domain.PermWriteItems),
	})
}

// wireMaintenance builds the maintenance handler with the same authentication
// and permission names as the catalog.
func wireMaintenance(pool *pgxpool.Pool, identity *httpapi.Handler) *maintenanceapi.Handler {
	svc := maintenanceapp.NewService(maintenancepg.New(pool), nil)
	return maintenanceapi.New(svc, identity.Authenticate(), maintenanceapi.Permissions{
		Read:       string(domain.PermRead),
		WriteItems: string(domain.PermWriteItems),
	})
}
