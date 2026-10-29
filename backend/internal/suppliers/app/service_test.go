package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/edgar/inventario/backend/internal/suppliers/adapters/memory"
	"github.com/edgar/inventario/backend/internal/suppliers/app"
	"github.com/edgar/inventario/backend/internal/suppliers/domain"
)

func ptr[T any](v T) *T { return &v }

func newSvc() *app.Service { return app.NewService(memory.NewStore()) }

func TestCreateValidatesAndRejectsDuplicateNames(t *testing.T) {
	ctx := context.Background()
	svc := newSvc()
	if _, err := svc.Create(ctx, domain.Input{Name: ""}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := svc.Create(ctx, domain.Input{Name: "Ferretería Sur", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, domain.Input{Name: "ferretería sur"}); !errors.Is(err, domain.ErrNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
}

func TestUpdatePatchesOnlyGivenFieldsAndRevalidates(t *testing.T) {
	ctx := context.Background()
	svc := newSvc()
	s, _ := svc.Create(ctx, domain.Input{Name: "Sur", Email: "a@sur.test", Phone: "1", Active: true})

	got, err := svc.Update(ctx, s.ID, app.Patch{Phone: ptr("2"), Active: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Phone != "2" || got.Active || got.Name != "Sur" || got.Email != "a@sur.test" {
		t.Fatalf("patch result: %+v", got)
	}
	if _, err := svc.Update(ctx, s.ID, app.Patch{Email: ptr("nope")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("bad email: %v", err)
	}
	if _, err := svc.Update(ctx, 999, app.Patch{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	other, _ := svc.Create(ctx, domain.Input{Name: "Norte", Active: true})
	if _, err := svc.Update(ctx, other.ID, app.Patch{Name: ptr("SUR")}); !errors.Is(err, domain.ErrNameTaken) {
		t.Fatalf("rename onto taken name: %v", err)
	}
}

func TestDeleteAndList(t *testing.T) {
	ctx := context.Background()
	svc := newSvc()
	a, _ := svc.Create(ctx, domain.Input{Name: "Alfa", Active: true})
	_, _ = svc.Create(ctx, domain.Input{Name: "Beta", Active: false})
	_, _ = svc.Create(ctx, domain.Input{Name: "Gamma", ContactName: "Alfonso", Active: true})

	got, total, nf, err := svc.List(ctx, domain.Filter{Q: "alf"})
	if err != nil || total != 2 || len(got) != 2 || nf.Limit != domain.DefaultLimit {
		t.Fatalf("search: %v total=%d got=%d nf=%+v", err, total, len(got), nf)
	}
	got, total, _, _ = svc.List(ctx, domain.Filter{Active: ptr(false)})
	if total != 1 || got[0].Name != "Beta" {
		t.Fatalf("inactive filter: %+v", got)
	}
	if _, _, _, err := svc.List(ctx, domain.Filter{Limit: 1000}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("limit bound: %v", err)
	}

	if err := svc.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if err := svc.Delete(ctx, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}
