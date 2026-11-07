package main

import (
	executionapi "github.com/edgar/inventario/backend/internal/executions/adapters/httpapi"
	executionpg "github.com/edgar/inventario/backend/internal/executions/adapters/postgres"
	executionapp "github.com/edgar/inventario/backend/internal/executions/app"
	"github.com/edgar/inventario/backend/internal/identity/adapters/httpapi"
	projectpg "github.com/edgar/inventario/backend/internal/projects/adapters/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func wireExecutions(pool *pgxpool.Pool, identity *httpapi.Handler) *executionapi.Handler {
	return executionapi.New(executionapp.NewService(executionpg.New(pool), projectpg.New(pool)), identity.Authenticate(), executionapi.Permissions{Read: "executions:read", Write: "executions:write"})
}
