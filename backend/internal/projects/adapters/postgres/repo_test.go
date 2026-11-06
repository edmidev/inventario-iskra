package postgres_test

import (
	"context"
	"errors"
	customerpg "github.com/edgar/inventario/backend/internal/customers/adapters/postgres"
	customerdomain "github.com/edgar/inventario/backend/internal/customers/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	"github.com/edgar/inventario/backend/internal/projects/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/projects/domain"
	"os"
	"testing"
)

func TestTemplateAndCustomerPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("PostgreSQL integration")
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	if e := db.Migrate(ctx, url); e != nil {
		t.Fatal(e)
	}
	pool, e := db.NewPool(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if _, e = pool.Exec(ctx, `TRUNCATE project_templates,customers RESTART IDENTITY CASCADE`); e != nil {
		t.Fatal(e)
	}
	customers := customerpg.New(pool)
	c := customerdomain.Customer{Input: customerdomain.Input{Name: "Customer persistence", Identification: "J123", Contact: "Ana", Active: true}}
	if e = customers.Create(ctx, &c); e != nil {
		t.Fatal(e)
	}
	c.Contact = "Luis"
	if e = customers.Update(ctx, &c); e != nil {
		t.Fatal(e)
	}
	if got, e := customers.Get(ctx, c.ID); e != nil || got.Contact != "Luis" {
		t.Fatalf("customer %+v %v", got, e)
	}
	if e = customers.Delete(ctx, c.ID); e != nil {
		t.Fatal(e)
	}
	var deleted bool
	if e = pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM customers WHERE id=$1`, c.ID).Scan(&deleted); e != nil || !deleted {
		t.Fatalf("customer history erased %v", e)
	}
	repo := postgres.New(pool)
	q, _ := quantity.Parse("1.125")
	p := domain.Project{Input: domain.Input{Name: "Marmita persistence", EstimatedDays: 3, Characteristics: []string{"50 litros"}, Consumables: []domain.Material{{ItemID: "material", Quantity: q}}, Tools: []string{"tool"}}}
	if e = repo.Create(ctx, &p); e != nil {
		t.Fatal(e)
	}
	got, e := repo.Get(ctx, p.ID)
	if e != nil || got.Consumables[0].Quantity != q {
		t.Fatalf("project %+v %v", got, e)
	}
	p.Name = "Updated template"
	p.Consumables[0].Quantity = 2 * q
	if e = repo.Update(ctx, &p); e != nil {
		t.Fatal(e)
	}
	got, e = repo.Get(ctx, p.ID)
	if e != nil || got.Name != p.Name || got.Consumables[0].Quantity != 2*q {
		t.Fatalf("update %+v %v", got, e)
	}
	if e = repo.Delete(ctx, p.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.Get(ctx, p.ID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatalf("deleted project %v", e)
	}
	if e = pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM project_templates WHERE id=$1`, p.ID).Scan(&deleted); e != nil || !deleted {
		t.Fatalf("project history erased %v", e)
	}
}
