package app_test

import (
	"context"
	"errors"
	"github.com/edgar/inventario/backend/internal/customers/adapters/memory"
	"github.com/edgar/inventario/backend/internal/customers/app"
	"github.com/edgar/inventario/backend/internal/customers/domain"
	"testing"
)

func TestCustomerLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := app.NewService(memory.NewStore())
	c, err := svc.Create(ctx, domain.Input{Name: "  Acero SA ", Identification: "J-123", Contact: "Ana", Active: true})
	if err != nil || c.Name != "Acero SA" {
		t.Fatalf("create %+v %v", c, err)
	}
	name := "Nuevo"
	c, err = svc.Update(ctx, c.ID, app.Patch{Name: &name})
	if err != nil || c.Identification != "J-123" {
		t.Fatalf("patch %+v %v", c, err)
	}
	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted %v", err)
	}
}
func TestCustomerRequiresName(t *testing.T) {
	_, err := app.NewService(memory.NewStore()).Create(context.Background(), domain.Input{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid name %v", err)
	}
}
