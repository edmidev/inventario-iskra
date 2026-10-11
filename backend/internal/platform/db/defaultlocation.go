package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// DefaultLocationName is the location that holds consumable stock recorded
// without a location ("Sin ubicación").
const DefaultLocationName = "Sin ubicación"

// EnsureDefaultLocation returns the id of the default location, creating it
// (kind storage) when it does not exist yet. It runs inside the caller's
// transaction; a transaction-scoped advisory lock keeps concurrent callers
// from creating it twice.
func EnsureDefaultLocation(ctx context.Context, tx pgx.Tx) (int64, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('inventario.default_location'))`); err != nil {
		return 0, fmt.Errorf("lock default location: %w", err)
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM locations WHERE name = $1 ORDER BY id LIMIT 1`, DefaultLocationName).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return 0, fmt.Errorf("find default location: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO locations (name, kind) VALUES ($1, 'storage') RETURNING id`, DefaultLocationName).Scan(&id); err != nil {
		return 0, fmt.Errorf("create default location: %w", err)
	}
	return id, nil
}
