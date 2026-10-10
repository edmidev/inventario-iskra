package main

import (
	"time"

	"github.com/edgar/inventario/backend/internal/identity/adapters"
	"github.com/edgar/inventario/backend/internal/identity/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/identity/app"
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
