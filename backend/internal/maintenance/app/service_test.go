package app_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/maintenance/adapters/memory"
	"github.com/edgar/inventario/backend/internal/maintenance/app"
	"github.com/edgar/inventario/backend/internal/maintenance/domain"
)

var t0 = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func ip(i int) *int         { return &i }
func sp(s string) *string   { return &s }
func fp(f float64) *float64 { return &f }
func tp(t time.Time) *time.Time {
	return &t
}

type fixture struct {
	svc   *app.Service
	store *memory.Store
	clock *time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	st := memory.NewStore()
	st.SeedUser("u1", "Ana Operator")
	st.SeedUser("u2", "Omar Operator")
	created := t0.Add(-100 * 24 * time.Hour)
	st.SeedItem(memory.Item{ID: "a1", Code: "ISK-HER-001", Description: "Drill", CategoryID: 1, CategoryName: "Tools", Kind: domain.ItemAsset, CreatedAt: created})
	st.SeedItem(memory.Item{ID: "a2", Code: "ISK-HER-002", Description: "Saw", CategoryID: 1, CategoryName: "Tools", Kind: domain.ItemAsset, IntervalDays: ip(30), CreatedAt: created})
	st.SeedItem(memory.Item{ID: "a3", Code: "ISK-VEH-001", Description: "Truck", CategoryID: 2, CategoryName: "Vehicles", Kind: domain.ItemAsset, IntervalDays: ip(365), LastMaintenanceAt: tp(t0.Add(-10 * 24 * time.Hour)), CreatedAt: created})
	st.SeedItem(memory.Item{ID: "c1", Code: "ISK-CON-001", Description: "Gloves", Kind: domain.ItemConsumable, CreatedAt: created})
	now := t0
	f := fixture{store: st, clock: &now}
	f.svc = app.NewService(st, func() time.Time { return *f.clock })
	return f
}

func (f fixture) fail(t *testing.T, id string, status domain.ItemStatus) domain.Record {
	t.Helper()
	r, err := f.svc.Create(t.Context(), id, "u1", domain.CreateRequest{Kind: domain.KindCorrective, Description: "Motor burnt", ItemStatus: status})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReportFailureOpensRecordAndMarksItemInoperative(t *testing.T) {
	f := newFixture(t)
	r := f.fail(t, "a1", "")
	if r.Status != domain.StatusOpen || r.Kind != domain.KindCorrective || r.ItemCode != "ISK-HER-001" || r.ReportedByName != "Ana Operator" || r.ItemStatus != domain.ItemInoperative {
		t.Errorf("%+v", r)
	}
	if got := f.store.Item("a1").Status; got != domain.ItemInoperative {
		t.Errorf("item status = %s", got)
	}
}

func TestSecondOpenFailureIsRejectedAndChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.fail(t, "a1", "")
	_, err := f.svc.Create(t.Context(), "a1", "u2", domain.CreateRequest{Kind: domain.KindCorrective, Description: "again", ItemStatus: domain.ItemInRepair})
	if !errors.Is(err, domain.ErrOpenFailureExists) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.store.Records()); n != 1 || f.store.Item("a1").Status != domain.ItemInoperative {
		t.Errorf("rejected report changed state: %d records, status %s", n, f.store.Item("a1").Status)
	}
}

func TestFailureRepairLifecycleDrivesItemStatus(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	r := f.fail(t, "a1", "")
	r, err := f.svc.Update(ctx, r.ID, domain.UpdateRequest{SetDestination: true, Destination: sp("Taller de Reparación Interno"), ItemStatus: domain.ItemInRepair})
	if err != nil || *r.Destination != "Taller de Reparación Interno" || r.ItemStatus != domain.ItemInRepair {
		t.Fatalf("%+v %v", r, err)
	}
	if got := f.store.Item("a1").Status; got != domain.ItemInRepair {
		t.Fatalf("item = %s", got)
	}
	*f.clock = t0.Add(72 * time.Hour)
	r, err = f.svc.Close(ctx, r.ID, "u2", domain.CloseRequest{Cost: fp(120.5), ResolutionNote: "Replaced motor"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.StatusClosed || r.ClosedByName == nil || *r.ClosedByName != "Omar Operator" || *r.Cost != 120.5 || r.ItemStatus != domain.ItemFunctional {
		t.Errorf("%+v", r)
	}
	it := f.store.Item("a1")
	if it.Status != domain.ItemFunctional || it.LastMaintenanceAt == nil || !it.LastMaintenanceAt.Equal(*r.ClosedAt) {
		t.Errorf("item after close: %+v", it)
	}
	if _, err := f.svc.Close(ctx, r.ID, "u2", domain.CloseRequest{}); !errors.Is(err, domain.ErrRecordClosed) {
		t.Errorf("second close: %v", err)
	}
	if _, err := f.svc.Update(ctx, r.ID, domain.UpdateRequest{Description: sp("x")}); !errors.Is(err, domain.ErrRecordClosed) {
		t.Errorf("update closed: %v", err)
	}
	// A new failure can be reported once the previous one is closed.
	f.fail(t, "a1", domain.ItemInRepair)
}

func TestCloseAsInoperativeKeepsLastMaintenance(t *testing.T) {
	f := newFixture(t)
	r := f.fail(t, "a1", "")
	if _, err := f.svc.Close(t.Context(), r.ID, "u1", domain.CloseRequest{ItemStatusAfter: domain.ItemInoperative}); err != nil {
		t.Fatal(err)
	}
	if it := f.store.Item("a1"); it.Status != domain.ItemInoperative || it.LastMaintenanceAt != nil {
		t.Errorf("%+v", it)
	}
}

func TestPreventiveUpdatesLastMaintenanceWithoutChangingStatus(t *testing.T) {
	f := newFixture(t)
	f.store.SeedItem(memory.Item{ID: "a4", Code: "ISK-HER-004", Description: "Broken", Kind: domain.ItemAsset, Status: domain.ItemInRepair, IntervalDays: ip(60), CreatedAt: t0})
	performed := t0.Add(-time.Hour)
	r, err := f.svc.Create(t.Context(), "a4", "u1", domain.CreateRequest{Kind: domain.KindPreventive, Description: "Greasing", PerformedAt: &performed, Cost: fp(10)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.StatusClosed || r.NextDueAt == nil || !r.NextDueAt.Equal(domain.AddDays(performed, 60)) || r.ClosedByName == nil {
		t.Errorf("%+v", r)
	}
	it := f.store.Item("a4")
	if it.Status != domain.ItemInRepair || it.LastMaintenanceAt == nil || !it.LastMaintenanceAt.Equal(performed) {
		t.Errorf("item: %+v", it)
	}
}

func TestRejectionsWriteNothing(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	cases := []struct {
		name   string
		itemID string
		req    domain.CreateRequest
		want   error
	}{
		{"consumable corrective", "c1", domain.CreateRequest{Kind: domain.KindCorrective, Description: "x"}, domain.ErrNotApplicable},
		{"consumable preventive", "c1", domain.CreateRequest{Kind: domain.KindPreventive, Description: "x"}, domain.ErrNotApplicable},
		{"unknown item", "nope", domain.CreateRequest{Kind: domain.KindCorrective, Description: "x"}, domain.ErrItemNotFound},
		{"bad kind", "a1", domain.CreateRequest{Kind: "other", Description: "x"}, domain.ErrInvalidInput},
		{"blank description", "a1", domain.CreateRequest{Kind: domain.KindCorrective}, domain.ErrInvalidInput},
	}
	for _, tc := range cases {
		if _, err := f.svc.Create(ctx, tc.itemID, "u1", tc.req); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := f.svc.Update(ctx, 99, domain.UpdateRequest{Description: sp("x")}); !errors.Is(err, domain.ErrRecordNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if _, err := f.svc.Close(ctx, 99, "u1", domain.CloseRequest{}); !errors.Is(err, domain.ErrRecordNotFound) {
		t.Errorf("close missing: %v", err)
	}
	if n := len(f.store.Records()); n != 0 {
		t.Errorf("rejections wrote %d records", n)
	}
}

func TestSoftDeletedItemIsRejected(t *testing.T) {
	f := newFixture(t)
	r := f.fail(t, "a1", "")
	it := f.store.Item("a1")
	it.Deleted = true
	f.store.SeedItem(it)
	if _, err := f.svc.Create(t.Context(), "a1", "u1", domain.CreateRequest{Kind: domain.KindPreventive, Description: "x"}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("create: %v", err)
	}
	if _, err := f.svc.Close(t.Context(), r.ID, "u1", domain.CloseRequest{}); !errors.Is(err, domain.ErrRecordNotFound) {
		t.Errorf("close: %v", err)
	}
	if _, _, _, err := f.svc.History(t.Context(), "a1", 0, 0); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("history: %v", err)
	}
}

func TestHistoryNewestFirstAndPaged(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	for i, d := range []time.Duration{-72, -24, -48} {
		at := t0.Add(d * time.Hour)
		if _, err := f.svc.Create(ctx, "a1", "u1", domain.CreateRequest{Kind: domain.KindPreventive, Description: string(rune('A' + i)), PerformedAt: &at}); err != nil {
			t.Fatal(err)
		}
	}
	rs, total, _, err := f.svc.History(ctx, "a1", 2, 0)
	if err != nil || total != 3 || len(rs) != 2 || rs[0].Description != "B" || rs[1].Description != "C" {
		t.Fatalf("%v %d %+v", err, total, rs)
	}
	rs, _, _, _ = f.svc.History(ctx, "a1", 2, 2)
	if len(rs) != 1 || rs[0].Description != "A" {
		t.Errorf("offset page: %+v", rs)
	}
	if _, _, _, err := f.svc.History(ctx, "a1", 500, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("limit: %v", err)
	}
}

func TestListFiltersAndOrdersOpenFirst(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	at := t0.Add(-time.Hour)
	if _, err := f.svc.Create(ctx, "a2", "u1", domain.CreateRequest{Kind: domain.KindPreventive, Description: "Blade sharpening", PerformedAt: &at}); err != nil {
		t.Fatal(err)
	}
	*f.clock = t0.Add(-48 * time.Hour)
	r := f.fail(t, "a1", domain.ItemInRepair)
	if _, err := f.svc.Update(ctx, r.ID, domain.UpdateRequest{SetDestination: true, Destination: sp("Espera de Repuestos")}); err != nil {
		t.Fatal(err)
	}
	*f.clock = t0
	codes := func(flt domain.ListFilter) []string {
		rs, _, _, err := f.svc.List(ctx, flt, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rs {
			out = append(out, r.ItemCode)
		}
		return out
	}
	if got := codes(domain.ListFilter{}); len(got) != 2 || got[0] != "ISK-HER-001" { // open first even though older
		t.Errorf("order: %v", got)
	}
	if got := codes(domain.ListFilter{Status: domain.StatusClosed}); len(got) != 1 || got[0] != "ISK-HER-002" {
		t.Errorf("status: %v", got)
	}
	if got := codes(domain.ListFilter{Kind: domain.KindCorrective}); len(got) != 1 || got[0] != "ISK-HER-001" {
		t.Errorf("kind: %v", got)
	}
	if got := codes(domain.ListFilter{Destination: " espera de repuestos "}); len(got) != 1 || got[0] != "ISK-HER-001" {
		t.Errorf("destination: %v", got)
	}
	if got := codes(domain.ListFilter{Q: "sharpen"}); len(got) != 1 || got[0] != "ISK-HER-002" {
		t.Errorf("q on record description: %v", got)
	}
	if got := codes(domain.ListFilter{Q: "drill"}); len(got) != 1 || got[0] != "ISK-HER-001" {
		t.Errorf("q on item description: %v", got)
	}
	for _, bad := range []domain.ListFilter{{Status: "x"}, {Kind: "x"}, {ItemID: "a1"}} {
		if _, _, _, err := f.svc.List(ctx, bad, 0, 0); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestDueListsOverdueAndUpcomingMostOverdueFirst(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	// a2: created 100d ago, 30d interval -> due 70d ago. a3: last 10d ago, 365d interval -> due in 355d.
	list := func(flt domain.DueFilter) ([]string, bool) {
		items, _, _, now, err := f.svc.Due(ctx, flt, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var codes []string
		overdue := false
		for _, d := range items {
			codes = append(codes, d.Code)
			overdue = overdue || d.Overdue(now)
		}
		return codes, overdue
	}
	if got, od := list(domain.DueFilter{}); len(got) != 1 || got[0] != "ISK-HER-002" || !od {
		t.Errorf("default window: %v overdue=%v", got, od)
	}
	if got, _ := list(domain.DueFilter{WithinDays: 365}); len(got) != 2 || got[0] != "ISK-HER-002" || got[1] != "ISK-VEH-001" {
		t.Errorf("365 window: %v", got)
	}
	if got, _ := list(domain.DueFilter{WithinDays: 365, CategoryID: func() *int64 { v := int64(2); return &v }()}); len(got) != 1 || got[0] != "ISK-VEH-001" {
		t.Errorf("category: %v", got)
	}
	if got, _ := list(domain.DueFilter{WithinDays: 365, Q: "truck"}); len(got) != 1 {
		t.Errorf("q: %v", got)
	}
	// Servicing a2 pushes it out of the overdue list.
	if _, err := f.svc.Create(ctx, "a2", "u1", domain.CreateRequest{Kind: domain.KindPreventive, Description: "Service"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := list(domain.DueFilter{}); len(got) != 0 {
		t.Errorf("after service: %v", got)
	}
	if _, _, _, _, err := f.svc.Due(ctx, domain.DueFilter{WithinDays: 366}, 0, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("within_days: %v", err)
	}
	if _, _, _, _, err := f.svc.Due(ctx, domain.DueFilter{WithinDays: -1}, 0, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("within_days negative: %v", err)
	}
}

func TestConcurrentFailureReportsYieldExactlyOne(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicts := 0, 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Create(t.Context(), "a1", "u1", domain.CreateRequest{Kind: domain.KindCorrective, Description: "x"})
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
	if ok != 1 || conflicts != 19 {
		t.Errorf("ok=%d conflicts=%d", ok, conflicts)
	}
}
