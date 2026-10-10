package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/stock/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/stock/app"
	"github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/edgar/inventario/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func ptr[T any](v T) *T { return &v }

type fixture struct {
	pool    *pgxpool.Pool
	url     string
	stock   *app.Service
	catalog *catalogapp.Service
	userID  string
	catID   int64
	locA    int64
	locB    int64
}

func setup(t *testing.T) fixture {
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, code_sequences, locations, categories, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	f := fixture{pool: pool, url: url}
	if err := pool.QueryRow(ctx, `INSERT INTO users (name, email, password_hash, role) VALUES ('Ana Operator', 'ana@example.com', 'x', 'operator') RETURNING id::text`).Scan(&f.userID); err != nil {
		t.Fatal(err)
	}
	cat := catalogpg.New(pool, catalogpg.WithCreateHook(openingHook))
	f.catalog = catalogapp.NewService(cat.Categories(), cat.Locations(), cat.Items())
	f.stock = app.NewService(postgres.New(pool))
	c, err := f.catalog.CreateCategory(ctx, "Consumibles", "CON")
	if err != nil {
		t.Fatal(err)
	}
	f.catID = c.ID
	for name, dst := range map[string]*int64{"Bodega": &f.locA, "Taller": &f.locB} {
		l, err := f.catalog.CreateLocation(ctx, name, catalogdomain.LocationStorage, nil)
		if err != nil {
			t.Fatal(err)
		}
		*dst = l.ID
	}
	return f
}

// openingHook mirrors cmd/api wiring: consumables created with stock get an
// opening movement in the same transaction.
func openingHook(ctx context.Context, tx pgx.Tx, it catalogdomain.Item) error {
	if it.Kind != catalogdomain.KindConsumable || it.StockCurrent <= 0 {
		return nil
	}
	uid := ""
	if p, ok := httpx.PrincipalFrom(ctx); ok {
		uid = p.UserID
	}
	return postgres.RecordOpening(ctx, tx, it.ID, it.StockCurrent, it.LocationID, uid)
}

func (f fixture) consumable(t *testing.T, desc string, stock, min int, loc *int64) catalogdomain.Item {
	t.Helper()
	ctx := httpx.WithPrincipal(t.Context(), httpx.Principal{UserID: f.userID})
	it, err := f.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: desc, CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"),
		StockCurrent: &stock, StockMin: min, LocationID: loc}})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func (f fixture) asset(t *testing.T, desc string, loc *int64) catalogdomain.Item {
	t.Helper()
	it, err := f.catalog.CreateItem(t.Context(), catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: desc, CategoryID: f.catID, Kind: catalogdomain.KindAsset, LocationID: loc}})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func (f fixture) stockOf(t *testing.T, id string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT stock_current FROM items WHERE id = $1::uuid`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateWithStockRecordsOpeningMovementAtomically(t *testing.T) {
	f := setup(t)
	it := f.consumable(t, "Guantes", 5, 2, &f.locA)
	ms, total, _, err := f.stock.History(t.Context(), it.ID, "", 25, 0)
	if err != nil || total != 1 {
		t.Fatalf("history: %+v total=%d err=%v", ms, total, err)
	}
	m := ms[0]
	if m.Type != domain.TypeOpening || m.Qty != 5 || m.StockBefore != 0 || m.StockAfter != 5 ||
		m.ToLocationID == nil || *m.ToLocationID != f.locA || m.UserName == nil || *m.UserName != "Ana Operator" {
		t.Errorf("opening movement: %+v", m)
	}

	// No stock, or an asset: no movement.
	zero := f.consumable(t, "Vacío", 0, 0, nil)
	asset := f.asset(t, "Taladro", &f.locA)
	for _, id := range []string{zero.ID, asset.ID} {
		if _, total, _, _ := f.stock.History(t.Context(), id, "", 25, 0); total != 0 {
			t.Errorf("item %s has %d movements, want 0", id, total)
		}
	}
}

func TestRecordKeepsStockAndLedgerInStep(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.consumable(t, "Guantes", 5, 2, &f.locA)

	steps := []struct {
		req   domain.Request
		stock int
	}{
		{domain.Request{Type: domain.TypeOut, Qty: 2}, 3},
		{domain.Request{Type: domain.TypeAdjust, Qty: 8, Note: "recount"}, 8},
		{domain.Request{Type: domain.TypeIn, Qty: 1}, 9},
		{domain.Request{Type: domain.TypeTransfer, ToLocationID: &f.locB}, 9},
	}
	for _, s := range steps {
		r, err := f.stock.Record(ctx, it.ID, f.userID, s.req)
		if err != nil {
			t.Fatalf("%s: %v", s.req.Type, err)
		}
		if r.StockCurrent != s.stock || f.stockOf(t, it.ID) != s.stock {
			t.Errorf("%s: stock result=%d db=%d, want %d", s.req.Type, r.StockCurrent, f.stockOf(t, it.ID), s.stock)
		}
	}
	var loc int64
	if err := f.pool.QueryRow(ctx, `SELECT location_id FROM items WHERE id = $1::uuid`, it.ID).Scan(&loc); err != nil || loc != f.locB {
		t.Errorf("item location = %d (%v), want %d", loc, err, f.locB)
	}

	ms, total, _, err := f.stock.History(ctx, it.ID, "", 25, 0)
	if err != nil || total != 5 {
		t.Fatalf("history total=%d err=%v", total, err)
	}
	if ms[0].Type != domain.TypeTransfer || *ms[0].FromLocationID != f.locA || *ms[0].ToLocationID != f.locB || ms[0].Qty != 0 {
		t.Errorf("newest = %+v", ms[0])
	}
	if ms[1].Type != domain.TypeIn || ms[2].Type != domain.TypeAdjust || ms[2].Qty != 5 || ms[3].Type != domain.TypeOut || ms[4].Type != domain.TypeOpening {
		t.Errorf("order/deltas wrong: %+v", ms)
	}
	// Filter and paging.
	if got, total, _, _ := f.stock.History(ctx, it.ID, domain.TypeOut, 25, 0); total != 1 || len(got) != 1 {
		t.Errorf("type filter: %d/%d", len(got), total)
	}
	if got, _, _, _ := f.stock.History(ctx, it.ID, "", 2, 3); len(got) != 2 || got[0].Type != domain.TypeOut {
		t.Errorf("offset page: %+v", got)
	}
}

func TestRecordRejectionsAndDeletedItems(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.consumable(t, "Guantes", 3, 0, nil)
	asset := f.asset(t, "Taladro", &f.locA)

	if _, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 4}); !errors.Is(err, domain.ErrInsufficientStock) {
		t.Errorf("oversell: %v", err)
	}
	if _, err := f.stock.Record(ctx, asset.ID, f.userID, domain.Request{Type: domain.TypeIn, Qty: 1}); !errors.Is(err, domain.ErrNotApplicable) {
		t.Errorf("asset in: %v", err)
	}
	if _, err := f.stock.Record(ctx, asset.ID, f.userID, domain.Request{Type: domain.TypeTransfer, ToLocationID: ptr(int64(9999))}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("dangling location: %v", err)
	}
	if r, err := f.stock.Record(ctx, asset.ID, f.userID, domain.Request{Type: domain.TypeTransfer, ToLocationID: &f.locB}); err != nil || r.StockCurrent != 1 {
		t.Errorf("asset transfer: %+v %v", r, err)
	}
	if _, err := f.stock.Record(ctx, "not-a-uuid", f.userID, domain.Request{Type: domain.TypeIn, Qty: 1}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("bad id: %v", err)
	}
	if err := f.catalog.DeleteItem(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeIn, Qty: 1}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("deleted item movement: %v", err)
	}
	if _, _, _, err := f.stock.History(ctx, it.ID, "", 25, 0); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("deleted item history: %v", err)
	}
	if got := f.stockOf(t, it.ID); got != 3 {
		t.Errorf("rejected movements changed stock to %d", got)
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE item_id = $1::uuid`, it.ID).Scan(&n)
	if n != 1 { // only the opening movement
		t.Errorf("movements for rejected item = %d, want 1", n)
	}
}

func TestMovementsAreImmutable(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.consumable(t, "Guantes", 5, 0, nil)
	if _, err := f.pool.Exec(ctx, `UPDATE stock_movements SET qty = 99 WHERE item_id = $1::uuid`, it.ID); err == nil {
		t.Error("UPDATE accepted")
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM stock_movements WHERE item_id = $1::uuid`, it.ID); err == nil {
		t.Error("DELETE accepted")
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE item_id = $1::uuid AND qty = 5`, it.ID).Scan(&n); err != nil || n != 1 {
		t.Errorf("history changed: n=%d err=%v", n, err)
	}
	// The table constraints also refuse rows that do not balance.
	if _, err := f.pool.Exec(ctx, `INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after) VALUES ($1::uuid, 'in', 2, 5, 9)`, it.ID); err == nil {
		t.Error("unbalanced movement accepted")
	}
}

func TestConcurrentOutNeverOversellsAndLedgerReconciles(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.consumable(t, "Guantes", 20, 0, nil)

	const workers = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	okCount, insufficient := 0, 0
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 1})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				okCount++
			case errors.Is(err, domain.ErrInsufficientStock):
				insufficient++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if okCount != 20 || insufficient != workers-20 {
		t.Errorf("ok=%d insufficient=%d, want 20/%d", okCount, insufficient, workers-20)
	}
	if got := f.stockOf(t, it.ID); got != 0 {
		t.Errorf("final stock = %d, want 0", got)
	}
	var sum int
	if err := f.pool.QueryRow(ctx, `SELECT COALESCE(sum(qty), 0) FROM stock_movements WHERE item_id = $1::uuid`, it.ID).Scan(&sum); err != nil || sum != 0 {
		t.Errorf("sum(movements) = %d (%v), want 0 (opening 20 minus 20 outs)", sum, err)
	}
	// Each movement chains onto the previous one: no gaps, no overlaps.
	rows, err := f.pool.Query(ctx, `SELECT stock_before, stock_after FROM stock_movements WHERE item_id = $1::uuid ORDER BY id`, it.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	prev := 0
	for rows.Next() {
		var before, after int
		if err := rows.Scan(&before, &after); err != nil {
			t.Fatal(err)
		}
		if before != prev {
			t.Fatalf("ledger gap: before=%d, previous after=%d", before, prev)
		}
		prev = after
	}
}

func TestAlertsOrderedBySeverityAndPaged(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	f.consumable(t, "Cinta", 0, 5, &f.locA) // shortage -5
	f.consumable(t, "Pegamento", 2, 3, nil) // -1
	f.consumable(t, "Tornillos", 4, 4, &f.locB)
	f.consumable(t, "Sobrado", 9, 3, nil)    // above minimum
	f.consumable(t, "Sin mínimo", 0, 0, nil) // no minimum configured
	f.asset(t, "Taladro", nil)
	gone := f.consumable(t, "Borrado", 0, 9, nil)
	if err := f.catalog.DeleteItem(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	as, total, _, err := f.stock.Alerts(ctx, 25, 0)
	if err != nil || total != 3 || len(as) != 3 {
		t.Fatalf("alerts: %+v total=%d err=%v", as, total, err)
	}
	for i, want := range []string{"Cinta", "Pegamento", "Tornillos"} {
		if as[i].Description != want {
			t.Errorf("alerts[%d] = %s, want %s", i, as[i].Description, want)
		}
	}
	if as[0].LocationName == nil || *as[0].LocationName != "Bodega" || as[1].LocationName != nil || as[0].Unit != "u" {
		t.Errorf("alert fields: %+v / %+v", as[0], as[1])
	}
	// Taking stock out moves an item up the list.
	if _, err := f.stock.Record(ctx, as[2].ItemID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 4}); err != nil {
		t.Fatal(err)
	}
	as, _, _, _ = f.stock.Alerts(ctx, 1, 0)
	if len(as) != 1 || as[0].Description != "Cinta" {
		t.Errorf("page 1: %+v", as)
	}
	as, _, _, _ = f.stock.Alerts(ctx, 1, 1)
	if len(as) != 1 || as[0].Description != "Tornillos" { // now -4
		t.Errorf("page 2: %+v", as)
	}
}

// TestMigrationBackfillReconcilesExistingConsumables rolls migration 00004
// back, creates consumables the way pre-stock versions did, re-applies it,
// and checks the history of every consumable ends at items.stock_current.
func TestMigrationBackfillReconcilesExistingConsumables(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	sqlDB, err := sql.Open("pgx", f.url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = provider.Up(context.Background()) }) // leave the schema migrated even on failure
	if _, err := provider.DownTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	legacy := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO items (code, description, category_id, kind, unit, stock_current, location_id) VALUES ('ISK-CON-101', 'Con stock', $1, 'consumable', 'u', 7, $2)`, []any{f.catID, f.locA}},
		{`INSERT INTO items (code, description, category_id, kind, unit, stock_current) VALUES ('ISK-CON-102', 'Sin stock', $1, 'consumable', 'u', 0)`, []any{f.catID}},
		{`INSERT INTO items (code, description, category_id, kind) VALUES ('ISK-CON-103', 'Activo', $1, 'asset')`, []any{f.catID}},
	}
	for _, l := range legacy {
		if _, err := f.pool.Exec(ctx, l.q, l.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var bad int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM items i
		WHERE i.kind = 'consumable'
		  AND COALESCE((SELECT m.stock_after FROM stock_movements m WHERE m.item_id = i.id ORDER BY m.created_at DESC, m.id DESC LIMIT 1), 0) <> i.stock_current`).
		Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("consumables whose latest stock_after differs from stock_current: %d (%v)", bad, err)
	}
	var total int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements`).Scan(&total)
	if total != 1 {
		t.Errorf("backfilled %d movements, want 1 (only the consumable with stock)", total)
	}
}

func TestTransferRejectedWhileOnLoanThenAllowedAfterReturn(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	asset := f.asset(t, "Taladro", &f.locA)
	var assignmentID int64
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO custody_assignments (item_id, assignee_location_id, origin_location_id, assigned_by)
		VALUES ($1::uuid, $2, $3, $4::uuid) RETURNING id`, asset.ID, f.locB, f.locA, f.userID).Scan(&assignmentID); err != nil {
		t.Fatal(err)
	}
	req := domain.Request{Type: domain.TypeTransfer, ToLocationID: &f.locB}
	if _, err := f.stock.Record(ctx, asset.ID, f.userID, req); !errors.Is(err, domain.ErrItemOnLoan) {
		t.Fatalf("transfer on loan: %v", err)
	}
	var loc int64
	if err := f.pool.QueryRow(ctx, `SELECT location_id FROM items WHERE id = $1::uuid`, asset.ID).Scan(&loc); err != nil || loc != f.locA {
		t.Errorf("rejected transfer moved the asset: loc=%d err=%v", loc, err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE custody_assignments SET returned_at = now(), returned_by = $2::uuid WHERE id = $1`, assignmentID, f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.stock.Record(ctx, asset.ID, f.userID, req); err != nil {
		t.Errorf("transfer after return: %v", err)
	}
}
