package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/edgar/inventario/backend/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

// assertInvariants checks, for the whole database, the two promises of stock
// per location: items.stock_current equals the sum of the item's rows, and
// replaying the ledger per location (in/out/adjust/opening at location_id,
// transfers as -moved_qty at from and +moved_qty at to) yields exactly the
// stored quantities. It also checks every item's history chains.
func assertInvariants(t *testing.T, f fixture) {
	t.Helper()
	ctx := t.Context()
	var bad int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM items i
		WHERE i.kind = 'consumable'
		  AND i.stock_current <> COALESCE((SELECT sum(quantity) FROM item_stocks s WHERE s.item_id = i.id), 0)`).Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("items whose stock_current differs from sum(item_stocks): %d (%v)", bad, err)
	}
	if err := f.pool.QueryRow(ctx, `
		WITH deltas AS (
			SELECT m.item_id, m.location_id AS loc, m.qty AS d FROM stock_movements m
			JOIN items i ON i.id = m.item_id AND i.kind = 'consumable'
			WHERE m.type IN ('opening', 'in', 'out', 'adjust')
			UNION ALL
			SELECT item_id, from_location_id, -moved_qty FROM stock_movements WHERE type = 'transfer' AND moved_qty > 0
			UNION ALL
			SELECT item_id, to_location_id, moved_qty FROM stock_movements WHERE type = 'transfer' AND moved_qty > 0
		), ledger AS (
			SELECT item_id, loc, sum(d) AS q FROM deltas GROUP BY item_id, loc HAVING sum(d) <> 0
		)
		SELECT count(*) FROM ledger l
		FULL JOIN item_stocks s ON s.item_id = l.item_id AND s.location_id = l.loc
		WHERE COALESCE(l.q, 0) <> COALESCE(s.quantity, 0)`).Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("(item, location) pairs where the ledger disagrees with item_stocks: %d (%v)", bad, err)
	}
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT stock_before, lag(stock_after) OVER (PARTITION BY item_id ORDER BY id) AS prev
			FROM stock_movements) x
		WHERE prev IS NOT NULL AND stock_before <> prev`).Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("movements that do not chain onto the previous one: %d (%v)", bad, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM item_stocks WHERE quantity = 0`).Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("rows left at quantity 0: %d (%v)", bad, err)
	}
}

func (f fixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (f fixture) location(t *testing.T, name string) int64 {
	t.Helper()
	l, err := f.catalog.CreateLocation(t.Context(), name, catalogdomain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	return l.ID
}

func TestMultiLocationOpeningBalanceWritesOneMovementPerRow(t *testing.T) {
	f := setup(t)
	ctx := httpx.WithPrincipal(t.Context(), httpx.Principal{UserID: f.userID})
	it, err := f.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Tornillos", CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"), StockMin: 3,
		Stocks: []catalogdomain.StockInput{{LocationID: f.locA, Quantity: 5}, {LocationID: f.locB, Quantity: 5}}}})
	if err != nil {
		t.Fatal(err)
	}
	if it.StockCurrent != 10 || it.LocationID != nil || len(it.Stocks) != 2 || it.Stocks[0].LocationName != "Bodega" {
		t.Fatalf("created item: %+v", it)
	}
	if got := f.stocksOf(t, it.ID); got[f.locA] != 5 || got[f.locB] != 5 || len(got) != 2 {
		t.Errorf("rows: %v", got)
	}
	ms, total, _, err := f.stock.History(t.Context(), it.ID, "", 25, 0)
	if err != nil || total != 2 || ms[0].Type != domain.TypeOpening || ms[0].StockBefore != 5 || ms[0].StockAfter != 10 || ms[1].StockBefore != 0 {
		t.Fatalf("history: %+v total=%d err=%v", ms, total, err)
	}
	assertInvariants(t, f)

	// The legacy form with no location lands in "Sin ubicación".
	legacy := f.consumable(t, "Cinta", 4, 0, nil)
	if len(legacy.Stocks) != 1 || legacy.Stocks[0].LocationName != "Sin ubicación" || legacy.StockCurrent != 4 {
		t.Errorf("legacy create: %+v", legacy)
	}
	assertInvariants(t, f)

	// A dangling location in the new form rolls the whole creation back.
	var n int
	_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM items`).Scan(&n)
	_, err = f.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Roto", CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"),
		Stocks: []catalogdomain.StockInput{{LocationID: f.locA, Quantity: 1}, {LocationID: 9999, Quantity: 1}}}})
	if !errors.Is(err, catalogdomain.ErrInvalidInput) {
		t.Fatalf("dangling location: %v", err)
	}
	var after int
	_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM items`).Scan(&after)
	if after != n {
		t.Errorf("failed creation left %d items behind", after-n)
	}
}

func TestRecordPerLocationEndToEnd(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	locC := f.location(t, "Maleta")
	it, err := f.catalog.CreateItem(httpx.WithPrincipal(ctx, httpx.Principal{UserID: f.userID}), catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Tornillos", CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"), StockMin: 4,
		Stocks: []catalogdomain.StockInput{{LocationID: f.locA, Quantity: 5}, {LocationID: f.locB, Quantity: 5}}}})
	if err != nil {
		t.Fatal(err)
	}
	// Ambiguous without a location.
	if _, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 1}); !errors.Is(err, domain.ErrLocationRequired) {
		t.Fatalf("ambiguous: %v", err)
	}
	// Out never goes below zero at a location, even though the total covers it.
	_, err = f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 6, LocationID: &f.locA})
	var ie *domain.InsufficientStockError
	if !errors.As(err, &ie) || ie.Available != 5 || ie.LocationName != "Bodega" {
		t.Fatalf("over-out: %v", err)
	}
	// Drain a location: its row disappears.
	r, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 5, LocationID: &f.locA})
	if err != nil || r.StockCurrent != 5 || len(r.Stocks) != 1 || r.Stocks[0].LocationID != f.locB {
		t.Fatalf("drain: %+v %v", r, err)
	}
	// Now there is exactly one row: legacy requests infer it.
	if r, err = f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeIn, Qty: 2}); err != nil || *r.Movement.LocationID != f.locB {
		t.Fatalf("inferred in: %+v %v", r, err)
	}
	// Partial transfer to a third location, then adjust it.
	if r, err = f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeTransfer, Qty: 3, ToLocationID: &locC}); err != nil || r.StockCurrent != 7 {
		t.Fatalf("transfer: %+v %v", r, err)
	}
	if r, err = f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeAdjust, Qty: 1, LocationID: &locC, Note: "recount"}); err != nil || r.StockCurrent != 5 {
		t.Fatalf("adjust: %+v %v", r, err)
	}
	if got := f.stocksOf(t, it.ID); len(got) != 2 || got[f.locB] != 4 || got[locC] != 1 {
		t.Errorf("rows: %v", got)
	}
	// Unknown location is a field error, not a 500.
	if _, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeIn, Qty: 1, LocationID: ptr(int64(9999))}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("unknown location: %v", err)
	}
	// Totals, alerts and the ledger all agree.
	as, _, _, err := f.stock.Alerts(ctx, 25, 0)
	if err != nil || len(as) != 0 {
		t.Errorf("alerts at stock 5 > min 4: %+v %v", as, err)
	}
	if _, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 1, LocationID: &locC}); err != nil {
		t.Fatal(err)
	}
	as, _, _, _ = f.stock.Alerts(ctx, 25, 0)
	if len(as) != 1 || as[0].StockCurrent != 4 || len(as[0].Stocks) != 1 || as[0].Stocks[0].LocationName != "Taller" {
		t.Errorf("alert breakdown: %+v", as)
	}
	assertInvariants(t, f)
}

func TestDatabaseRejectsTotalsThatDisagreeWithTheRows(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.consumable(t, "Guantes", 5, 0, &f.locA)
	asset := f.asset(t, "Taladro", &f.locA)

	commitFails := func(name string, stmts ...string) {
		t.Helper()
		tx, err := f.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		for _, s := range stmts {
			if _, err := tx.Exec(ctx, s); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		err = tx.Commit(ctx)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: commit err = %v, want check_violation", name, err)
		}
	}
	commitFails("total without rows", fmt.Sprintf(`UPDATE items SET stock_current = 9 WHERE id = '%s'`, it.ID))
	commitFails("row without total", fmt.Sprintf(`UPDATE item_stocks SET quantity = 9 WHERE item_id = '%s'`, it.ID))
	commitFails("row on an asset", fmt.Sprintf(`INSERT INTO item_stocks (item_id, location_id, quantity) VALUES ('%s', %d, 1)`, asset.ID, f.locA))
	commitFails("deleted row", fmt.Sprintf(`DELETE FROM item_stocks WHERE item_id = '%s'`, it.ID))

	// Changing both sides inside one transaction is fine (checked at commit).
	f.exec(t, fmt.Sprintf(`WITH a AS (UPDATE item_stocks SET quantity = 9 WHERE item_id = '%s' RETURNING 1)
		UPDATE items SET stock_current = 9 WHERE id = '%s'`, it.ID, it.ID))
	if _, err := f.pool.Exec(ctx, `INSERT INTO item_stocks (item_id, location_id, quantity) VALUES ($1::uuid, $2, -1)`, it.ID, f.locB); err == nil {
		t.Error("negative quantity accepted")
	}
}

func TestLocationDeleteGuard(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	unused := f.location(t, "Libre")
	withStock := f.location(t, "Con stock")
	it := f.consumable(t, "Guantes", 5, 0, &withStock)

	if err := f.catalog.DeleteLocation(ctx, withStock); !errors.Is(err, catalogdomain.ErrLocationInUse) {
		t.Errorf("location holding stock: %v", err)
	}
	// Even after the stock leaves, the history keeps the location alive.
	if _, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeTransfer, Qty: 5, FromLocationID: &withStock, ToLocationID: &f.locA}); err != nil {
		t.Fatal(err)
	}
	if err := f.catalog.DeleteLocation(ctx, withStock); !errors.Is(err, catalogdomain.ErrLocationInUse) {
		t.Errorf("location with history: %v", err)
	}
	if err := f.catalog.DeleteLocation(ctx, f.locA); !errors.Is(err, catalogdomain.ErrLocationInUse) {
		t.Errorf("location holding the moved stock: %v", err)
	}
	if err := f.catalog.DeleteLocation(ctx, unused); err != nil {
		t.Errorf("unused location: %v", err)
	}
	assertInvariants(t, f)
}

func TestMovementsStayImmutableWithLocationColumns(t *testing.T) {
	f := setup(t)
	it := f.consumable(t, "Guantes", 5, 0, &f.locA)
	for _, q := range []string{
		`UPDATE stock_movements SET location_id = NULL WHERE item_id = $1::uuid`,
		`UPDATE stock_movements SET moved_qty = 3 WHERE item_id = $1::uuid`,
		`DELETE FROM stock_movements WHERE item_id = $1::uuid`,
	} {
		if _, err := f.pool.Exec(t.Context(), q, it.ID); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}
	// New CHECKs: a transfer cannot carry a location_id, only transfers move quantity.
	for _, q := range []string{
		`INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, from_location_id, to_location_id, location_id, moved_qty)
			VALUES ($1::uuid, 'transfer', 0, 5, 5, $2, $3, $2, 1)`,
		`INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, location_id, moved_qty)
			VALUES ($1::uuid, 'in', 1, 5, 6, $2, 1)`,
	} {
		if _, err := f.pool.Exec(t.Context(), q, it.ID, f.locA, f.locB); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}
	assertInvariants(t, f)
}

func TestConcurrentOutAtOneLocationNeverOversellsAnother(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it, err := f.catalog.CreateItem(httpx.WithPrincipal(ctx, httpx.Principal{UserID: f.userID}), catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Tornillos", CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"),
		Stocks: []catalogdomain.StockInput{{LocationID: f.locA, Quantity: 20}, {LocationID: f.locB, Quantity: 30}}}})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, insufficient := 0, 0
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeOut, Qty: 1, LocationID: &f.locA})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrInsufficientStock):
				insufficient++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 20 || insufficient != workers-20 {
		t.Errorf("ok=%d insufficient=%d, want 20/%d", ok, insufficient, workers-20)
	}
	if got := f.stocksOf(t, it.ID); len(got) != 1 || got[f.locB] != 30 || f.stockOf(t, it.ID) != 30 {
		t.Errorf("rows=%v total=%d, want only B:30", got, f.stockOf(t, it.ID))
	}
	assertInvariants(t, f)
}

func TestOppositeParallelTransfersConserveTheTotalAndDoNotDeadlock(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	it, err := f.catalog.CreateItem(httpx.WithPrincipal(ctx, httpx.Principal{UserID: f.userID}), catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Tornillos", CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"),
		Stocks: []catalogdomain.StockInput{{LocationID: f.locA, Quantity: 10}, {LocationID: f.locB, Quantity: 10}}}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			from, to := f.locA, f.locB
			if i%2 == 1 {
				from, to = to, from
			}
			if _, err := f.stock.Record(ctx, it.ID, f.userID, domain.Request{Type: domain.TypeTransfer, Qty: 1 + i%3, FromLocationID: &from, ToLocationID: &to}); err != nil &&
				!errors.Is(err, domain.ErrInsufficientStock) {
				t.Errorf("transfer %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		t.Fatal("transfers did not finish in time (deadlock?)")
	}
	if f.stockOf(t, it.ID) != 20 {
		t.Errorf("total = %d, want 20", f.stockOf(t, it.ID))
	}
	assertInvariants(t, f)
}

// TestRandomMixKeepsEveryInvariant hammers several items from several
// goroutines with a seeded random mix of in/out/adjust/transfer (with and
// without an explicit location) and checks the invariants afterwards.
func TestRandomMixKeepsEveryInvariant(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	locC := f.location(t, "Maleta")
	locs := []int64{f.locA, f.locB, locC}
	var items []string
	for i := range 3 {
		it, err := f.catalog.CreateItem(httpx.WithPrincipal(ctx, httpx.Principal{UserID: f.userID}), catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
			Description: fmt.Sprintf("Item %d", i), CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"),
			Stocks: []catalogdomain.StockInput{{LocationID: locs[i], Quantity: 12}, {LocationID: locs[(i+1)%3], Quantity: 7}}}})
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, it.ID)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := map[domain.MovementType]int{}
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(w) + 1))
			for range 40 {
				id := items[rng.Intn(len(items))]
				pick := func() *int64 {
					if rng.Intn(5) == 0 {
						return nil // exercise the inference / location_required path
					}
					l := locs[rng.Intn(len(locs))]
					return &l
				}
				req := domain.Request{Qty: 1 + rng.Intn(6), LocationID: pick()}
				switch rng.Intn(4) {
				case 0:
					req.Type = domain.TypeIn
				case 1:
					req.Type = domain.TypeOut
				case 2:
					req.Type, req.Qty, req.Note = domain.TypeAdjust, rng.Intn(15), "recount"
				default:
					req.Type, req.LocationID = domain.TypeTransfer, nil
					req.FromLocationID, req.ToLocationID = pick(), pick()
				}
				_, err := f.stock.Record(ctx, id, f.userID, req)
				switch {
				case err == nil:
					mu.Lock()
					accepted[req.Type]++
					mu.Unlock()
				case errors.Is(err, domain.ErrInsufficientStock), errors.Is(err, domain.ErrLocationRequired), errors.Is(err, domain.ErrInvalidInput):
				default:
					t.Errorf("%s: unexpected error: %v", req.Type, err)
				}
			}
		}()
	}
	wg.Wait()
	for _, typ := range []domain.MovementType{domain.TypeIn, domain.TypeOut, domain.TypeAdjust, domain.TypeTransfer} {
		if accepted[typ] == 0 {
			t.Errorf("the mix never exercised an accepted %s: %v", typ, accepted)
		}
	}
	assertInvariants(t, f)
}

// migrationRig rolls the schema back to migration 6 so a test can create
// data the way the pre-per-location versions did, then re-applies 00007.
type migrationRig struct {
	f        fixture
	provider *goose.Provider
	sqlDB    *sql.DB
}

func newMigrationRig(t *testing.T) migrationRig {
	t.Helper()
	f := setup(t)
	sqlDB, err := sql.Open("pgx", f.url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = provider.Up(context.Background()) }) // leave the schema migrated even on failure
	if _, err := provider.DownTo(t.Context(), 6); err != nil {
		t.Fatal(err)
	}
	return migrationRig{f: f, provider: provider, sqlDB: sqlDB}
}

// legacyItem inserts a consumable as migration 6 stored it and returns its id.
func (r migrationRig) legacyItem(t *testing.T, code string, stock int, loc *int64) string {
	t.Helper()
	var id string
	if err := r.f.pool.QueryRow(t.Context(), `
		INSERT INTO items (code, description, category_id, kind, unit, stock_current, location_id)
		VALUES ($1, $1, $2, 'consumable', 'u', $3, $4) RETURNING id::text`, code, r.f.catID, stock, loc).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// legacyMove inserts a movement row of the 6-era shape.
func (r migrationRig) legacyMove(t *testing.T, item, typ string, qty, before int, from, to *int64) {
	t.Helper()
	r.f.exec(t, `INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, from_location_id, to_location_id, note)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, 'legacy adjust')`, item, typ, qty, before, before+qty, from, to)
}

func TestMigration00007BackfillsFromTheSingleLocationSchema(t *testing.T) {
	r := newMigrationRig(t)
	f := r.f
	// A: opened at Bodega, one out. B: stocked with no location at all.
	// C: no stock. D: opened at Bodega, moved to Taller, then received more.
	// E: no location, later moved to Taller (transfer from nowhere).
	a := r.legacyItem(t, "ISK-CON-001", 5, &f.locA)
	r.legacyMove(t, a, "opening", 7, 0, nil, &f.locA)
	r.legacyMove(t, a, "out", -2, 7, nil, nil)
	b := r.legacyItem(t, "ISK-CON-002", 4, nil)
	r.legacyMove(t, b, "opening", 4, 0, nil, nil)
	c := r.legacyItem(t, "ISK-CON-003", 0, &f.locA)
	d := r.legacyItem(t, "ISK-CON-004", 15, &f.locB)
	r.legacyMove(t, d, "opening", 10, 0, nil, &f.locA)
	r.legacyMove(t, d, "transfer", 0, 10, &f.locA, &f.locB)
	r.legacyMove(t, d, "in", 5, 10, nil, nil)
	e := r.legacyItem(t, "ISK-CON-005", 6, &f.locB)
	r.legacyMove(t, e, "opening", 6, 0, nil, nil)
	r.legacyMove(t, e, "transfer", 0, 6, nil, &f.locB)
	var asset string
	if err := f.pool.QueryRow(t.Context(), `INSERT INTO items (code, description, category_id, kind, location_id)
		VALUES ('ISK-HER-001', 'Taladro', $1, 'asset', $2) RETURNING id::text`, f.catID, f.locA).Scan(&asset); err != nil {
		t.Fatal(err)
	}

	if _, err := r.provider.Up(t.Context()); err != nil {
		t.Fatal(err)
	}

	var sin int64
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM locations WHERE name = 'Sin ubicación' AND kind = 'storage'`).Scan(&sin); err != nil {
		t.Fatalf("the default location was not created: %v", err)
	}
	want := map[string]map[int64]int{
		a: {f.locA: 5}, b: {sin: 4}, c: {}, d: {f.locB: 15}, e: {f.locB: 6}, asset: {},
	}
	for id, w := range want {
		got := f.stocksOf(t, id)
		if len(got) != len(w) {
			t.Errorf("item %s rows = %v, want %v", id, got, w)
			continue
		}
		for loc, q := range w {
			if got[loc] != q {
				t.Errorf("item %s rows = %v, want %v", id, got, w)
			}
		}
	}
	var nonNull int
	_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM items WHERE kind = 'consumable' AND location_id IS NOT NULL`).Scan(&nonNull)
	if nonNull != 0 {
		t.Errorf("%d consumables still carry items.location_id", nonNull)
	}
	var assetLoc int64
	if err := f.pool.QueryRow(t.Context(), `SELECT location_id FROM items WHERE id = $1::uuid`, asset).Scan(&assetLoc); err != nil || assetLoc != f.locA {
		t.Errorf("asset location = %d (%v), want it untouched", assetLoc, err)
	}
	var moved int
	if err := f.pool.QueryRow(t.Context(), `SELECT moved_qty FROM stock_movements WHERE item_id = $1::uuid AND type = 'transfer'`, d).Scan(&moved); err != nil || moved != 10 {
		t.Errorf("legacy transfer moved_qty = %d (%v), want the whole 10", moved, err)
	}
	assertInvariants(t, f)

	// The backfilled data is fully usable by the new rules.
	ctx := t.Context()
	if _, err := f.stock.Record(ctx, d, f.userID, domain.Request{Type: domain.TypeOut, Qty: 3}); err != nil {
		t.Errorf("out on a backfilled item (location inferred): %v", err)
	}
	if _, err := f.stock.Record(ctx, b, f.userID, domain.Request{Type: domain.TypeTransfer, Qty: 1, ToLocationID: &f.locA}); err != nil {
		t.Errorf("transfer out of the default location: %v", err)
	}
	assertInvariants(t, f)
	// The ledger is still append-only after the backfill.
	if _, err := f.pool.Exec(ctx, `UPDATE stock_movements SET note = 'x'`); err == nil {
		t.Error("UPDATE accepted after migration")
	}
}

func TestMigration00007CreatesNoDefaultLocationWhenNothingNeedsIt(t *testing.T) {
	r := newMigrationRig(t)
	f := r.f
	a := r.legacyItem(t, "ISK-CON-001", 5, &f.locA)
	r.legacyMove(t, a, "opening", 5, 0, nil, &f.locA)
	r.legacyItem(t, "ISK-CON-002", 0, nil) // no stock, no history, no location
	if _, err := r.provider.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM locations WHERE name = 'Sin ubicación'`).Scan(&n)
	if n != 0 {
		t.Errorf("created %d default locations that nothing needed", n)
	}
	assertInvariants(t, f)
}

func TestMigration00007RollsBackToASingleLocation(t *testing.T) {
	f := setup(t)
	locC := f.location(t, "Maleta")
	it, err := f.catalog.CreateItem(httpx.WithPrincipal(t.Context(), httpx.Principal{UserID: f.userID}), catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Tornillos", CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: ptr("u"),
		Stocks: []catalogdomain.StockInput{{LocationID: f.locA, Quantity: 3}, {LocationID: locC, Quantity: 9}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.stock.Record(t.Context(), it.ID, f.userID, domain.Request{Type: domain.TypeTransfer, Qty: 1, FromLocationID: &f.locA, ToLocationID: &f.locB}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := sql.Open("pgx", f.url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = provider.Up(context.Background()) })
	// The relaxed opening shape cannot be restored over multi-location
	// openings, so the Down refuses instead of corrupting the ledger.
	if _, err := provider.DownTo(t.Context(), 6); err == nil {
		t.Fatal("down migration accepted a ledger that the old shape cannot hold")
	}
}
