package postgres_test

import (
	"database/sql"
	"strings"
	"testing"

	qtyval "github.com/edgar/inventario/backend/internal/platform/quantity"
	"github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/edgar/inventario/backend/migrations"
	"github.com/pressly/goose/v3"
)

func TestFractionalStockLedgerAndLosslessMigrationGuard(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.consumable(t, "Gas", 2, 1, &f.locA)
	q, err := qtyval.Parse("0.125")
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: q, LocationID: &f.locA})
	if err != nil {
		t.Fatal(err)
	}
	if got.StockCurrent.String() != "1.875" || got.Movement.StockBefore.String() != "2" || got.Movement.Qty.String() != "-0.125" {
		t.Fatalf("fractional movement %+v", got)
	}
	var stock, sum qtyval.Quantity
	if err = f.pool.QueryRow(ctx, `SELECT stock_current,(SELECT sum(quantity) FROM item_stocks WHERE item_id=items.id) FROM items WHERE id=$1`, it.ID).Scan(&stock, &sum); err != nil || stock != sum || stock.String() != "1.875" {
		t.Fatalf("balance %s %s %v", stock, sum, err)
	}
	db, err := sql.Open("pgx", f.url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 12); err == nil || !strings.Contains(err.Error(), "fractional") {
		t.Fatalf("lossy rollback accepted: %v", err)
	}
	var after qtyval.Quantity
	if err = f.pool.QueryRow(ctx, `SELECT stock_current FROM items WHERE id=$1`, it.ID).Scan(&after); err != nil || after != stock {
		t.Fatalf("failed rollback mutated stock %s %v", after, err)
	}
}

func TestDecimalMigrationPreservesWholeUnitHistory(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.consumable(t, "Legacy quantity", 7, 2, &f.locA)
	db, err := sql.Open("pgx", f.url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	var old int
	if err = f.pool.QueryRow(ctx, `SELECT stock_current FROM items WHERE id=$1`, it.ID).Scan(&old); err != nil || old != 7 {
		t.Fatalf("legacy integer %d %v", old, err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var current, ledger qtyval.Quantity
	if err = f.pool.QueryRow(ctx, `SELECT stock_current,(SELECT sum(qty) FROM stock_movements WHERE item_id=items.id) FROM items WHERE id=$1`, it.ID).Scan(&current, &ledger); err != nil || current.String() != "7" || current != ledger {
		t.Fatalf("history changed %s %s %v", current, ledger, err)
	}
}
