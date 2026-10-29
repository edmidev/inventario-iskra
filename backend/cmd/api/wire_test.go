package main

import (
	"context"
	"errors"
	"os"
	"testing"

	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	purchasingdomain "github.com/edgar/inventario/backend/internal/purchasing/domain"
	supplierspg "github.com/edgar/inventario/backend/internal/suppliers/adapters/postgres"
	suppliersapp "github.com/edgar/inventario/backend/internal/suppliers/app"
	suppliersdomain "github.com/edgar/inventario/backend/internal/suppliers/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests run purchasing on the real catalog and stock services, exactly as
// newPurchasingService wires them, against a real PostgreSQL.

type realEnv struct {
	pool     *pgxpool.Pool
	catalog  *catalogapp.Service
	userID   string
	supplier int64
	loc      int64
	lapCat   int64
	conCat   int64
}

func ptr[T any](v T) *T { return &v }

func setupReal(t *testing.T) (realEnv, context.Context) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE suppliers, items, code_sequences, locations, categories, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	e := realEnv{pool: pool, catalog: newCatalogService(pool)}
	if err := pool.QueryRow(ctx, `INSERT INTO users (name, email, password_hash, role) VALUES ('Ana', 'ana@example.com', 'x', 'operator') RETURNING id::text`).Scan(&e.userID); err != nil {
		t.Fatal(err)
	}
	sup, err := suppliersapp.NewService(supplierspg.New(pool)).Create(ctx, suppliersdomain.Input{Name: "Ferretería Sur", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	e.supplier = sup.ID
	lap, err := e.catalog.CreateCategory(ctx, "Laptops", "LAP")
	if err != nil {
		t.Fatal(err)
	}
	con, err := e.catalog.CreateCategory(ctx, "Consumibles", "CON")
	if err != nil {
		t.Fatal(err)
	}
	loc, err := e.catalog.CreateLocation(ctx, "Bodega", catalogdomain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.lapCat, e.conCat, e.loc = lap.ID, con.ID, loc.ID
	return e, ctx
}

func (e realEnv) codes(t *testing.T, desc string) []string {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT code FROM items WHERE description = $1 ORDER BY code`, desc)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

func TestReceivingThroughRealCatalogAndStock(t *testing.T) {
	e, ctx := setupReal(t)
	svc := newPurchasingService(e.pool)

	existing, err := e.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Papel A4", CategoryID: e.conCat, Kind: catalogdomain.KindConsumable, Unit: ptr("resma")}})
	if err != nil {
		t.Fatal(err)
	}

	o, err := svc.Create(ctx, e.userID, purchasingdomain.Input{
		SupplierID: e.supplier, Currency: purchasingdomain.VES,
		Lines: []purchasingdomain.LineInput{
			{NewItem: &purchasingdomain.NewItem{Description: "Laptop", CategoryID: e.lapCat, Kind: "asset", Attributes: map[string]any{"ram": "16GB"}}, Quantity: 3, UnitCostMinor: 90000},
			{ItemID: existing.ID, Quantity: 10, UnitCostMinor: 450},
			{NewItem: &purchasingdomain.NewItem{Description: "Tinta", CategoryID: e.conCat, Kind: "consumable", Unit: "u", StockMin: 2}, Quantity: 8, UnitCostMinor: 1200},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if o, err = svc.Place(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	line := func(i, q int) purchasingdomain.ReceiveLine {
		return purchasingdomain.ReceiveLine{LineID: o.Lines[i].ID, Quantity: q}
	}

	// First delivery: two laptops, part of the paper and the ink.
	r1, err := svc.Receive(ctx, o.ID, e.userID, purchasingdomain.ReceiveRequest{
		LocationID: e.loc, Note: "primer camión", Lines: []purchasingdomain.ReceiveLine{line(0, 2), line(1, 4), line(2, 5)}})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Order.Status != purchasingdomain.StatusPartiallyReceived {
		t.Fatalf("status %s", r1.Order.Status)
	}
	if got := e.codes(t, "Laptop"); len(got) != 2 || got[0] != "ISK-LAP-001" || got[1] != "ISK-LAP-002" {
		t.Fatalf("laptop codes after the first delivery: %v", got)
	}
	// Second delivery completes everything.
	r2, err := svc.Receive(ctx, o.ID, e.userID, purchasingdomain.ReceiveRequest{
		LocationID: e.loc, Lines: []purchasingdomain.ReceiveLine{line(0, 1), line(1, 6), line(2, 3)}})
	if err != nil || r2.Order.Status != purchasingdomain.StatusReceived {
		t.Fatalf("second delivery: %v %v", r2.Order.Status, err)
	}

	// Assets landed in the catalog, with distinct generated codes, at the location.
	codes := e.codes(t, "Laptop")
	if len(codes) != 3 || codes[0] == codes[1] || codes[1] == codes[2] || codes[2] != "ISK-LAP-003" {
		t.Fatalf("laptop codes: %v", codes)
	}
	for _, rl := range r1.Receipt.Lines[:1] {
		for _, it := range rl.Items {
			a, err := e.catalog.GetItem(ctx, it.ItemID)
			if err != nil {
				t.Fatal(err)
			}
			if a.Kind != catalogdomain.KindAsset || a.Status != catalogdomain.StatusFunctional ||
				a.LocationID == nil || *a.LocationID != e.loc || a.Attributes["ram"] != "16GB" || a.Code != it.Code {
				t.Fatalf("created asset: %+v", a)
			}
		}
	}

	// Consumables show in stock at the receipt location, through `in` movements.
	paper, err := e.catalog.GetItem(ctx, existing.ID)
	if err != nil || paper.StockCurrent != 10 || len(paper.Stocks) != 1 || paper.Stocks[0].LocationID != e.loc {
		t.Fatalf("paper: %+v %v", paper, err)
	}
	if n := len(e.codes(t, "Tinta")); n != 1 {
		t.Fatalf("Tinta items: %d", n)
	}
	ink, err := e.catalog.GetItem(ctx, r2.Order.Lines[2].ItemID)
	if err != nil || ink.StockCurrent != 8 || ink.StockMin != 2 || ink.Code != "ISK-CON-002" {
		t.Fatalf("ink: %+v %v", ink, err)
	}
	var movements int
	var user, note string
	if err := e.pool.QueryRow(ctx,
		`SELECT count(*), max(user_id::text), max(note) FROM stock_movements WHERE type = 'in' AND item_id = $1::uuid`, existing.ID).Scan(&movements, &user, &note); err != nil {
		t.Fatal(err)
	}
	if movements != 2 || user != e.userID || note != "Purchase order #1: primer camión" {
		t.Fatalf("movements=%d user=%s note=%q", movements, user, note)
	}
	var assetMoves int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements m JOIN items i ON i.id = m.item_id WHERE i.kind = 'asset'`).Scan(&assetMoves); err != nil || assetMoves != 0 {
		t.Fatalf("assets recorded %d movements (%v)", assetMoves, err)
	}
}

func TestRealRollbackLeavesNoItemsAndNoStock(t *testing.T) {
	e, ctx := setupReal(t)
	svc := newPurchasingService(e.pool)
	doomed, err := e.catalog.CreateCategory(ctx, "Efímera", "EFI")
	if err != nil {
		t.Fatal(err)
	}
	o, err := svc.Create(ctx, e.userID, purchasingdomain.Input{
		SupplierID: e.supplier, Currency: purchasingdomain.USD,
		Lines: []purchasingdomain.LineInput{
			{NewItem: &purchasingdomain.NewItem{Description: "Laptop", CategoryID: e.lapCat, Kind: "asset"}, Quantity: 2, UnitCostMinor: 1},
			{NewItem: &purchasingdomain.NewItem{Description: "Cable", CategoryID: doomed.ID, Kind: "consumable", Unit: "u"}, Quantity: 5, UnitCostMinor: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if o, err = svc.Place(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	// The category disappears after the order was saved: the second line cannot
	// be created, after the first line's two assets already were.
	if err := e.catalog.DeleteCategory(ctx, doomed.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Receive(ctx, o.ID, e.userID, purchasingdomain.ReceiveRequest{
		LocationID: e.loc, Lines: []purchasingdomain.ReceiveLine{
			{LineID: o.Lines[0].ID, Quantity: 2}, {LineID: o.Lines[1].ID, Quantity: 5}}})
	if !errors.Is(err, purchasingdomain.ErrInvalidInput) {
		t.Fatalf("want a validation error, got %v", err)
	}
	var items, moves, receipts int
	if err := e.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM items), (SELECT count(*) FROM stock_movements), (SELECT count(*) FROM po_receipts)`).Scan(&items, &moves, &receipts); err != nil {
		t.Fatal(err)
	}
	if items != 0 || moves != 0 || receipts != 0 {
		t.Fatalf("items=%d movements=%d receipts=%d after rollback", items, moves, receipts)
	}
	got, _ := svc.Get(ctx, o.ID)
	if got.Status != purchasingdomain.StatusOrdered || got.Lines[0].ReceivedQty != 0 {
		t.Fatalf("order changed: %+v", got)
	}
}

func TestRealPortsReportSaveTimeErrors(t *testing.T) {
	e, ctx := setupReal(t)
	svc := newPurchasingService(e.pool)
	asset, err := e.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Proyector", CategoryID: e.lapCat, Kind: catalogdomain.KindAsset, LocationID: &e.loc}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(ctx, e.userID, purchasingdomain.Input{SupplierID: e.supplier, Currency: purchasingdomain.USD, Lines: []purchasingdomain.LineInput{
		{ItemID: asset.ID, Quantity: 1},
		{ItemID: "00000000-0000-0000-0000-000000000000", Quantity: 1},
		{NewItem: &purchasingdomain.NewItem{Description: "x", CategoryID: 9999, Kind: "asset"}, Quantity: 1},
		{NewItem: &purchasingdomain.NewItem{Description: "y", CategoryID: e.conCat, Kind: "consumable"}, Quantity: 1},
		{NewItem: &purchasingdomain.NewItem{Description: "z", CategoryID: e.lapCat, Kind: "asset", Unit: "u"}, Quantity: 1},
	}})
	var ve *purchasingdomain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	for _, f := range []string{"lines[0].item_id", "lines[1].item_id", "lines[2].new_item.category_id", "lines[3].new_item.unit", "lines[4].new_item.unit"} {
		if ve.Fields[f] == "" {
			t.Errorf("field %s not reported: %v", f, ve.Fields)
		}
	}
	if ve.Fields["lines[0].item_id"] != "existing_item_must_be_consumable" {
		t.Errorf("asset message: %q", ve.Fields["lines[0].item_id"])
	}
}
