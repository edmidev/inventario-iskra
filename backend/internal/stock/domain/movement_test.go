package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/edgar/inventario/backend/internal/stock/domain"
)

func p64(v int64) *int64 { return &v }

var locs = map[int64]string{1: "Warehouse", 2: "Workshop", 3: "Toolbox"}

func stocks(pairs ...int) []domain.LocationStock {
	var out []domain.LocationStock
	for i := 0; i < len(pairs); i += 2 {
		id := int64(pairs[i])
		out = append(out, domain.LocationStock{LocationID: id, LocationName: locs[id], Quantity: pairs[i+1]})
	}
	return out
}

func cons(total int, pairs ...int) domain.ItemState {
	return domain.ItemState{Kind: domain.KindConsumable, StockCurrent: total, Stocks: stocks(pairs...), Locations: locs}
}

// change is the expected absolute quantity at a location.
type change struct {
	loc int64
	qty int
}

func TestDecide(t *testing.T) {
	one := cons(10, 1, 10)      // all 10 at Warehouse
	two := cons(10, 1, 6, 2, 4) // 6 Warehouse + 4 Workshop
	empty := cons(0)
	asset := domain.ItemState{Kind: domain.KindAsset, StockCurrent: 1, LocationID: p64(1), Locations: locs}

	tests := []struct {
		name      string
		state     domain.ItemState
		req       domain.Request
		wantErr   error
		wantField string
		wantCode  string // "location_required" / "insufficient"
		qty       int
		after     int
		moved     int
		moveLoc   *int64 // movement.location_id
		changes   []change
		def       bool
		assetLoc  *int64
	}{
		// in
		{name: "in at explicit location", state: two, req: domain.Request{Type: domain.TypeIn, Qty: 5, LocationID: p64(2)}, qty: 5, after: 15, moveLoc: p64(2), changes: []change{{2, 9}}},
		{name: "in at a location without stock yet", state: two, req: domain.Request{Type: domain.TypeIn, Qty: 5, LocationID: p64(3)}, qty: 5, after: 15, moveLoc: p64(3), changes: []change{{3, 5}}},
		{name: "in infers the only stocked location", state: one, req: domain.Request{Type: domain.TypeIn, Qty: 5}, qty: 5, after: 15, moveLoc: p64(1), changes: []change{{1, 15}}},
		{name: "in ambiguous without location", state: two, req: domain.Request{Type: domain.TypeIn, Qty: 5}, wantErr: domain.ErrLocationRequired, wantField: "location_id"},
		{name: "in on never-stocked item uses the default location", state: empty, req: domain.Request{Type: domain.TypeIn, Qty: 5}, qty: 5, after: 5, def: true, changes: []change{{0, 5}}},
		{name: "in on drained item returns to its last location", state: func() domain.ItemState { s := empty; s.LastLocationID = p64(2); return s }(), req: domain.Request{Type: domain.TypeIn, Qty: 5}, qty: 5, after: 5, moveLoc: p64(2), changes: []change{{2, 5}}},
		{name: "in unknown location", state: one, req: domain.Request{Type: domain.TypeIn, Qty: 1, LocationID: p64(99)}, wantErr: domain.ErrInvalidInput, wantField: "location_id"},
		{name: "in negative qty", state: one, req: domain.Request{Type: domain.TypeIn, Qty: -1}, wantErr: domain.ErrInvalidInput, wantField: "qty"},
		{name: "in overflow of the total", state: two, req: domain.Request{Type: domain.TypeIn, Qty: domain.MaxStock, LocationID: p64(1)}, wantErr: domain.ErrInvalidInput, wantField: "qty"},
		// out
		{name: "out at explicit location", state: two, req: domain.Request{Type: domain.TypeOut, Qty: 4, LocationID: p64(2)}, qty: -4, after: 6, moveLoc: p64(2), changes: []change{{2, 0}}},
		{name: "out partial at location", state: two, req: domain.Request{Type: domain.TypeOut, Qty: 2, LocationID: p64(1)}, qty: -2, after: 8, moveLoc: p64(1), changes: []change{{1, 4}}},
		{name: "out never below zero at a location", state: two, req: domain.Request{Type: domain.TypeOut, Qty: 5, LocationID: p64(2)}, wantErr: domain.ErrInsufficientStock, wantCode: "insufficient"},
		{name: "out cannot borrow from another location", state: two, req: domain.Request{Type: domain.TypeOut, Qty: 7, LocationID: p64(1)}, wantErr: domain.ErrInsufficientStock, wantCode: "insufficient"},
		{name: "out at a location with no stock", state: two, req: domain.Request{Type: domain.TypeOut, Qty: 1, LocationID: p64(3)}, wantErr: domain.ErrInsufficientStock, wantCode: "insufficient"},
		{name: "out infers the only stocked location", state: one, req: domain.Request{Type: domain.TypeOut, Qty: 10}, qty: -10, after: 0, moveLoc: p64(1), changes: []change{{1, 0}}},
		{name: "out ambiguous without location", state: two, req: domain.Request{Type: domain.TypeOut, Qty: 1}, wantErr: domain.ErrLocationRequired, wantField: "location_id"},
		{name: "out with nothing in stock", state: empty, req: domain.Request{Type: domain.TypeOut, Qty: 1}, wantErr: domain.ErrInsufficientStock, wantCode: "insufficient"},
		{name: "out zero qty", state: one, req: domain.Request{Type: domain.TypeOut, Qty: 0}, wantErr: domain.ErrInvalidInput, wantField: "qty"},
		// adjust
		{name: "adjust sets the count at the location", state: two, req: domain.Request{Type: domain.TypeAdjust, Qty: 9, LocationID: p64(2), Note: "count"}, qty: 5, after: 15, moveLoc: p64(2), changes: []change{{2, 9}}},
		{name: "adjust down at the location", state: two, req: domain.Request{Type: domain.TypeAdjust, Qty: 1, LocationID: p64(1), Note: "count"}, qty: -5, after: 5, moveLoc: p64(1), changes: []change{{1, 1}}},
		{name: "adjust to zero removes the location", state: one, req: domain.Request{Type: domain.TypeAdjust, Qty: 0, Note: "empty shelf"}, qty: -10, after: 0, moveLoc: p64(1), changes: []change{{1, 0}}},
		{name: "adjust a location that had none", state: two, req: domain.Request{Type: domain.TypeAdjust, Qty: 3, LocationID: p64(3), Note: "found"}, qty: 3, after: 13, moveLoc: p64(3), changes: []change{{3, 3}}},
		{name: "adjust infers the only location", state: one, req: domain.Request{Type: domain.TypeAdjust, Qty: 7, Note: "count"}, qty: -3, after: 7, moveLoc: p64(1), changes: []change{{1, 7}}},
		{name: "adjust ambiguous without location", state: two, req: domain.Request{Type: domain.TypeAdjust, Qty: 7, Note: "count"}, wantErr: domain.ErrLocationRequired, wantField: "location_id"},
		{name: "adjust without note", state: one, req: domain.Request{Type: domain.TypeAdjust, Qty: 3, Note: "  "}, wantErr: domain.ErrInvalidInput, wantField: "note"},
		{name: "adjust negative", state: one, req: domain.Request{Type: domain.TypeAdjust, Qty: -1, Note: "x"}, wantErr: domain.ErrInvalidInput, wantField: "qty"},
		// consumable transfer
		{name: "transfer partial quantity", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 2, FromLocationID: p64(1), ToLocationID: p64(2)}, qty: 0, after: 10, moved: 2, changes: []change{{1, 4}, {2, 6}}},
		{name: "transfer everything empties the source", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 6, FromLocationID: p64(1), ToLocationID: p64(3)}, qty: 0, after: 10, moved: 6, changes: []change{{1, 0}, {3, 6}}},
		{name: "transfer infers the only source", state: one, req: domain.Request{Type: domain.TypeTransfer, Qty: 4, ToLocationID: p64(2)}, qty: 0, after: 10, moved: 4, changes: []change{{1, 6}, {2, 4}}},
		{name: "transfer ambiguous source", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 1, ToLocationID: p64(3)}, wantErr: domain.ErrLocationRequired, wantField: "from_location_id"},
		{name: "transfer more than available", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 7, FromLocationID: p64(1), ToLocationID: p64(2)}, wantErr: domain.ErrInsufficientStock, wantCode: "insufficient"},
		{name: "transfer same location", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 1, FromLocationID: p64(1), ToLocationID: p64(1)}, wantErr: domain.ErrInvalidInput, wantField: "to_location_id"},
		{name: "transfer without qty", state: two, req: domain.Request{Type: domain.TypeTransfer, FromLocationID: p64(1), ToLocationID: p64(2)}, wantErr: domain.ErrInvalidInput, wantField: "qty"},
		{name: "transfer without destination", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 1, FromLocationID: p64(1)}, wantErr: domain.ErrInvalidInput, wantField: "to_location_id"},
		{name: "transfer to unknown location", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 1, FromLocationID: p64(1), ToLocationID: p64(99)}, wantErr: domain.ErrInvalidInput, wantField: "to_location_id"},
		{name: "transfer from unknown location", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 1, FromLocationID: p64(99), ToLocationID: p64(2)}, wantErr: domain.ErrInvalidInput, wantField: "from_location_id"},
		{name: "transfer nothing in stock", state: empty, req: domain.Request{Type: domain.TypeTransfer, Qty: 1, ToLocationID: p64(2)}, wantErr: domain.ErrInvalidInput, wantField: "from_location_id"},
		{name: "transfer rejects location_id", state: two, req: domain.Request{Type: domain.TypeTransfer, Qty: 1, LocationID: p64(1), FromLocationID: p64(1), ToLocationID: p64(2)}, wantErr: domain.ErrInvalidInput, wantField: "location_id"},
		// assets keep a single location
		{name: "transfer asset", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2)}, qty: 0, after: 1, assetLoc: p64(2)},
		{name: "transfer asset from nowhere", state: domain.ItemState{Kind: domain.KindAsset, StockCurrent: 1, Locations: locs}, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2)}, qty: 0, after: 1, assetLoc: p64(2)},
		{name: "transfer asset on loan", state: domain.ItemState{Kind: domain.KindAsset, StockCurrent: 1, LocationID: p64(1), OnLoan: true, Locations: locs}, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2)}, wantErr: domain.ErrItemOnLoan},
		{name: "transfer asset without destination", state: asset, req: domain.Request{Type: domain.TypeTransfer}, wantErr: domain.ErrInvalidInput, wantField: "to_location_id"},
		{name: "transfer asset to same location", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(1)}, wantErr: domain.ErrInvalidInput, wantField: "to_location_id"},
		{name: "transfer asset to unknown location", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(99)}, wantErr: domain.ErrInvalidInput, wantField: "to_location_id"},
		{name: "transfer asset with qty", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2), Qty: 3}, wantErr: domain.ErrInvalidInput, wantField: "qty"},
		{name: "transfer asset with from_location_id", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2), FromLocationID: p64(1)}, wantErr: domain.ErrInvalidInput, wantField: "from_location_id"},
		{name: "in on asset", state: asset, req: domain.Request{Type: domain.TypeIn, Qty: 1}, wantErr: domain.ErrNotApplicable},
		{name: "out on asset", state: asset, req: domain.Request{Type: domain.TypeOut, Qty: 1}, wantErr: domain.ErrNotApplicable},
		{name: "adjust on asset", state: asset, req: domain.Request{Type: domain.TypeAdjust, Qty: 1, Note: "x"}, wantErr: domain.ErrNotApplicable},
		// generic
		{name: "opening not requestable", state: one, req: domain.Request{Type: domain.TypeOpening, Qty: 1}, wantErr: domain.ErrInvalidInput, wantField: "type"},
		{name: "unknown type", state: one, req: domain.Request{Type: "burn", Qty: 1}, wantErr: domain.ErrInvalidInput, wantField: "type"},
		{name: "destination only for transfer", state: one, req: domain.Request{Type: domain.TypeIn, Qty: 1, ToLocationID: p64(2)}, wantErr: domain.ErrInvalidInput, wantField: "to_location_id"},
		{name: "source only for transfer", state: one, req: domain.Request{Type: domain.TypeOut, Qty: 1, FromLocationID: p64(1)}, wantErr: domain.ErrInvalidInput, wantField: "from_location_id"},
		{name: "note too long", state: one, req: domain.Request{Type: domain.TypeIn, Qty: 1, Note: strings.Repeat("x", 501)}, wantErr: domain.ErrInvalidInput, wantField: "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.Decide(tc.state, tc.req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if tc.wantField != "" {
					var fields map[string]string
					var ve *domain.ValidationError
					var le *domain.LocationRequiredError
					switch {
					case errors.As(err, &ve):
						fields = ve.Fields
					case errors.As(err, &le):
						fields = le.Fields
					}
					if fields[tc.wantField] == "" {
						t.Fatalf("err = %v, want a message on field %q", err, tc.wantField)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			m := got.Movement
			if m.Qty != tc.qty || m.StockAfter != tc.after || got.StockCurrent != tc.after || m.MovedQty != tc.moved {
				t.Errorf("qty/after/moved = %d/%d/%d (state %d), want %d/%d/%d", m.Qty, m.StockAfter, m.MovedQty, got.StockCurrent, tc.qty, tc.after, tc.moved)
			}
			if m.StockBefore != tc.state.StockCurrent || m.StockAfter != m.StockBefore+m.Qty {
				t.Errorf("ledger does not balance: %+v", m)
			}
			if tc.req.Type != domain.TypeTransfer && (m.LocationID == nil) != (tc.moveLoc == nil) {
				t.Errorf("movement location = %v, want %v", m.LocationID, tc.moveLoc)
			} else if tc.moveLoc != nil && *m.LocationID != *tc.moveLoc {
				t.Errorf("movement location = %d, want %d", *m.LocationID, *tc.moveLoc)
			}
			if got.DefaultLocation != tc.def {
				t.Errorf("DefaultLocation = %v, want %v", got.DefaultLocation, tc.def)
			}
			if len(got.Changes) != len(tc.changes) {
				t.Fatalf("changes = %+v, want %+v", got.Changes, tc.changes)
			}
			for i, c := range tc.changes {
				if got.Changes[i].LocationID != c.loc || got.Changes[i].Quantity != c.qty {
					t.Errorf("change %d = %+v, want %+v", i, got.Changes[i], c)
				}
			}
			if tc.state.Kind == domain.KindConsumable && tc.req.Type == domain.TypeTransfer {
				if m.LocationID != nil {
					t.Errorf("transfer must not carry location_id: %v", m.LocationID)
				}
				if m.FromLocationID == nil || m.ToLocationID == nil {
					t.Errorf("transfer needs from/to: %+v", m)
				}
			}
			if tc.state.Kind == domain.KindAsset {
				if (got.LocationID == nil) != (tc.assetLoc == nil) || (tc.assetLoc != nil && *got.LocationID != *tc.assetLoc) {
					t.Errorf("asset location = %v, want %v", got.LocationID, tc.assetLoc)
				}
				if len(got.Changes) != 0 {
					t.Errorf("assets have no per-location changes: %+v", got.Changes)
				}
			}
		})
	}
}

// TestDecideKeepsTheTotalEqualToTheSumOfLocations applies every accepted
// outcome to the per-location state and checks the invariant the database
// also enforces: stock_current is the sum of the locations.
func TestDecideKeepsTheTotalEqualToTheSumOfLocations(t *testing.T) {
	state := cons(10, 1, 6, 2, 4)
	reqs := []domain.Request{
		{Type: domain.TypeIn, Qty: 3, LocationID: p64(2)},
		{Type: domain.TypeOut, Qty: 6, LocationID: p64(1)},
		{Type: domain.TypeTransfer, Qty: 5, FromLocationID: p64(2), ToLocationID: p64(3)},
		{Type: domain.TypeAdjust, Qty: 9, LocationID: p64(3), Note: "count"},
		{Type: domain.TypeTransfer, Qty: 9, FromLocationID: p64(3), ToLocationID: p64(1)},
	}
	for i, req := range reqs {
		out, err := domain.Decide(state, req)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		q := map[int64]int{}
		for _, s := range state.Stocks {
			q[s.LocationID] = s.Quantity
		}
		for _, c := range out.Changes {
			q[c.LocationID] = c.Quantity
		}
		var next []domain.LocationStock
		sum := 0
		for id := int64(1); id <= 3; id++ {
			if q[id] > 0 {
				next = append(next, domain.LocationStock{LocationID: id, LocationName: locs[id], Quantity: q[id]})
				sum += q[id]
			}
		}
		if sum != out.StockCurrent {
			t.Fatalf("step %d: total %d != sum of locations %d", i, out.StockCurrent, sum)
		}
		state = domain.ItemState{Kind: domain.KindConsumable, StockCurrent: out.StockCurrent, Stocks: next, Locations: locs}
	}
}

func TestInsufficientStockNamesTheLocation(t *testing.T) {
	_, err := domain.Decide(cons(6, 1, 2, 2, 4), domain.Request{Type: domain.TypeOut, Qty: 5, LocationID: p64(1)})
	var ie *domain.InsufficientStockError
	if !errors.As(err, &ie) || ie.Available != 2 || ie.Requested != 5 || ie.LocationName != "Warehouse" || ie.LocationID == nil || *ie.LocationID != 1 {
		t.Fatalf("err = %#v", err)
	}
}

func TestAlertShortage(t *testing.T) {
	if got := (domain.Alert{StockCurrent: 1, StockMin: 5}).Shortage(); got != -4 {
		t.Errorf("shortage = %d", got)
	}
}

func TestNormalizePage(t *testing.T) {
	p, err := domain.NormalizePage(0, 0)
	if err != nil || p.Limit != 25 {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	for _, c := range [][2]int{{101, 0}, {-1, 0}, {10, -1}} {
		if _, err := domain.NormalizePage(c[0], c[1]); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%v accepted", c)
		}
	}
}
