package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func pool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration tests")
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	p, err := db.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if _, err := p.Exec(ctx, `TRUNCATE suppliers RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return p, ctx
}

func count(t *testing.T, p *pgxpool.Pool, ctx context.Context) int {
	t.Helper()
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM suppliers`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBeginWithoutAmbientTxOwnsTheTransaction(t *testing.T) {
	p, ctx := pool(t)
	tx, commit, rollback, err := db.Begin(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO suppliers (name) VALUES ('a')`); err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	rollback() // deferred rollback after commit must be harmless
	if n := count(t, p, ctx); n != 1 {
		t.Fatalf("committed rows = %d", n)
	}

	_, _, rollback, err = db.Begin(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	rollback()
}

func TestBeginJoinsAmbientTxAndOwnerDecidesOutcome(t *testing.T) {
	p, ctx := pool(t)
	outer, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	joined := db.WithTx(ctx, outer)

	tx, commit, rollback, err := db.Begin(joined, p)
	if err != nil {
		t.Fatal(err)
	}
	if tx != outer {
		t.Fatal("Begin did not return the ambient transaction")
	}
	if _, err := tx.Exec(joined, `INSERT INTO suppliers (name) VALUES ('b')`); err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil { // no-op: the owner has not committed
		t.Fatal(err)
	}
	rollback() // no-op as well
	if n := count(t, p, ctx); n != 0 {
		t.Fatalf("row visible before the owner committed: %d", n)
	}
	if _, err := outer.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatalf("joined commit/rollback must not end the owner's transaction: %v", err)
	}
	if err := outer.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, p, ctx); n != 0 {
		t.Fatalf("rows after owner rollback = %d", n)
	}
}
