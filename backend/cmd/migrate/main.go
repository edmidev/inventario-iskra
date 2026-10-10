// Command migrate applies the embedded database migrations and exits.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/edgar/inventario/backend/internal/platform/config"
	"github.com/edgar/inventario/backend/internal/platform/db"
)

func main() {
	cfg := config.Load()
	if err := db.Migrate(context.Background(), cfg.DatabaseURL); err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}
