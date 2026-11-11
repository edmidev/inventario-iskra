package app_test

import (
	"context"
	"errors"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	"github.com/edgar/inventario/backend/internal/projects/adapters/memory"
	"github.com/edgar/inventario/backend/internal/projects/app"
	"github.com/edgar/inventario/backend/internal/projects/domain"
	"testing"
)

type catalog map[string]string

func (c catalog) Kind(_ context.Context, id string) (string, error) {
	if k, ok := c[id]; ok {
		return k, nil
	}
	return "", app.ErrItemNotFound
}
func TestTemplateLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := app.NewService(memory.NewStore(), catalog{"screw": "consumable", "welder": "asset"})
	q, _ := quantity.Parse("2.125")
	in := domain.Input{Name: "Marmita", EstimatedDays: 10, Characteristics: []string{" Acero "}, Consumables: []domain.Material{{ItemID: "screw", Quantity: q}}, Tools: []string{"welder"}}
	p, e := svc.Create(ctx, in)
	if e != nil || p.Characteristics[0] != "Acero" {
		t.Fatalf("create %+v %v", p, e)
	}
	got, e := svc.Get(ctx, p.ID)
	if e != nil || got.Consumables[0].Quantity != q {
		t.Fatalf("get %+v %v", got, e)
	}
	if e = svc.Delete(ctx, p.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Get(ctx, p.ID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatalf("deleted %v", e)
	}
}
func TestRejectsWrongKindsDuplicateAndNonpositiveNeeds(t *testing.T) {
	svc := app.NewService(memory.NewStore(), catalog{"tool": "asset", "material": "consumable"})
	one, _ := quantity.Parse("1")
	cases := []domain.Input{{Name: "x", EstimatedDays: 1, Consumables: []domain.Material{{ItemID: "tool", Quantity: one}}}, {Name: "x", EstimatedDays: 1, Tools: []string{"material"}}, {Name: "x", EstimatedDays: 1, Tools: []string{"tool", "tool"}}, {Name: "x", EstimatedDays: 1, Consumables: []domain.Material{{ItemID: "material", Quantity: 0}}}, {Name: "x", EstimatedDays: 0}}
	for i, in := range cases {
		if _, err := svc.Create(context.Background(), in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("case%d: %v", i, err)
		}
	}
}
