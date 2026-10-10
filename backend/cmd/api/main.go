// Command api starts the inventory HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/config"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// newRouter builds the root router; mount funcs register module routes under /api/v1.
func newRouter(mount ...func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", httpx.Health)
		for _, m := range mount {
			m(r)
		}
	})
	return r
}

func main() {
	cfg := config.Load()
	if err := cfg.ValidateServer(); err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		slog.Error("migrations failed", "err", err)
		os.Exit(1)
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database unavailable", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	identityHandler, identitySvc, err := wireIdentity(pool, cfg)
	if err != nil {
		slog.Error("identity wiring failed", "err", err)
		os.Exit(1)
	}
	created, err := identitySvc.Bootstrap(ctx, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword)
	if err != nil {
		slog.Error("bootstrap admin failed", "err", err)
		os.Exit(1)
	}
	if created {
		slog.Info("bootstrap admin created", "email", cfg.BootstrapAdminEmail)
	}

	catalogHandler := wireCatalog(pool, identityHandler)
	stockHandler := wireStock(pool, identityHandler)
	custodyHandler := wireCustody(pool, identityHandler)
	maintenanceHandler := wireMaintenance(pool, identityHandler)
	dashboardHandler := wireDashboard(pool, identityHandler)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           newRouter(identityHandler.Register, catalogHandler.Register, stockHandler.Register, custodyHandler.Register, maintenanceHandler.Register, dashboardHandler.Register),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("api listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
}
