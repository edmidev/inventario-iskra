package app_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/edgar/inventario/backend/internal/stock/adapters/memory"
	"github.com/edgar/inventario/backend/internal/stock/app"
	"github.com/edgar/inventario/backend/internal/stock/domain"
)

func p64(v int64) *int64 { return &v }

func setup() (*app.Service, *memory.Store) {
	st := memory.NewStore()
	st.SeedLocation(1, "Warehouse")
	st.SeedLocation(2, "Workshop")
	st.SeedUser("u1", "Ana")
	st.SeedItem(memory.Item{ID: "c1", Code: "ISK-CON-001", Description: "Gloves", Kind: domain.KindConsumable, Unit: "pair", StockCurrent: 10, StockMin: 4, LocationID: p64(1)})
	st.SeedItem(memory.Item{ID: "a1", Code: "ISK-HER-001", Description: "Drill", Kind: domain.KindAsset, StockCurrent: 1, LocationID: p64(1)})
	st.SeedItem(memory.Item{ID: "d1", Code: "ISK-CON-002", Description: "Gone", Kind: domain.KindConsumable, Unit: "u", StockCurrent: 3, Deleted: true})
	return app.NewService(st), st
}

func TestRecordUpdatesStockAndLedger(t *testing.T) {
	svc, st := setup()
	ctx := t.Context()

	r, err := svc.Record(ctx, "c1", "u1", domain.Request{Type: domain.TypeOut, Qty: 3})
	if err != nil {
		t.Fatal(err)
	}
	if r.StockCurrent != 7 || r.Movement.Qty != -3 || r.Movement.StockBefore != 10 || r.Movement.StockAfter != 7 {
		t.Fatalf("result: %+v", r)
	}
	if got := st.Item("c1").StockCurrent; got != 7 {
		t.Errorf("stored stock = %d", got)
	}
	if r.Movement.UserID == nil || *r.Movement.UserID != "u1" || r.Movement.UserName == nil || *r.Movement.UserName != "Ana" {
		t.Errorf("actor not recorded: %+v", r.Movement)
	}
}

func TestRejectedMovementWritesNothing(t *testing.T) {
	svc, st := setup()
	ctx := t.Context()
	cases := []struct {
		name   string
		itemID string
		req    domain.Request
		want   error
	}{
		{"insufficient", "c1", domain.Request{Type: domain.TypeOut, Qty: 11}, domain.ErrInsufficientStock},
		{"asset not applicable", "a1", domain.Request{Type: domain.TypeIn, Qty: 1}, domain.ErrNotApplicable},
		{"invalid", "c1", domain.Request{Type: domain.TypeIn, Qty: 0}, domain.ErrInvalidInput},
		{"deleted item", "d1", domain.Request{Type: domain.TypeIn, Qty: 1}, domain.ErrItemNotFound},
		{"unknown item", "zz", domain.Request{Type: domain.TypeIn, Qty: 1}, domain.ErrItemNotFound},
		{"unknown destination", "a1", domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(99)}, domain.ErrInvalidInput},
	}
	for _, tc := range cases {
		if _, err := svc.Record(ctx, tc.itemID, "u1", tc.req); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if len(st.Movements()) != 0 || st.Item("c1").StockCurrent != 10 || st.Item("a1").LocationID == nil || *st.Item("a1").LocationID != 1 {
		t.Error("rejected movements changed state")
	}
}

func TestTransferMovesAssetAndRecordsLocations(t *testing.T) {
	svc, st := setup()
	r, err := svc.Record(t.Context(), "a1", "u1", domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2), Note: "to workshop"})
	if err != nil {
		t.Fatal(err)
	}
	m := r.Movement
	if m.Qty != 0 || m.FromLocationID == nil || *m.FromLocationID != 1 || *m.ToLocationID != 2 || m.StockAfter != 1 {
		t.Errorf("movement: %+v", m)
	}
	if loc := st.Item("a1").LocationID; loc == nil || *loc != 2 || r.LocationID == nil || *r.LocationID != 2 {
		t.Errorf("location not updated")
	}
}

func TestHistoryNewestFirstFilterAndPaging(t *testing.T) {
	svc, _ := setup()
	ctx := t.Context()
	for _, req := range []domain.Request{
		{Type: domain.TypeIn, Qty: 1}, {Type: domain.TypeOut, Qty: 2}, {Type: domain.TypeIn, Qty: 3},
	} {
		if _, err := svc.Record(ctx, "c1", "u1", req); err != nil {
			t.Fatal(err)
		}
	}
	ms, total, _, err := svc.History(ctx, "c1", "", 2, 0)
	if err != nil || total != 3 || len(ms) != 2 || ms[0].Qty != 3 || ms[1].Qty != -2 {
		t.Fatalf("page 1: %+v total=%d err=%v", ms, total, err)
	}
	ms, _, _, _ = svc.History(ctx, "c1", "", 2, 2)
	if len(ms) != 1 || ms[0].Qty != 1 {
		t.Errorf("page 2: %+v", ms)
	}
	ms, total, _, _ = svc.History(ctx, "c1", domain.TypeIn, 25, 0)
	if total != 2 || len(ms) != 2 {
		t.Errorf("type filter: %+v total=%d", ms, total)
	}
	if _, _, _, err := svc.History(ctx, "c1", "bogus", 25, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad type: %v", err)
	}
	if _, _, _, err := svc.History(ctx, "c1", "", 101, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("limit 101: %v", err)
	}
	if _, _, _, err := svc.History(ctx, "d1", "", 25, 0); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("deleted item history: %v", err)
	}
}

func TestAlertsOrderedBySeverity(t *testing.T) {
	svc, st := setup()
	st.SeedItem(memory.Item{ID: "c2", Code: "ISK-CON-010", Description: "Tape", Kind: domain.KindConsumable, Unit: "roll", StockCurrent: 0, StockMin: 5})
	st.SeedItem(memory.Item{ID: "c3", Code: "ISK-CON-011", Description: "Glue", Kind: domain.KindConsumable, Unit: "u", StockCurrent: 2, StockMin: 3})
	st.SeedItem(memory.Item{ID: "c4", Code: "ISK-CON-012", Description: "Ok", Kind: domain.KindConsumable, Unit: "u", StockCurrent: 9, StockMin: 3})
	st.SeedItem(memory.Item{ID: "c5", Code: "ISK-CON-013", Description: "No min", Kind: domain.KindConsumable, Unit: "u", StockCurrent: 0, StockMin: 0})
	ctx := t.Context()
	if _, err := svc.Record(ctx, "c1", "u1", domain.Request{Type: domain.TypeOut, Qty: 6}); err != nil { // 4 <= min 4
		t.Fatal(err)
	}
	as, total, _, err := svc.Alerts(ctx, 25, 0)
	if err != nil || total != 3 {
		t.Fatalf("alerts: %+v total=%d err=%v", as, total, err)
	}
	want := []string{"ISK-CON-010", "ISK-CON-011", "ISK-CON-001"} // shortages -5, -1, 0
	for i, c := range want {
		if as[i].Code != c {
			t.Errorf("alerts[%d] = %s, want %s", i, as[i].Code, c)
		}
	}
	if as[2].LocationName == nil || *as[2].LocationName != "Warehouse" {
		t.Errorf("location name missing: %+v", as[2])
	}
}

func TestConcurrentOutNeverOversells(t *testing.T) {
	svc, st := setup()
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Record(t.Context(), "c1", "u1", domain.Request{Type: domain.TypeOut, Qty: 1}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if !errors.Is(err, domain.ErrInsufficientStock) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok != 10 || st.Item("c1").StockCurrent != 0 {
		t.Errorf("successes=%d stock=%d, want 10 and 0", ok, st.Item("c1").StockCurrent)
	}
}

func TestTransferRejectedWhileOnLoan(t *testing.T) {
	svc, st := setup()
	st.SeedItem(memory.Item{ID: "a-loan", Kind: domain.KindAsset, StockCurrent: 1, LocationID: p64(1), OnLoan: true})
	_, err := svc.Record(t.Context(), "a-loan", "u1", domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2)})
	if !errors.Is(err, domain.ErrItemOnLoan) {
		t.Fatalf("err = %v, want ErrItemOnLoan", err)
	}
	if loc := st.Item("a-loan").LocationID; loc == nil || *loc != 1 || len(st.Movements()) != 0 {
		t.Error("rejected transfer changed state")
	}
}

// multiSetup seeds a consumable stocked in two locations: 6 in the
// warehouse and 4 in the workshop.
func multiSetup() (*app.Service, *memory.Store) {
	svc, st := setup()
	st.SeedLocation(3, "Toolbox")
	st.SeedItem(memory.Item{ID: "m1", Code: "ISK-CON-050", Description: "Screws", Kind: domain.KindConsumable, Unit: "u",
		StockMin: 3, Stocks: map[int64]int{1: 6, 2: 4}})
	return svc, st
}

func TestOutAtOneLocationLeavesTheOthersUntouched(t *testing.T) {
	svc, st := multiSetup()
	r, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeOut, Qty: 4, LocationID: p64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if r.StockCurrent != 6 || len(r.Stocks) != 1 || r.Stocks[0].LocationID != 1 || r.Stocks[0].Quantity != 6 {
		t.Fatalf("result: %+v", r)
	}
	if m := r.Movement; m.LocationID == nil || *m.LocationID != 2 || m.Qty != -4 || m.StockBefore != 10 || m.StockAfter != 6 {
		t.Errorf("movement: %+v", m)
	}
	if it := st.Item("m1"); it.StockCurrent != 6 || it.Stocks[2] != 0 || it.Stocks[1] != 6 {
		t.Errorf("stored: %+v", it)
	}
}

func TestOutBeyondALocationIsRejectedEvenWhenTheTotalCoversIt(t *testing.T) {
	svc, st := multiSetup()
	_, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeOut, Qty: 5, LocationID: p64(2)})
	var ie *domain.InsufficientStockError
	if !errors.As(err, &ie) || ie.Available != 4 || ie.LocationName != "Workshop" {
		t.Fatalf("err = %v", err)
	}
	if len(st.Movements()) != 0 || st.Item("m1").StockCurrent != 10 {
		t.Error("rejected out changed state")
	}
}

func TestAmbiguousLocationIsRejectedAndSingleLocationIsInferred(t *testing.T) {
	svc, st := multiSetup()
	for _, typ := range []domain.MovementType{domain.TypeIn, domain.TypeOut, domain.TypeAdjust} {
		_, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: typ, Qty: 1, Note: "x"})
		if !errors.Is(err, domain.ErrLocationRequired) {
			t.Errorf("%s: err = %v, want ErrLocationRequired", typ, err)
		}
	}
	if len(st.Movements()) != 0 {
		t.Error("rejected requests wrote movements")
	}
	// c1 has a single stock row: the location is inferred.
	r, err := svc.Record(t.Context(), "c1", "u1", domain.Request{Type: domain.TypeIn, Qty: 2})
	if err != nil || r.Movement.LocationID == nil || *r.Movement.LocationID != 1 || r.StockCurrent != 12 {
		t.Fatalf("inferred: %+v err=%v", r, err)
	}
}

func TestAdjustSetsTheCountAtOneLocation(t *testing.T) {
	svc, _ := multiSetup()
	r, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeAdjust, Qty: 9, LocationID: p64(2), Note: "recount"})
	if err != nil {
		t.Fatal(err)
	}
	if r.StockCurrent != 15 || r.Movement.Qty != 5 || len(r.Stocks) != 2 {
		t.Fatalf("result: %+v", r)
	}
	if _, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeAdjust, Qty: 9, LocationID: p64(2)}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("adjust without note: %v", err)
	}
}

func TestPartialTransferKeepsTheTotal(t *testing.T) {
	svc, st := multiSetup()
	r, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeTransfer, Qty: 2, FromLocationID: p64(1), ToLocationID: p64(3)})
	if err != nil {
		t.Fatal(err)
	}
	m := r.Movement
	if r.StockCurrent != 10 || m.Qty != 0 || m.MovedQty != 2 || *m.FromLocationID != 1 || *m.ToLocationID != 3 || m.LocationID != nil {
		t.Fatalf("result: %+v", r)
	}
	it := st.Item("m1")
	if it.Stocks[1] != 4 || it.Stocks[2] != 4 || it.Stocks[3] != 2 {
		t.Errorf("stocks: %v", it.Stocks)
	}
	// Moving everything drops the source row.
	if _, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeTransfer, Qty: 4, FromLocationID: p64(1), ToLocationID: p64(2)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Item("m1").Stocks[1]; ok {
		t.Errorf("empty location kept a row: %v", st.Item("m1").Stocks)
	}
	if _, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeTransfer, Qty: 9, FromLocationID: p64(2), ToLocationID: p64(3)}); !errors.Is(err, domain.ErrInsufficientStock) {
		t.Errorf("over-transfer: %v", err)
	}
}

func TestInOnNeverStockedItemGoesToTheDefaultLocationThenSticks(t *testing.T) {
	svc, st := setup()
	st.SeedItem(memory.Item{ID: "n1", Code: "ISK-CON-060", Description: "Fresh", Kind: domain.KindConsumable, Unit: "u"})
	r, err := svc.Record(t.Context(), "n1", "u1", domain.Request{Type: domain.TypeIn, Qty: 3})
	if err != nil || len(r.Stocks) != 1 || r.Stocks[0].LocationName != "Sin ubicación" || r.Movement.LocationID == nil {
		t.Fatalf("default location: %+v err=%v", r, err)
	}
	// Drained and refilled without a location: back to where it was.
	if _, err := svc.Record(t.Context(), "n1", "u1", domain.Request{Type: domain.TypeOut, Qty: 3}); err != nil {
		t.Fatal(err)
	}
	r2, err := svc.Record(t.Context(), "n1", "u1", domain.Request{Type: domain.TypeIn, Qty: 1})
	if err != nil || *r2.Movement.LocationID != *r.Movement.LocationID {
		t.Fatalf("refill: %+v err=%v", r2, err)
	}
}

func TestAlertsCarryThePerLocationBreakdown(t *testing.T) {
	svc, _ := multiSetup()
	if _, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeOut, Qty: 4, LocationID: p64(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeOut, Qty: 3, LocationID: p64(1)}); err != nil {
		t.Fatal(err)
	}
	as, _, _, err := svc.Alerts(t.Context(), 25, 0)
	if err != nil || len(as) != 1 || as[0].Code != "ISK-CON-050" {
		t.Fatalf("alerts: %+v err=%v", as, err)
	}
	if as[0].StockCurrent != 3 || len(as[0].Stocks) != 1 || as[0].LocationName == nil || *as[0].LocationName != "Warehouse" {
		t.Errorf("alert: %+v", as[0])
	}
}

func TestConcurrentOutAtOneLocationNeverOversells(t *testing.T) {
	svc, st := multiSetup()
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Record(t.Context(), "m1", "u1", domain.Request{Type: domain.TypeOut, Qty: 1, LocationID: p64(2)}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if !errors.Is(err, domain.ErrInsufficientStock) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if it := st.Item("m1"); ok != 4 || it.StockCurrent != 6 || it.Stocks[1] != 6 {
		t.Errorf("successes=%d item=%+v", ok, it)
	}
}
