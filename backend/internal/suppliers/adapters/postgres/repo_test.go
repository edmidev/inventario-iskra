package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/suppliers/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/suppliers/app"
	"github.com/edgar/inventario/backend/internal/suppliers/domain"
)

func ptr[T any](v T) *T { return &v }

func setup(t *testing.T) (*app.Service, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration tests")
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE suppliers RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return app.NewService(postgres.New(pool)), ctx
}

func TestNameUniqueAmongLiveSuppliers(t *testing.T) {
	svc, ctx := setup(t)
	a, err := svc.Create(ctx, domain.Input{Name: "Ferretería Sur", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, domain.Input{Name: "FERRETERÍA SUR"}); !errors.Is(err, domain.ErrNameTaken) {
		t.Fatalf("case-insensitive duplicate: %v", err)
	}
	b, _ := svc.Create(ctx, domain.Input{Name: "Norte", Active: true})
	if _, err := svc.Update(ctx, b.ID, app.Patch{Name: ptr("ferretería sur")}); !errors.Is(err, domain.ErrNameTaken) {
		t.Fatalf("rename onto taken: %v", err)
	}
	// A soft-deleted name can be reused.
	if err := svc.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, domain.Input{Name: "Ferretería Sur", Active: true}); err != nil {
		t.Fatalf("reuse deleted name: %v", err)
	}
}

func TestSoftDeleteHidesFromGetUpdateDeleteAndList(t *testing.T) {
	svc, ctx := setup(t)
	s, _ := svc.Create(ctx, domain.Input{Name: "Sur", Active: true})
	if err := svc.Delete(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Update(ctx, s.ID, app.Patch{Phone: ptr("1")}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update: %v", err)
	}
	if err := svc.Delete(ctx, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, total, _, _ := svc.List(ctx, domain.Filter{}); total != 0 {
		t.Fatalf("list total = %d", total)
	}
}

func TestListSearchFilterOrderAndPaging(t *testing.T) {
	svc, ctx := setup(t)
	for _, in := range []domain.Input{
		{Name: "beta tools", ContactName: "Ana", Active: false},
		{Name: "Alfa", Email: "ventas@alfa.test", Active: true},
		{Name: "100% Pernos", Active: true},
		{Name: "Gamma", ContactName: "Alfonso", Active: true},
	} {
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatal(err)
		}
	}

	got, total, _, err := svc.List(ctx, domain.Filter{Limit: 2, Offset: 1})
	if err != nil || total != 4 || len(got) != 2 || got[0].Name != "Alfa" || got[1].Name != "beta tools" {
		t.Fatalf("order/paging: err=%v total=%d got=%+v", err, total, got)
	}
	// Search covers name, contact name and email, case-insensitively.
	for q, want := range map[string]int{"alf": 2, "ANA": 1, "ventas@": 1, "zzz": 0} {
		if _, total, _, _ := svc.List(ctx, domain.Filter{Q: q}); total != want {
			t.Fatalf("q=%q total=%d want %d", q, total, want)
		}
	}
	// LIKE wildcards in the query are literal.
	if got, total, _, _ := svc.List(ctx, domain.Filter{Q: "100%"}); total != 1 || got[0].Name != "100% Pernos" {
		t.Fatalf("literal %%: total=%d %+v", total, got)
	}
	if _, total, _, _ := svc.List(ctx, domain.Filter{Q: "_"}); total != 0 {
		t.Fatalf("literal underscore matched %d rows", total)
	}
	if _, total, _, _ := svc.List(ctx, domain.Filter{Active: ptr(false)}); total != 1 {
		t.Fatalf("inactive total = %d", total)
	}
}
