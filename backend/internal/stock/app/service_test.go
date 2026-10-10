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
