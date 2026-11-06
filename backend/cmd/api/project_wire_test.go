package main

import (
	"errors"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	identitydomain "github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	projectapp "github.com/edgar/inventario/backend/internal/projects/app"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
	"testing"
)

func TestProjectCatalogBridgeAndFractionalRequirements(t *testing.T) {
	e, ctx := setupReal(t)
	material, err := e.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{Description: "Sheet", Unit: ptr("m"), CategoryID: e.conCat, Kind: catalogdomain.KindConsumable}})
	if err != nil {
		t.Fatal(err)
	}
	svc := newProjectService(e.pool)
	q, err := quantity.Parse("1.125")
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Create(ctx, projectdomain.Input{Name: "Marmita", EstimatedDays: 4, Consumables: []projectdomain.Material{{ItemID: material.ID, Quantity: q}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, p.ID)
	if err != nil || got.Consumables[0].Quantity != q {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	_, err = svc.Create(ctx, projectdomain.Input{Name: "Wrong tool", EstimatedDays: 4, Tools: []string{material.ID}})
	if !errors.Is(err, projectdomain.ErrInvalidInput) {
		t.Fatalf("consumable tool accepted: %v", err)
	}
	_, err = (projectCatalog{catalog: e.catalog}).Kind(ctx, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, projectapp.ErrItemNotFound) {
		t.Fatalf("missing kind=%v", err)
	}
}
func TestManufacturingPermissionsAreConfigurable(t *testing.T) {
	for _, p := range []identitydomain.Permission{"customers:read", "customers:write", "projects:read", "projects:write"} {
		if err := identitydomain.ValidateRolePermissions(identitydomain.RoleWorker, []identitydomain.Permission{p}); err != nil {
			t.Fatalf("grant %s: %v", p, err)
		}
	}
}
