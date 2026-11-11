package main

import (
	"context"
	"errors"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	customerapi "github.com/edgar/inventario/backend/internal/customers/adapters/httpapi"
	customerpg "github.com/edgar/inventario/backend/internal/customers/adapters/postgres"
	customerapp "github.com/edgar/inventario/backend/internal/customers/app"
	identityapi "github.com/edgar/inventario/backend/internal/identity/adapters/httpapi"
	projectapi "github.com/edgar/inventario/backend/internal/projects/adapters/httpapi"
	projectpg "github.com/edgar/inventario/backend/internal/projects/adapters/postgres"
	projectapp "github.com/edgar/inventario/backend/internal/projects/app"
	"github.com/jackc/pgx/v5/pgxpool"
)

type projectCatalog struct{ catalog *catalogapp.Service }

func (p projectCatalog) Kind(ctx context.Context, id string) (string, error) {
	item, err := p.catalog.GetItem(ctx, id)
	if errors.Is(err, catalogdomain.ErrItemNotFound) {
		return "", projectapp.ErrItemNotFound
	}
	if err != nil {
		return "", err
	}
	return string(item.Kind), nil
}
func newProjectService(pool *pgxpool.Pool) *projectapp.Service {
	return projectapp.NewService(projectpg.New(pool), projectCatalog{catalog: newCatalogService(pool)})
}
func wireProjects(pool *pgxpool.Pool, identity *identityapi.Handler) *projectapi.Handler {
	return projectapi.New(newProjectService(pool), identity.Authenticate(), projectapi.Permissions{Read: "projects:read", Write: "projects:write"})
}
func wireCustomers(pool *pgxpool.Pool, identity *identityapi.Handler) *customerapi.Handler {
	return customerapi.New(customerapp.NewService(customerpg.New(pool)), identity.Authenticate(), customerapi.Permissions{Read: "customers:read", Write: "customers:write"})
}
