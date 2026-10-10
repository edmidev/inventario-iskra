package app_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/custody/adapters/memory"
	"github.com/edgar/inventario/backend/internal/custody/app"
	"github.com/edgar/inventario/backend/internal/custody/domain"
)

var t0 = time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

func p64(v int64) *int64 { return &v }

type fixture struct {
	svc   *app.Service
	store *memory.Store
	clock *time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	st := memory.NewStore()
	st.SeedPlace(1, "Bodega", domain.PlaceStorage)
	st.SeedPlace(2, "Alejandro", domain.PlacePerson)
	st.SeedPlace(3, "Maleta", domain.PlaceContainer)
	st.SeedUser("u1", "Ana Operator")
	st.SeedUser("u2", "Omar Operator")
	st.SeedItem(memory.Item{ID: "a1", Code: "ISK-HER-001", Description: "Drill", Kind: domain.KindAsset, LocationID: p64(1)})
	st.SeedItem(memory.Item{ID: "a2", Code: "ISK-HER-002", Description: "Saw", Kind: domain.KindAsset, LocationID: p64(1)})
	st.SeedItem(memory.Item{ID: "c1", Code: "ISK-CON-001", Description: "Gloves", Kind: domain.KindConsumable})
	now := t0
	f := fixture{store: st, clock: &now}
	f.svc = app.NewService(st, func() time.Time { return *f.clock })
	return f
}

func TestAssignMovesItemAndRecordsOrigin(t *testing.T) {
	f := newFixture(t)
	a, err := f.svc.Assign(t.Context(), "a1", "u1", domain.AssignRequest{AssigneeLocationID: 2, Note: "field job"})
	if err != nil {
		t.Fatal(err)
	}
	if a.AssigneeName != "Alejandro" || a.AssignedByName != "Ana Operator" || a.ItemCode != "ISK-HER-001" || !a.Active() {
		t.Errorf("%+v", a)
	}
	if a.OriginLocationID == nil || *a.OriginLocationID != 1 {
		t.Errorf("origin = %v", a.OriginLocationID)
	}
	if loc := f.store.ItemLocation("a1"); loc == nil || *loc != 2 {
		t.Errorf("item location = %v, want assignee 2", loc)
	}
}

func TestSecondAssignIsRejectedAndChangesNothing(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Assign(t.Context(), "a1", "u1", domain.AssignRequest{AssigneeLocationID: 2}); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.Assign(t.Context(), "a1", "u2", domain.AssignRequest{AssigneeLocationID: 3})
	if !errors.Is(err, domain.ErrAlreadyAssigned) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.store.Rows()); n != 1 {
		t.Errorf("rows = %d", n)
	}
	if loc := f.store.ItemLocation("a1"); *loc != 2 {
		t.Errorf("location changed by a rejected assign: %v", *loc)
	}
}

func TestAssignRejections(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	if _, err := f.svc.Assign(ctx, "c1", "u1", domain.AssignRequest{AssigneeLocationID: 2}); !errors.Is(err, domain.ErrNotApplicable) {
		t.Errorf("consumable: %v", err)
	}
	if _, err := f.svc.Assign(ctx, "nope", "u1", domain.AssignRequest{AssigneeLocationID: 2}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("missing: %v", err)
	}
	f.store.SetStatus("a2", "in_repair")
	if _, err := f.svc.Assign(ctx, "a2", "u1", domain.AssignRequest{AssigneeLocationID: 2}); !errors.Is(err, domain.ErrItemNotAvailable) {
		t.Errorf("in repair: %v", err)
	}
	if _, err := f.svc.Assign(ctx, "a1", "u1", domain.AssignRequest{AssigneeLocationID: 1}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("storage assignee: %v", err)
	}
	past := t0.Add(-time.Hour)
	if _, err := f.svc.Assign(ctx, "a1", "u1", domain.AssignRequest{AssigneeLocationID: 2, DueAt: &past}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("past due: %v", err)
	}
	if n := len(f.store.Rows()); n != 0 {
		t.Errorf("rejections wrote %d rows", n)
	}
}

func TestReturnRestoresOriginAndKeepsHistory(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	if _, err := f.svc.Assign(ctx, "a1", "u1", domain.AssignRequest{AssigneeLocationID: 2}); err != nil {
		t.Fatal(err)
	}
	*f.clock = t0.Add(48 * time.Hour)
	a, err := f.svc.Return(ctx, "a1", "u2", domain.ReturnRequest{Note: "all good"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Active() || a.ReturnedByName == nil || *a.ReturnedByName != "Omar Operator" || a.ReturnNote != "all good" {
		t.Errorf("%+v", a)
	}
	if loc := f.store.ItemLocation("a1"); loc == nil || *loc != 1 {
		t.Errorf("location = %v, want origin 1", loc)
	}
	if _, err := f.svc.Return(ctx, "a1", "u2", domain.ReturnRequest{}); !errors.Is(err, domain.ErrNotAssigned) {
		t.Errorf("second return: %v", err)
	}
	// The item can be lent again and history keeps both loans, newest first.
	*f.clock = t0.Add(72 * time.Hour)
	if _, err := f.svc.Assign(ctx, "a1", "u1", domain.AssignRequest{AssigneeLocationID: 3}); err != nil {
		t.Fatal(err)
	}
	hist, total, _, err := f.svc.History(ctx, "a1", 0, 0)
	if err != nil || total != 2 || hist[0].AssigneeName != "Maleta" || hist[1].Active() {
		t.Fatalf("%v %d %+v", err, total, hist)
	}
}

func TestReturnToChosenLocationAndWhileInRepair(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	if _, err := f.svc.Assign(ctx, "a1", "u1", domain.AssignRequest{AssigneeLocationID: 2}); err != nil {
		t.Fatal(err)
	}
	f.store.SetStatus("a1", "in_repair") // broke while on loan
	if _, err := f.svc.Return(ctx, "a1", "u1", domain.ReturnRequest{ReturnLocationID: p64(99)}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("unknown return location: %v", err)
	}
	if _, err := f.svc.Return(ctx, "a1", "u1", domain.ReturnRequest{ReturnLocationID: p64(3)}); err != nil {
		t.Fatal(err)
	}
	if loc := f.store.ItemLocation("a1"); *loc != 3 {
		t.Errorf("location = %v", *loc)
	}
}

func TestActiveListingOrdersOverdueFirstThenOldest(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	soon := t0.Add(24 * time.Hour)
	// a2 lent first without due date, a1 lent later with a due date that will lapse.
	if _, err := f.svc.Assign(ctx, "a2", "u1", domain.AssignRequest{AssigneeLocationID: 3}); err != nil {
		t.Fatal(err)
	}
	*f.clock = t0.Add(time.Hour)
	if _, err := f.svc.Assign(ctx, "a1", "u1", domain.AssignRequest{AssigneeLocationID: 2, DueAt: &soon}); err != nil {
		t.Fatal(err)
	}
	list := func(flt domain.ActiveFilter) []string {
		as, _, _, err := f.svc.Active(ctx, flt, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var codes []string
		for _, a := range as {
			codes = append(codes, a.ItemCode)
		}
		return codes
	}
	if got := list(domain.ActiveFilter{}); got[0] != "ISK-HER-002" { // oldest first while nothing is overdue
		t.Errorf("before due: %v", got)
	}
	*f.clock = t0.Add(72 * time.Hour)
	if got := list(domain.ActiveFilter{}); got[0] != "ISK-HER-001" {
		t.Errorf("overdue must come first: %v", got)
	}
	if got := list(domain.ActiveFilter{OverdueOnly: true}); len(got) != 1 || got[0] != "ISK-HER-001" {
		t.Errorf("overdue filter: %v", got)
	}
	if got := list(domain.ActiveFilter{AssigneeLocationID: p64(3)}); len(got) != 1 || got[0] != "ISK-HER-002" {
		t.Errorf("assignee filter: %v", got)
	}
	if got := list(domain.ActiveFilter{Q: " saw "}); len(got) != 1 || got[0] != "ISK-HER-002" {
		t.Errorf("q filter: %v", got)
	}
}

func TestPagingValidationAndMissingItemHistory(t *testing.T) {
	f := newFixture(t)
	if _, _, _, err := f.svc.History(t.Context(), "a1", 500, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Error(err)
	}
	if _, _, _, err := f.svc.History(t.Context(), "nope", 0, 0); !errors.Is(err, domain.ErrItemNotFound) {
		t.Error(err)
	}
	if _, _, _, err := f.svc.Active(t.Context(), domain.ActiveFilter{}, 0, -1); !errors.Is(err, domain.ErrInvalidInput) {
		t.Error(err)
	}
}

func TestConcurrentAssignsYieldExactlyOne(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicts := 0, 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Assign(t.Context(), "a1", "u1", domain.AssignRequest{AssigneeLocationID: 2})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrAlreadyAssigned):
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
