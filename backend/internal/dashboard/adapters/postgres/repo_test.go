package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/dashboard/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/dashboard/app"
	"github.com/edgar/inventario/backend/internal/dashboard/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func days(n int) time.Time { return now.AddDate(0, 0, n) }

type seed struct {
	t    *testing.T
	pool *pgxpool.Pool
	cat  int64
	user string
	seq  int
}

func newSeed(t *testing.T) *seed {
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, code_sequences, locations, categories, users, suppliers, purchase_orders RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	s := &seed{t: t, pool: pool}
	s.must(`INSERT INTO categories (name, code_prefix) VALUES ('Tools', 'TOO')`)
	s.cat = 1
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (name, email, password_hash, role) VALUES ('Ana', 'ana@example.com', 'x', 'operator') RETURNING id::text`).Scan(&s.user); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *seed) must(sql string, args ...any) {
	s.t.Helper()
	if _, err := s.pool.Exec(context.Background(), sql, args...); err != nil {
		s.t.Fatalf("%s: %v", sql, err)
	}
}

func (s *seed) location(name, kind string) int64 {
	s.t.Helper()
	var id int64
	if err := s.pool.QueryRow(context.Background(),
		`INSERT INTO locations (name, kind) VALUES ($1, $2) RETURNING id`, name, kind).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *seed) code() string {
	s.seq++
	return "ISK-TOO-" + string(rune('A'+s.seq/26)) + string(rune('A'+s.seq%26))
}

// asset inserts an asset created at createdAt; interval may be nil.
func (s *seed) asset(desc, status string, createdAt time.Time, interval *int, lastMaint *time.Time) string {
	s.t.Helper()
	var id string
	if err := s.pool.QueryRow(context.Background(), `
		INSERT INTO items (code, description, category_id, kind, status, maintenance_interval_days, last_maintenance_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'asset', $4, $5, $6, $7, $7) RETURNING id::text`,
		s.code(), desc, s.cat, status, interval, lastMaint, createdAt).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *seed) consumable(desc string, current, min int, loc *int64) string {
	s.t.Helper()
	stocks := map[int64]int{}
	if current > 0 { // stock lives per location; no location means "Sin ubicación"
		at := s.defaultLocation()
		if loc != nil {
			at = *loc
		}
		stocks[at] = current
	}
	return s.consumableAt(desc, min, stocks)
}

// consumableAt creates a consumable spread over several locations. The item
// and its rows go in one transaction: the total check runs at commit.
func (s *seed) consumableAt(desc string, min int, stocks map[int64]int) string {
	s.t.Helper()
	total := 0
	for _, q := range stocks {
		total += q
	}
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO items (code, description, category_id, kind, unit, stock_current, stock_min, created_at, updated_at)
		VALUES ($1, $2, $3, 'consumable', 'unit', $4, $5, $6, $6) RETURNING id::text`,
		s.code(), desc, s.cat, total, min, now).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	for loc, q := range stocks {
		if _, err := tx.Exec(ctx, `INSERT INTO item_stocks (item_id, location_id, quantity) VALUES ($1::uuid, $2, $3)`, id, loc, q); err != nil {
			s.t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *seed) defaultLocation() int64 {
	s.t.Helper()
	var id int64
	err := s.pool.QueryRow(context.Background(), `SELECT id FROM locations WHERE name = 'Sin ubicación'`).Scan(&id)
	if err != nil {
		id = s.location("Sin ubicación", "storage")
	}
	return id
}

func (s *seed) loan(itemID string, assignee int64, assignedAt time.Time, due *time.Time) {
	s.t.Helper()
	s.must(`INSERT INTO custody_assignments (item_id, assignee_location_id, assigned_at, due_at, assigned_by)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid)`, itemID, assignee, assignedAt, due, s.user)
}

func (s *seed) openFailure(itemID string) {
	s.t.Helper()
	s.must(`INSERT INTO maintenance_records (item_id, kind, status, reported_at, description, reported_by)
		VALUES ($1::uuid, 'corrective', 'open', $2, 'broken', $3::uuid)`, itemID, days(-1), s.user)
}

func (s *seed) summary() domain.Summary {
	s.t.Helper()
	svc := app.NewService(postgres.New(s.pool), func() time.Time { return now })
	sum, err := svc.Summary(context.Background())
	if err != nil {
		s.t.Fatal(err)
	}
	return sum
}

func ip(i int) *int { return &i }

func TestSummaryOnEmptyDatabaseIsZeroWithEmptyLists(t *testing.T) {
	sum := newSeed(t).summary()
	if sum.Items != (domain.ItemCounts{}) || sum.Assets != (domain.AssetCounts{}) || sum.LowStock.Count != 0 ||
		sum.Loans.Active != 0 || sum.Maintenance.OpenFailures != 0 {
		t.Fatalf("expected zeros, got %+v", sum)
	}
	if sum.LowStock.Top == nil || sum.Loans.OverdueTop == nil || sum.Maintenance.OverdueTop == nil {
		t.Error("top lists must be empty, not nil")
	}
}

func TestSummaryCountersAcrossAMixedInventory(t *testing.T) {
	s := newSeed(t)
	bodega := s.location("Bodega", "storage")
	juan := s.location("Juan", "person")
	maria := s.location("Maria", "person")

	// Assets: 2 available, 1 on loan (functional), 1 on loan but inoperative,
	// 1 inoperative, 1 in repair with an open failure.
	s.asset("Available 1", "functional", now, nil, nil)
	s.asset("Available 2", "functional", now, nil, nil)
	loaned := s.asset("Loaned on time", "functional", now, nil, nil)
	s.loan(loaned, juan, days(-2), ptr(days(5)))
	brokenLoan := s.asset("Broken and loaned", "inoperative", now, nil, nil)
	s.loan(brokenLoan, maria, days(-20), ptr(days(-10))) // overdue
	s.asset("Inoperative", "inoperative", now, nil, nil)
	repair := s.asset("In repair", "in_repair", now, nil, nil)
	s.openFailure(repair)
	// Returned loans must not count as active.
	past := s.asset("Was loaned", "functional", now, nil, nil)
	s.loan(past, juan, days(-30), nil)
	s.must(`UPDATE custody_assignments SET returned_at = $2, returned_by = assigned_by WHERE item_id = $1::uuid`, past, days(-20))

	// Consumables: two low (one exactly at the minimum), one fine, one with no minimum.
	s.consumable("Low", 1, 4, &bodega)
	s.consumable("At minimum", 3, 3, nil)
	s.consumable("Fine", 10, 3, &bodega)
	s.consumable("No minimum", 0, 0, nil)

	// Soft-deleted items never count anywhere.
	gone := s.asset("Deleted overdue", "inoperative", days(-400), ip(30), nil)
	s.openFailure(gone)
	goneConsumable := s.consumable("Deleted low", 0, 5, nil)
	s.must(`UPDATE items SET deleted_at = now() WHERE id IN ($1::uuid, $2::uuid)`, gone, goneConsumable)

	sum := s.summary()

	if want := (domain.ItemCounts{Total: 11, Assets: 7, Consumables: 4}); sum.Items != want {
		t.Errorf("items = %+v, want %+v", sum.Items, want)
	}
	// available = functional and no active loan: Available 1, 2 and Was loaned.
	if want := (domain.AssetCounts{Available: 3, Unavailable: 4, OnLoan: 2, Inoperative: 2, InRepair: 1}); sum.Assets != want {
		t.Errorf("assets = %+v, want %+v", sum.Assets, want)
	}
	if sum.LowStock.Count != 2 {
		t.Errorf("low_stock.count = %d, want 2", sum.LowStock.Count)
	}
	if sum.Loans.Active != 2 || sum.Loans.Overdue != 1 {
		t.Errorf("loans = %d active / %d overdue, want 2 / 1", sum.Loans.Active, sum.Loans.Overdue)
	}
	if sum.Maintenance.OpenFailures != 1 {
		t.Errorf("open failures = %d, want 1 (deleted item excluded)", sum.Maintenance.OpenFailures)
	}
	if sum.Maintenance.Overdue != 0 {
		t.Errorf("maintenance overdue = %d, want 0 (deleted item excluded)", sum.Maintenance.Overdue)
	}
	if len(sum.Loans.OverdueTop) != 1 || sum.Loans.OverdueTop[0].AssigneeName != "Maria" ||
		sum.Loans.OverdueTop[0].ItemID != brokenLoan || !sum.Loans.OverdueTop[0].DueAt.Equal(days(-10)) {
		t.Errorf("overdue_top = %+v", sum.Loans.OverdueTop)
	}
	if sum.GeneratedAt != now {
		t.Errorf("generated_at = %v, want the injected clock", sum.GeneratedAt)
	}
}

func TestLowStockTopIsMostSevereFirstAndCapped(t *testing.T) {
	s := newSeed(t)
	bodega := s.location("Bodega", "storage")
	// severity = current/min; 7 low items, only the 5 worst are listed.
	s.consumable("ratio 0.50", 2, 4, &bodega)
	s.consumable("ratio 1.00", 5, 5, nil)
	s.consumable("ratio 0.00", 0, 3, &bodega)
	s.consumable("ratio 0.25", 1, 4, nil)
	s.consumable("ratio 0.80", 4, 5, nil)
	s.consumable("ratio 0.90", 9, 10, nil)
	s.consumable("ratio 0.10", 1, 10, nil)
	s.consumable("fine", 11, 10, nil)

	sum := s.summary()
	if sum.LowStock.Count != 7 {
		t.Errorf("count = %d, want 7", sum.LowStock.Count)
	}
	var got []string
	for _, it := range sum.LowStock.Top {
		got = append(got, it.Description)
	}
	want := []string{"ratio 0.00", "ratio 0.10", "ratio 0.25", "ratio 0.50", "ratio 0.80"}
	if len(got) != len(want) {
		t.Fatalf("top = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("top = %v, want %v", got, want)
		}
	}
	first := sum.LowStock.Top[0]
	if first.LocationName != nil || first.Unit != "unit" || first.StockMin != 3 || first.StockCurrent != 0 {
		t.Errorf("first = %+v", first)
	}
	if second := sum.LowStock.Top[1]; second.LocationName == nil || *second.LocationName != "Sin ubicación" {
		t.Errorf("stock recorded without a location sits in the default location, got %v", second.LocationName)
	}
	// A stocked item with a single location reports its name.
	if last := sum.LowStock.Top[3]; last.Description != "ratio 0.50" || last.LocationName == nil || *last.LocationName != "Bodega" {
		t.Errorf("single-location item: %+v", last)
	}
	if len(first.Stocks) != 0 { // out of stock: no rows
		t.Errorf("first.Stocks = %+v, want none", first.Stocks)
	}
}

func TestLowStockBreakdownListsEveryLocationAndHidesTheSingleName(t *testing.T) {
	s := newSeed(t)
	bodega := s.location("Bodega", "storage")
	taller := s.location("Taller", "storage")
	s.consumableAt("Tornillos", 12, map[int64]int{bodega: 5, taller: 5}) // total 10 <= 12
	s.consumableAt("Pernos", 20, map[int64]int{taller: 7})               // total 7 <= 20

	top := s.summary().LowStock.Top
	if len(top) != 2 || top[0].Description != "Pernos" || top[1].Description != "Tornillos" { // ratios 0.35, 0.83
		t.Fatalf("top = %+v", top)
	}
	if top[0].LocationName == nil || *top[0].LocationName != "Taller" || len(top[0].Stocks) != 1 || top[0].Stocks[0].Quantity != 7 {
		t.Errorf("single-location item: %+v", top[0])
	}
	if top[1].LocationName != nil || len(top[1].Stocks) != 2 || top[1].Stocks[0].LocationName != "Bodega" ||
		top[1].Stocks[1].LocationName != "Taller" || top[1].StockCurrent != 10 {
		t.Errorf("multi-location item: %+v", top[1])
	}
}

func TestOverdueLoansTopIsOldestFirstAndCapped(t *testing.T) {
	s := newSeed(t)
	juan := s.location("Juan", "person")
	for i := 1; i <= 7; i++ { // due 1..7 days ago
		id := s.asset("loan", "functional", now, nil, nil)
		s.loan(id, juan, days(-30), ptr(days(-i)))
	}
	notYet := s.asset("not due", "functional", now, nil, nil)
	s.loan(notYet, juan, days(-30), ptr(days(1)))
	noDue := s.asset("no due date", "functional", now, nil, nil)
	s.loan(noDue, juan, days(-30), nil)

	sum := s.summary()
	if sum.Loans.Active != 9 || sum.Loans.Overdue != 7 {
		t.Errorf("loans = %d active / %d overdue, want 9 / 7", sum.Loans.Active, sum.Loans.Overdue)
	}
	if len(sum.Loans.OverdueTop) != 5 {
		t.Fatalf("overdue_top has %d rows, want 5", len(sum.Loans.OverdueTop))
	}
	for i, o := range sum.Loans.OverdueTop {
		if want := days(-(7 - i)); !o.DueAt.Equal(want) {
			t.Errorf("overdue_top[%d].due_at = %v, want %v", i, o.DueAt, want)
		}
	}
}

func TestMaintenanceOverdueUsesLastMaintenanceOrCreation(t *testing.T) {
	s := newSeed(t)
	// due = COALESCE(last, created) + interval days; overdue when < now.
	s.asset("created 100d ago, 30d interval (due -70d)", "functional", days(-100), ip(30), nil)
	s.asset("serviced 10d ago, 30d interval", "functional", days(-100), ip(30), ptr(days(-10)))
	s.asset("serviced 40d ago, 30d interval (due -10d)", "functional", days(-100), ip(30), ptr(days(-40)))
	s.asset("no interval", "functional", days(-400), nil, nil)
	s.asset("due exactly now", "functional", days(-30), ip(30), nil)
	s.asset("due tomorrow", "functional", days(-29), ip(30), nil)
	s.asset("created 6d ago, 5d interval (due -1d)", "inoperative", days(-6), ip(5), nil)

	sum := s.summary()
	if sum.Maintenance.Overdue != 3 {
		t.Errorf("maintenance.overdue = %d, want 3", sum.Maintenance.Overdue)
	}
	if len(sum.Maintenance.OverdueTop) != 3 {
		t.Fatalf("overdue_top = %+v", sum.Maintenance.OverdueTop)
	}
	for i, wantDue := range []time.Time{days(-70), days(-10), days(-1)} {
		if got := sum.Maintenance.OverdueTop[i].MaintenanceDueAt; !got.Equal(wantDue) {
			t.Errorf("overdue_top[%d].due = %v, want %v", i, got, wantDue)
		}
	}
	if sum.Maintenance.OverdueTop[0].Description != "created 100d ago, 30d interval (due -70d)" {
		t.Errorf("most overdue first, got %q", sum.Maintenance.OverdueTop[0].Description)
	}
}

func TestMaintenanceOverdueTopIsCapped(t *testing.T) {
	s := newSeed(t)
	for i := 1; i <= 8; i++ {
		s.asset("asset", "functional", days(-10-i), ip(5), nil) // due (5-10-i) days
	}
	sum := s.summary()
	if sum.Maintenance.Overdue != 8 || len(sum.Maintenance.OverdueTop) != 5 {
		t.Fatalf("overdue = %d, top = %d; want 8 and 5", sum.Maintenance.Overdue, len(sum.Maintenance.OverdueTop))
	}
	if !sum.Maintenance.OverdueTop[0].MaintenanceDueAt.Equal(days(-13)) { // created -18d + 5d
		t.Errorf("first due = %v", sum.Maintenance.OverdueTop[0].MaintenanceDueAt)
	}
}

func ptr[T any](v T) *T { return &v }

// order inserts a purchase order for the supplier with one line for the item.
func (s *seed) order(supplier int64, status, currency string, orderedAt *time.Time, itemID string, qty, received int, cost int64) {
	s.t.Helper()
	var id int64
	if err := s.pool.QueryRow(context.Background(),
		`INSERT INTO purchase_orders (supplier_id, status, currency, ordered_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		supplier, status, currency, orderedAt).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	s.must(`INSERT INTO purchase_order_lines (order_id, position, item_id, quantity, received_qty, unit_cost_minor)
		VALUES ($1, 1, $2, $3, $4, $5)`, id, itemID, qty, received, cost)
}

func (s *seed) supplier(name string, active bool) int64 {
	s.t.Helper()
	var id int64
	if err := s.pool.QueryRow(context.Background(),
		`INSERT INTO suppliers (name, active) VALUES ($1, $2) RETURNING id`, name, active).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func TestPendingOrderedCountsOnlyOpenOrders(t *testing.T) {
	s := newSeed(t)
	sup := s.supplier("Alfa", true)
	item := s.consumable("Tape", 1, 5, nil)
	at := days(-3)
	s.order(sup, "ordered", "USD", &at, item, 10, 0, 100)
	s.order(sup, "partially_received", "USD", &at, item, 8, 3, 100) // 5 left
	s.order(sup, "draft", "USD", nil, item, 99, 0, 100)
	s.order(sup, "received", "USD", &at, item, 7, 7, 100)
	s.order(sup, "cancelled", "USD", &at, item, 50, 0, 100)

	top := s.summary().LowStock.Top
	if len(top) != 1 || top[0].PendingOrdered != 15 {
		t.Fatalf("pending_ordered = %+v, want 15 (10 + 5 left)", top)
	}
}

func TestReorderCandidatesUseLatestLiveSupplier(t *testing.T) {
	s := newSeed(t)
	ctx := context.Background()
	alfa := s.supplier("Alfa", true)
	beta := s.supplier("Beta", true)
	gone := s.supplier("Gone", false) // inactive: ignored as habitual supplier

	both := s.consumable("Tape", 1, 5, nil)
	never := s.consumable("Glue", 0, 4, nil)
	onlyInactive := s.consumable("Pins", 2, 6, nil)
	s.consumable("Plenty", 50, 5, nil) // not low

	old, recent := days(-30), days(-2)
	s.order(alfa, "received", "VES", &old, both, 5, 5, 1000)
	s.order(beta, "ordered", "USD", &recent, both, 5, 0, 250)
	s.order(gone, "received", "USD", &old, onlyInactive, 5, 5, 300)

	repo := postgres.New(s.pool)
	got, err := repo.ReorderCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("candidates = %d, want the 3 low-stock consumables: %+v", len(got), got)
	}
	by := map[string]domain.ReorderCandidate{}
	for _, c := range got {
		by[c.ItemID] = c
	}
	b := by[both]
	if b.SupplierName == nil || *b.SupplierName != "Beta" || b.LastUnitCostMinor == nil || *b.LastUnitCostMinor != 250 ||
		b.LastCurrency == nil || *b.LastCurrency != "USD" || b.PendingOrdered != 5 {
		t.Errorf("both = %+v, want latest order (Beta, 250 USD, pending 5)", b)
	}
	if n := by[never]; n.SupplierID != nil || n.LastUnitCostMinor != nil || n.LastCurrency != nil {
		t.Errorf("never bought must have no supplier or cost: %+v", n)
	}
	if o := by[onlyInactive]; o.SupplierID != nil {
		t.Errorf("an inactive supplier is not a habitual supplier: %+v", o)
	}
	// Most severe first: stock/min 0, then 1/5, then 2/6.
	if got[0].ItemID != never || got[1].ItemID != both || got[2].ItemID != onlyInactive {
		t.Errorf("order = %s %s %s", got[0].Code, got[1].Code, got[2].Code)
	}
}

func TestReorderCandidatesOnEmptyDatabase(t *testing.T) {
	s := newSeed(t)
	got, err := postgres.New(s.pool).ReorderCandidates(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, %v; want empty non-nil slice", got, err)
	}
}
