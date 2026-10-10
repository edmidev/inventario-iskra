package main

import (
	"time"

	catalogapi "github.com/edgar/inventario/backend/internal/catalog/adapters/httpapi"
	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	"github.com/edgar/inventario/backend/internal/identity/adapters"
	"github.com/edgar/inventario/backend/internal/identity/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/config"
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

// wireCatalog builds the catalog handler, reusing identity's bearer-token
// middleware and permission names.
func wireCatalog(pool *pgxpool.Pool, identity *httpapi.Handler) *catalogapi.Handler {
	repo := catalogpg.New(pool)
	svc := catalogapp.NewService(repo.Categories(), repo.Locations(), repo.Items())
	return catalogapi.New(svc, identity.Authenticate(), catalogapi.Permissions{
		Read:           string(domain.PermRead),
		WriteItems:     string(domain.PermWriteItems),
		ManageCatalogs: string(domain.PermManageCatalogs),
	})
}
