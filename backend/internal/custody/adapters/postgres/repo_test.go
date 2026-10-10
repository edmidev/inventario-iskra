package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/custody/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/custody/app"
	"github.com/edgar/inventario/backend/internal/custody/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func p64(v int64) *int64 { return &v }

type fixture struct {
	pool    *pgxpool.Pool
	custody *app.Service
	catalog *catalogapp.Service
	userA   string
	userB   string
	catID   int64
	bodega  int64
	person  int64
	maleta  int64
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
	f.custody = app.NewService(postgres.New(pool), nil)
	c, err := f.catalog.CreateCategory(ctx, "Herramientas", "HER")
	if err != nil {
		t.Fatal(err)
	}
	f.catID = c.ID
	for _, l := range []struct {
		name string
		kind catalogdomain.LocationKind
		dst  *int64
	}{
		{"Bodega", catalogdomain.LocationStorage, &f.bodega},
		{"Alejandro", catalogdomain.LocationPerson, &f.person},
		{"Maleta de Alejandro", catalogdomain.LocationContainer, &f.maleta},
	} {
		loc, err := f.catalog.CreateLocation(ctx, l.name, l.kind, nil)
		if err != nil {
			t.Fatal(err)
		}
		*l.dst = loc.ID
	}
	return f
}

func (f fixture) asset(t *testing.T, code string) catalogdomain.Item {
	t.Helper()
	it, err := f.catalog.CreateItem(t.Context(), catalogapp.CreateItemInput{Code: code, NewItemInput: catalogdomain.NewItemInput{
		Description: "Taladro " + code, CategoryID: f.catID, Kind: catalogdomain.KindAsset, LocationID: &f.bodega}})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func (f fixture) location(t *testing.T, itemID string) *int64 {
	t.Helper()
	var loc *int64
	if err := f.pool.QueryRow(t.Context(), `SELECT location_id FROM items WHERE id = $1::uuid`, itemID).Scan(&loc); err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestAssignAndReturnMoveTheItem(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "ISK-HER-001")
	due := time.Now().Add(24 * time.Hour)

	a, err := f.custody.Assign(ctx, it.ID, f.userA, domain.AssignRequest{AssigneeLocationID: f.person, DueAt: &due, Note: "site visit"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.Active() || a.AssigneeName != "Alejandro" || a.AssigneeKind != domain.PlacePerson || a.AssignedByName != "Ana Operator" ||
		a.ItemCode != "ISK-HER-001" || a.OriginName == nil || *a.OriginName != "Bodega" || a.AssignNote != "site visit" {
		t.Errorf("%+v", a)
	}
	if loc := f.location(t, it.ID); loc == nil || *loc != f.person {
		t.Errorf("after assign location = %v, want %d", loc, f.person)
	}

	r, err := f.custody.Return(ctx, it.ID, f.userB, domain.ReturnRequest{Note: "clean"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Active() || r.ReturnedByName == nil || *r.ReturnedByName != "Omar Operator" || r.ReturnNote != "clean" ||
		r.ReturnedToID == nil || *r.ReturnedToID != f.bodega {
		t.Errorf("%+v", r)
	}
	if loc := f.location(t, it.ID); loc == nil || *loc != f.bodega {
		t.Errorf("after return location = %v, want origin %d", loc, f.bodega)
	}
	if _, err := f.custody.Return(ctx, it.ID, f.userB, domain.ReturnRequest{}); !errors.Is(err, domain.ErrNotAssigned) {
		t.Errorf("second return: %v", err)
	}

	// Returned into a chosen container, after a second loan.
	if _, err := f.custody.Assign(ctx, it.ID, f.userA, domain.AssignRequest{AssigneeLocationID: f.person}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.custody.Return(ctx, it.ID, f.userA, domain.ReturnRequest{ReturnLocationID: &f.maleta}); err != nil {
		t.Fatal(err)
	}
	if loc := f.location(t, it.ID); *loc != f.maleta {
		t.Errorf("location = %v, want maleta", *loc)
	}
}

func TestRejectedAssignsWriteNothing(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "ISK-HER-001")
	cons, err := f.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Guantes", CategoryID: f.catID, Kind: catalogdomain.KindConsumable, Unit: new("pair")}})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := f.catalog.UpdateItem(ctx, f.asset(t, "ISK-HER-101").ID, catalogdomain.ItemPatch{Status: new(catalogdomain.StatusInRepair)})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		id   string
		req  domain.AssignRequest
		want error
	}{
		{"consumable", cons.ID, domain.AssignRequest{AssigneeLocationID: f.person}, domain.ErrNotApplicable},
		{"in repair", broken.ID, domain.AssignRequest{AssigneeLocationID: f.person}, domain.ErrItemNotAvailable},
		{"storage assignee", it.ID, domain.AssignRequest{AssigneeLocationID: f.bodega}, domain.ErrInvalidInput},
		{"unknown assignee", it.ID, domain.AssignRequest{AssigneeLocationID: 9999}, domain.ErrInvalidInput},
		{"past due", it.ID, domain.AssignRequest{AssigneeLocationID: f.person, DueAt: new(time.Now().Add(-time.Hour))}, domain.ErrInvalidInput},
		{"unknown item", "00000000-0000-4000-8000-000000000000", domain.AssignRequest{AssigneeLocationID: f.person}, domain.ErrItemNotFound},
		{"malformed item id", "nope", domain.AssignRequest{AssigneeLocationID: f.person}, domain.ErrItemNotFound},
	}
	for _, tc := range cases {
		if _, err := f.custody.Assign(ctx, tc.id, f.userA, tc.req); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM custody_assignments`).Scan(&n)
	if n != 0 {
		t.Errorf("rejected assigns left %d rows", n)
	}
	if loc := f.location(t, it.ID); *loc != f.bodega {
		t.Errorf("location changed by a rejected assign: %v", *loc)
	}
}

// The partial unique index plus the item row lock must let exactly one of N
// parallel assigns of the same item win; everyone else sees already_assigned.
func TestConcurrentAssignsOfOneItemYieldExactlyOne(t *testing.T) {
	f := setup(t)
	it := f.asset(t, "ISK-HER-001")
	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	won, conflicts := 0, 0
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		assignee := f.person
		if i%2 == 1 {
			assignee = f.maleta
		}
		go func() {
			defer wg.Done()
			<-start
			_, err := f.custody.Assign(context.Background(), it.ID, f.userA, domain.AssignRequest{AssigneeLocationID: assignee})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case errors.Is(err, domain.ErrAlreadyAssigned):
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if won != 1 || conflicts != n-1 {
		t.Fatalf("won=%d conflicts=%d, want 1 and %d", won, conflicts, n-1)
	}
	var active int
	_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM custody_assignments WHERE item_id = $1::uuid AND returned_at IS NULL`, it.ID).Scan(&active)
	if active != 1 {
		t.Errorf("active rows = %d", active)
	}
}

func TestConcurrentReturnsCloseTheLoanOnce(t *testing.T) {
	f := setup(t)
	it := f.asset(t, "ISK-HER-001")
	if _, err := f.custody.Assign(t.Context(), it.ID, f.userA, domain.AssignRequest{AssigneeLocationID: f.person}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, notAssigned := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.custody.Return(context.Background(), it.ID, f.userB, domain.ReturnRequest{})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrNotAssigned):
				notAssigned++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || notAssigned != 7 {
		t.Errorf("ok=%d notAssigned=%d", ok, notAssigned)
	}
}

func TestSchemaConstraints(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "ISK-HER-001")
	insert := func(sql string, args ...any) error {
		_, err := f.pool.Exec(ctx, sql, args...)
		return err
	}
	base := `INSERT INTO custody_assignments (item_id, assignee_location_id, assigned_by, assigned_at`
	if err := insert(base+`) VALUES ($1::uuid, $2, $3::uuid, now())`, it.ID, f.person, f.userA); err != nil {
		t.Fatal(err)
	}
	if err := insert(base+`) VALUES ($1::uuid, $2, $3::uuid, now())`, it.ID, f.maleta, f.userA); err == nil {
		t.Error("a second active assignment for one item must violate the partial unique index")
	}
	if err := insert(`UPDATE custody_assignments SET returned_at = assigned_at - interval '1 second', returned_by = $1::uuid`, f.userA); err == nil {
		t.Error("returned_at before assigned_at must be rejected")
	}
	if err := insert(`UPDATE custody_assignments SET returned_at = now()`); err == nil {
		t.Error("a return without returned_by must be rejected")
	}
	if err := insert(`UPDATE custody_assignments SET returned_by = $1::uuid`, f.userA); err == nil {
		t.Error("returned_by without returned_at must be rejected")
	}
	if err := insert(`UPDATE custody_assignments SET due_at = assigned_at`); err == nil {
		t.Error("due_at must be after assigned_at")
	}
	// A closed row no longer blocks a new active one.
	if err := insert(`UPDATE custody_assignments SET returned_at = now(), returned_by = $1::uuid`, f.userA); err != nil {
		t.Fatal(err)
	}
	if err := insert(base+`) VALUES ($1::uuid, $2, $3::uuid, now())`, it.ID, f.maleta, f.userA); err != nil {
		t.Errorf("new loan after return: %v", err)
	}
}

func TestOriginDeletedReturnsWithoutLocation(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.asset(t, "ISK-HER-001")
	if _, err := f.custody.Assign(ctx, it.ID, f.userA, domain.AssignRequest{AssigneeLocationID: f.person}); err != nil {
		t.Fatal(err)
	}
	if err := f.catalog.DeleteLocation(ctx, f.bodega); err != nil { // origin: no item sits there now
		t.Fatal(err)
	}
	r, err := f.custody.Return(ctx, it.ID, f.userA, domain.ReturnRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if r.ReturnedToID != nil || f.location(t, it.ID) != nil {
		t.Errorf("returned_to=%v location=%v, want both nil", r.ReturnedToID, f.location(t, it.ID))
	}
	// The assignee cannot be deleted while history references it.
	if err := f.catalog.DeleteLocation(ctx, f.person); !errors.Is(err, catalogdomain.ErrLocationInUse) {
		t.Errorf("deleting an assignee with history: %v", err)
	}
}

func TestHistoryAndActiveListing(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	drill := f.asset(t, "ISK-HER-001")
	saw := f.asset(t, "ISK-HER-002")
	late := f.asset(t, "ISK-HER-003")

	if _, err := f.custody.Assign(ctx, drill.ID, f.userA, domain.AssignRequest{AssigneeLocationID: f.person}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.custody.Return(ctx, drill.ID, f.userA, domain.ReturnRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.custody.Assign(ctx, drill.ID, f.userB, domain.AssignRequest{AssigneeLocationID: f.maleta}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.custody.Assign(ctx, saw.ID, f.userA, domain.AssignRequest{AssigneeLocationID: f.person}); err != nil {
		t.Fatal(err)
	}
	// late is lent with a due date that then lapses.
	if _, err := f.custody.Assign(ctx, late.ID, f.userA, domain.AssignRequest{AssigneeLocationID: f.person, DueAt: new(time.Now().Add(time.Hour))}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE custody_assignments SET due_at = assigned_at + interval '1 millisecond' WHERE item_id = $1::uuid`, late.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	hist, total, _, err := f.custody.History(ctx, drill.ID, 0, 0)
	if err != nil || total != 2 || len(hist) != 2 {
		t.Fatalf("%v %d %d", err, total, len(hist))
	}
	if hist[0].AssigneeName != "Maleta de Alejandro" || !hist[0].Active() || hist[1].Active() || hist[1].AssignedByName != "Ana Operator" {
		t.Errorf("history order/names: %+v", hist)
	}
	if page, total, _, _ := f.custody.History(ctx, drill.ID, 1, 1); total != 2 || len(page) != 1 || page[0].Active() {
		t.Errorf("history paging: %d %+v", total, page)
	}
	if _, _, _, err := f.custody.History(ctx, "00000000-0000-4000-8000-000000000000", 0, 0); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("unknown item: %v", err)
	}

	codes := func(flt domain.ActiveFilter) []string {
		as, _, _, err := f.custody.Active(ctx, flt, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, a := range as {
			out = append(out, a.ItemCode)
		}
		return out
	}
	eq := func(name string, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: %v, want %v", name, got, want)
			return
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: %v, want %v", name, got, want)
				return
			}
		}
	}
	eq("overdue first, then oldest", codes(domain.ActiveFilter{}), "ISK-HER-003", "ISK-HER-001", "ISK-HER-002")
	eq("overdue only", codes(domain.ActiveFilter{OverdueOnly: true}), "ISK-HER-003")
	eq("by assignee", codes(domain.ActiveFilter{AssigneeLocationID: &f.maleta}), "ISK-HER-001")
	eq("q on code", codes(domain.ActiveFilter{Q: "her-002"}), "ISK-HER-002")
	eq("q on description", codes(domain.ActiveFilter{Q: "TALADRO ISK-HER-001"}), "ISK-HER-001")
	eq("q escapes wildcards", codes(domain.ActiveFilter{Q: "%"}))
	if as, total, page, _ := f.custody.Active(ctx, domain.ActiveFilter{}, 2, 2); total != 3 || len(as) != 1 || page.Offset != 2 {
		t.Errorf("active paging: total=%d len=%d", total, len(as))
	}
}
