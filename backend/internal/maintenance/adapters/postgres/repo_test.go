package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/maintenance/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/maintenance/app"
	"github.com/edgar/inventario/backend/internal/maintenance/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ip(i int) *int         { return &i }
func sp(s string) *string   { return &s }
func fp(f float64) *float64 { return &f }

type fixture struct {
	pool    *pgxpool.Pool
	svc     *app.Service
	catalog *catalogapp.Service
	userA   string
	userB   string
	catHer  int64
	catVeh  int64
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
	f := fixture{pool: pool}
	for email, dst := range map[string]*string{"ana@example.com": &f.userA, "omar@example.com": &f.userB} {
		name := "Ana Operator"
		if dst == &f.userB {
			name = "Omar Operator"
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (name, email, password_hash, role) VALUES ($1, $2, 'x', 'operator') RETURNING id::text`,
			name, email).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	cat := catalogpg.New(pool)
	f.catalog = catalogapp.NewService(cat.Categories(), cat.Locations(), cat.Items())
	f.svc = app.NewService(postgres.New(pool), nil)
	for _, c := range []struct {
		name, prefix string
		dst          *int64
	}{{"Herramientas", "HER", &f.catHer}, {"Vehiculos", "VEH", &f.catVeh}} {
		cr, err := f.catalog.CreateCategory(ctx, c.name, c.prefix)
		if err != nil {
			t.Fatal(err)
		}
		*c.dst = cr.ID
	}
	return f
}

func (f fixture) asset(t *testing.T, desc string, interval *int) catalogdomain.Item {
	t.Helper()
	it, err := f.catalog.CreateItem(t.Context(), catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: desc, CategoryID: f.catHer, Kind: catalogdomain.KindAsset, MaintenanceIntervalDays: interval}})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

type itemRow struct {
	status string
	last   *time.Time
}

func (f fixture) item(t *testing.T, id string) itemRow {
	t.Helper()
	var r itemRow
	if err := f.pool.QueryRow(t.Context(), `SELECT status, last_maintenance_at FROM items WHERE id = $1::uuid`, id).Scan(&r.status, &r.last); err != nil {
		t.Fatal(err)
	}
	return r
}

func (f fixture) report(t *testing.T, itemID string, status domain.ItemStatus) domain.Record {
	t.Helper()
	r, err := f.svc.Create(t.Context(), itemID, f.userA, domain.CreateRequest{Kind: domain.KindCorrective, Description: "Motor burnt", ItemStatus: status})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFailureRepairLifecycleKeepsRecordAndItemInStep(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "Taladro", nil)

	r := f.report(t, it.ID, "")
	if r.Status != domain.StatusOpen || r.ItemStatus != domain.ItemInoperative || r.ReportedByName != "Ana Operator" || r.ItemCode != it.Code || r.ClosedAt != nil {
		t.Errorf("report: %+v", r)
	}
	if got := f.item(t, it.ID); got.status != "inoperative" || got.last != nil {
		t.Errorf("item after report: %+v", got)
	}

	r, err := f.svc.Update(ctx, r.ID, domain.UpdateRequest{SetDestination: true, Destination: sp("Taller de Reparación Interno"), ItemStatus: domain.ItemInRepair})
	if err != nil || r.Destination == nil || *r.Destination != "Taller de Reparación Interno" || r.ItemStatus != domain.ItemInRepair {
		t.Fatalf("update: %+v %v", r, err)
	}
	if got := f.item(t, it.ID); got.status != "in_repair" {
		t.Errorf("item after update: %+v", got)
	}

	r, err = f.svc.Close(ctx, r.ID, f.userB, domain.CloseRequest{Cost: fp(120.55), ResolutionNote: "Replaced motor"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.StatusClosed || r.ClosedByName == nil || *r.ClosedByName != "Omar Operator" || r.Cost == nil || *r.Cost != 120.55 ||
		r.ResolutionNote == nil || r.ClosedAt == nil || r.ItemStatus != domain.ItemFunctional {
		t.Errorf("close: %+v", r)
	}
	got := f.item(t, it.ID)
	if got.status != "functional" || got.last == nil || !got.last.Equal(*r.ClosedAt) {
		t.Errorf("item after close: %+v (closed_at %v)", got, r.ClosedAt)
	}
	if _, err := f.svc.Close(ctx, r.ID, f.userB, domain.CloseRequest{}); !errors.Is(err, domain.ErrRecordClosed) {
		t.Errorf("second close: %v", err)
	}
	if _, err := f.svc.Update(ctx, r.ID, domain.UpdateRequest{Description: sp("x")}); !errors.Is(err, domain.ErrRecordClosed) {
		t.Errorf("update closed: %v", err)
	}
	f.report(t, it.ID, domain.ItemInRepair) // a new failure is allowed once the last one is closed
}

func TestRejectedWritesChangeNothing(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "Taladro", nil)
	cons, err := f.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Guantes", CategoryID: f.catHer, Kind: catalogdomain.KindConsumable, Unit: sp("pair")}})
	if err != nil {
		t.Fatal(err)
	}
	f.report(t, it.ID, "")
	if _, err := f.svc.Create(ctx, it.ID, f.userA, domain.CreateRequest{Kind: domain.KindCorrective, Description: "again", ItemStatus: domain.ItemInRepair}); !errors.Is(err, domain.ErrOpenFailureExists) {
		t.Errorf("second open failure: %v", err)
	}
	if _, err := f.svc.Create(ctx, cons.ID, f.userA, domain.CreateRequest{Kind: domain.KindPreventive, Description: "x"}); !errors.Is(err, domain.ErrNotApplicable) {
		t.Errorf("consumable: %v", err)
	}
	if _, err := f.svc.Create(ctx, "not-a-uuid", f.userA, domain.CreateRequest{Kind: domain.KindCorrective, Description: "x"}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("bad id: %v", err)
	}
	if _, err := f.svc.Update(ctx, 9999, domain.UpdateRequest{Description: sp("x")}); !errors.Is(err, domain.ErrRecordNotFound) {
		t.Errorf("missing record: %v", err)
	}
	if got := f.item(t, it.ID); got.status != "inoperative" {
		t.Errorf("rejected report changed the item to %s", got.status)
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM maintenance_records`).Scan(&n)
	if n != 1 {
		t.Errorf("records = %d, want 1", n)
	}

	// Soft-deleted items reject every write and read.
	if err := f.catalog.DeleteItem(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, it.ID, f.userA, domain.CreateRequest{Kind: domain.KindPreventive, Description: "x"}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("deleted create: %v", err)
	}
	if _, err := f.svc.Close(ctx, 1, f.userA, domain.CloseRequest{}); !errors.Is(err, domain.ErrRecordNotFound) {
		t.Errorf("deleted close: %v", err)
	}
	if _, _, _, err := f.svc.History(ctx, it.ID, 0, 0); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("deleted history: %v", err)
	}
	if rs, total, _, _ := f.svc.List(ctx, domain.ListFilter{}, 0, 0); total != 0 || len(rs) != 0 {
		t.Errorf("records of deleted items must not be listed: %d", total)
	}
}

func TestPreventiveUpdatesLastMaintenanceKeepsStatusAndDerivesNextDue(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "Compresor", ip(90))
	f.report(t, it.ID, domain.ItemInRepair)

	performed := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	r, err := f.svc.Create(ctx, it.ID, f.userB, domain.CreateRequest{Kind: domain.KindPreventive, Description: "Filter change", PerformedAt: &performed, Cost: fp(25)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.StatusClosed || r.NextDueAt == nil || !r.NextDueAt.Equal(domain.AddDays(performed, 90)) || r.ClosedByName == nil || *r.ClosedByName != "Omar Operator" {
		t.Errorf("%+v", r)
	}
	got := f.item(t, it.ID)
	if got.status != "in_repair" || got.last == nil || !got.last.Equal(performed) {
		t.Errorf("item: %+v", got)
	}

	// An older service is recorded but never moves last_maintenance_at back.
	old := performed.Add(-30 * 24 * time.Hour)
	if _, err := f.svc.Create(ctx, it.ID, f.userA, domain.CreateRequest{Kind: domain.KindPreventive, Description: "Backfilled", PerformedAt: &old}); err != nil {
		t.Fatal(err)
	}
	if got := f.item(t, it.ID); !got.last.Equal(performed) {
		t.Errorf("last maintenance moved back to %v", got.last)
	}
}

func TestConcurrentFailureReportsYieldExactlyOne(t *testing.T) {
	f := setup(t)
	it := f.asset(t, "Taladro", nil)
	const n = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicts := 0, 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Create(t.Context(), it.ID, f.userA, domain.CreateRequest{Kind: domain.KindCorrective, Description: "parallel"})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrOpenFailureExists):
				conflicts++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || conflicts != n-1 {
		t.Errorf("ok=%d conflicts=%d", ok, conflicts)
	}
	var open int
	_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM maintenance_records WHERE item_id = $1::uuid AND status = 'open'`, it.ID).Scan(&open)
	if open != 1 {
		t.Errorf("open records = %d", open)
	}
}

func TestConcurrentCloseYieldsExactlyOneSuccess(t *testing.T) {
	f := setup(t)
	it := f.asset(t, "Taladro", nil)
	r := f.report(t, it.ID, "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, closed := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Close(t.Context(), r.ID, f.userB, domain.CloseRequest{})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrRecordClosed):
				closed++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || closed != 7 {
		t.Errorf("ok=%d closed=%d", ok, closed)
	}
}

func TestHistoryAndListOrderingAndFilters(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	a := f.asset(t, "Taladro Bosch", nil)
	b := f.asset(t, "Sierra", nil)
	now := time.Now()
	at := func(d time.Duration) *time.Time { v := now.Add(d).Truncate(time.Microsecond); return &v }

	if _, err := f.svc.Create(ctx, a.ID, f.userA, domain.CreateRequest{Kind: domain.KindPreventive, Description: "Greasing", PerformedAt: at(-72 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, b.ID, f.userA, domain.CreateRequest{Kind: domain.KindPreventive, Description: "Blade sharpening", PerformedAt: at(-1 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	open, err := f.svc.Create(ctx, a.ID, f.userA, domain.CreateRequest{Kind: domain.KindCorrective, Description: "Chuck broken", ReportedAt: at(-48 * time.Hour), Destination: sp("Espera de Repuestos")})
	if err != nil {
		t.Fatal(err)
	}

	hist, total, _, err := f.svc.History(ctx, a.ID, 0, 0)
	if err != nil || total != 2 || hist[0].ID != open.ID || hist[1].Description != "Greasing" {
		t.Fatalf("history: %v %d %+v", err, total, hist)
	}
	if page, _, _, _ := f.svc.History(ctx, a.ID, 1, 1); len(page) != 1 || page[0].Description != "Greasing" {
		t.Errorf("history offset: %+v", page)
	}

	codes := func(flt domain.ListFilter) []string {
		rs, _, _, err := f.svc.List(ctx, flt, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rs {
			out = append(out, r.ItemCode+"/"+r.Description)
		}
		return out
	}
	all := codes(domain.ListFilter{})
	if len(all) != 3 || !strings.HasSuffix(all[0], "Chuck broken") || !strings.HasSuffix(all[1], "Blade sharpening") || !strings.HasSuffix(all[2], "Greasing") {
		t.Errorf("open first, then newest: %v", all)
	}
	if got := codes(domain.ListFilter{Status: domain.StatusOpen}); len(got) != 1 {
		t.Errorf("status: %v", got)
	}
	if got := codes(domain.ListFilter{Kind: domain.KindPreventive}); len(got) != 2 {
		t.Errorf("kind: %v", got)
	}
	if got := codes(domain.ListFilter{Destination: "espera de repuestos"}); len(got) != 1 {
		t.Errorf("destination case-insensitive: %v", got)
	}
	if got := codes(domain.ListFilter{Destination: "espera"}); len(got) != 0 {
		t.Errorf("destination is exact: %v", got)
	}
	if got := codes(domain.ListFilter{ItemID: b.ID}); len(got) != 1 {
		t.Errorf("item: %v", got)
	}
	if got := codes(domain.ListFilter{Q: "bosch"}); len(got) != 2 {
		t.Errorf("q on item description: %v", got)
	}
	if got := codes(domain.ListFilter{Q: "sharpen"}); len(got) != 1 {
		t.Errorf("q on record description: %v", got)
	}
	if got := codes(domain.ListFilter{Q: a.Code}); len(got) != 2 {
		t.Errorf("q on item code: %v", got)
	}
	if got := codes(domain.ListFilter{Q: "100%"}); len(got) != 0 {
		t.Errorf("q must escape LIKE wildcards: %v", got)
	}
}

func TestDueListingOrderingWindowAndFilters(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	now := time.Now()
	mk := func(desc string, cat int64, interval *int, created time.Time, last *time.Time) string {
		it, err := f.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
			Description: desc, CategoryID: cat, Kind: catalogdomain.KindAsset, MaintenanceIntervalDays: interval}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `UPDATE items SET created_at = $2, last_maintenance_at = $3 WHERE id = $1::uuid`, it.ID, created, last); err != nil {
			t.Fatal(err)
		}
		return it.ID
	}
	day := 24 * time.Hour
	lastTen := now.Add(-10 * day)
	veryOverdue := mk("Very overdue", f.catHer, ip(30), now.Add(-200*day), nil)          // due 170d ago (from creation)
	overdue := mk("Overdue", f.catHer, ip(30), now.Add(-200*day), ptr(now.Add(-40*day))) // due 10d ago
	soon := mk("Soon", f.catVeh, ip(30), now.Add(-200*day), ptr(now.Add(-25*day)))       // due in 5d
	far := mk("Far", f.catHer, ip(365), now.Add(-200*day), &lastTen)                     // due in 355d
	mk("No interval", f.catHer, nil, now.Add(-200*day), nil)
	deleted := mk("Deleted overdue", f.catHer, ip(1), now.Add(-200*day), nil)
	cons, err := f.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Gloves", CategoryID: f.catHer, Kind: catalogdomain.KindConsumable, Unit: sp("pair")}})
	if err != nil {
		t.Fatal(err)
	}
	_ = cons
	if err := f.catalog.DeleteItem(ctx, deleted); err != nil {
		t.Fatal(err)
	}

	ids := func(flt domain.DueFilter) []string {
		items, _, _, _, err := f.svc.Due(ctx, flt, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range items {
			out = append(out, d.ItemID)
		}
		return out
	}
	eq := func(name string, got, want []string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
	eq("default window = overdue only, most overdue first", ids(domain.DueFilter{}), []string{veryOverdue, overdue})
	eq("30-day window", ids(domain.DueFilter{WithinDays: 30}), []string{veryOverdue, overdue, soon})
	eq("365-day window", ids(domain.DueFilter{WithinDays: 365}), []string{veryOverdue, overdue, soon, far})
	eq("category", ids(domain.DueFilter{WithinDays: 365, CategoryID: &f.catVeh}), []string{soon})
	eq("q", ids(domain.DueFilter{WithinDays: 365, Q: "soon"}), []string{soon})

	items, total, _, clock, err := f.svc.Due(ctx, domain.DueFilter{WithinDays: 30}, 2, 0)
	if err != nil || total != 3 || len(items) != 2 {
		t.Fatalf("paging: %v total=%d len=%d", err, total, len(items))
	}
	if !items[0].Overdue(clock) || !items[1].Overdue(clock) || items[0].CategoryName != "Herramientas" || items[0].IntervalDays != 30 {
		t.Errorf("%+v", items)
	}
	if rest, _, _, _, _ := f.svc.Due(ctx, domain.DueFilter{WithinDays: 30}, 2, 2); len(rest) != 1 || rest[0].ItemID != soon || rest[0].Overdue(clock) {
		t.Errorf("second page: %+v", rest)
	}

	// Servicing an overdue asset moves it out of the overdue list.
	if _, err := f.svc.Create(ctx, veryOverdue, f.userA, domain.CreateRequest{Kind: domain.KindPreventive, Description: "Service"}); err != nil {
		t.Fatal(err)
	}
	eq("after service", ids(domain.DueFilter{}), []string{overdue})
}

func ptr[T any](v T) *T { return &v }

func TestCheckConstraintsGuardTheTable(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "Taladro", nil)
	insert := func(sql string, args ...any) error {
		_, err := f.pool.Exec(ctx, sql, args...)
		return err
	}
	base := `INSERT INTO maintenance_records (item_id, kind, status, reported_at, description, destination, cost, next_due_at, closed_at, resolution_note, reported_by, closed_by) VALUES `
	cases := []struct {
		name string
		sql  string
		ok   bool
	}{
		{"open corrective", `($1::uuid, 'corrective', 'open', now() - interval '1 day', 'x', NULL, NULL, NULL, NULL, NULL, $2::uuid, NULL)`, true},
		{"preventive cannot be open", `($1::uuid, 'preventive', 'open', now(), 'x', NULL, NULL, NULL, NULL, NULL, $2::uuid, NULL)`, false},
		{"open rows have no closed fields", `($1::uuid, 'corrective', 'open', now() - interval '1 day', 'x', NULL, NULL, NULL, now(), NULL, $2::uuid, NULL)`, false},
		{"open rows have no cost", `($1::uuid, 'corrective', 'open', now() - interval '1 day', 'x', NULL, 5, NULL, NULL, NULL, $2::uuid, NULL)`, false},
		{"closed needs closed_at", `($1::uuid, 'corrective', 'closed', now() - interval '1 day', 'x', NULL, NULL, NULL, NULL, NULL, $2::uuid, $2::uuid)`, false},
		{"closed needs closed_by", `($1::uuid, 'corrective', 'closed', now() - interval '1 day', 'x', NULL, NULL, NULL, now(), NULL, $2::uuid, NULL)`, false},
		{"closed before reported", `($1::uuid, 'corrective', 'closed', now(), 'x', NULL, NULL, NULL, now() - interval '1 day', NULL, $2::uuid, $2::uuid)`, false},
		{"closed corrective", `($1::uuid, 'corrective', 'closed', now() - interval '2 day', 'x', NULL, 10.5, NULL, now() - interval '1 day', 'done', $2::uuid, $2::uuid)`, true},
		{"negative cost", `($1::uuid, 'preventive', 'closed', now(), 'x', NULL, -1, NULL, now(), NULL, $2::uuid, $2::uuid)`, false},
		{"blank description", `($1::uuid, 'preventive', 'closed', now(), '  ', NULL, NULL, NULL, now(), NULL, $2::uuid, $2::uuid)`, false},
		{"destination too long", `($1::uuid, 'preventive', 'closed', now(), 'x', repeat('d', 121), NULL, NULL, now(), NULL, $2::uuid, $2::uuid)`, false},
		{"unknown kind", `($1::uuid, 'other', 'closed', now(), 'x', NULL, NULL, NULL, now(), NULL, $2::uuid, $2::uuid)`, false},
		{"next due only for preventive", `($1::uuid, 'corrective', 'closed', now() - interval '2 day', 'x', NULL, NULL, now() + interval '1 day', now() - interval '1 day', NULL, $2::uuid, $2::uuid)`, false},
		{"next due must follow performed", `($1::uuid, 'preventive', 'closed', now(), 'x', NULL, NULL, now() - interval '1 day', now(), NULL, $2::uuid, $2::uuid)`, false},
		{"valid preventive", `($1::uuid, 'preventive', 'closed', now(), 'x', NULL, 3, now() + interval '30 day', now(), NULL, $2::uuid, $2::uuid)`, true},
	}
	for _, c := range cases {
		err := insert(base+c.sql, it.ID, f.userA)
		if c.ok != (err == nil) {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
	// The unique index rejects a second open corrective record even without the app.
	if err := insert(base+`($1::uuid, 'corrective', 'open', now(), 'dup', NULL, NULL, NULL, NULL, NULL, $2::uuid, NULL)`, it.ID, f.userA); err == nil || !strings.Contains(err.Error(), "maintenance_records_one_open_failure_per_item") {
		t.Errorf("duplicate open failure: %v", err)
	}
}
