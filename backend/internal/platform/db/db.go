// Package db provides the PostgreSQL connection pool and migration runner.
package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/edgar/inventario/backend/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// NewPool opens a pgx pool and verifies connectivity.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, fmt.Errorf("db: DATABASE_URL is empty")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending embedded migrations.
func Migrate(ctx context.Context, url string) error {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("db: open for migrate: %w", err)
	}
	defer sqlDB.Close()
	return MigrateDB(ctx, sqlDB)
}

// MigrateDB applies pending migrations on an existing database handle.
func MigrateDB(ctx context.Context, sqlDB *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("db: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	return nil
}

var _ = stdlib.GetDefaultDriver // ensure the "pgx" database/sql driver is registered
