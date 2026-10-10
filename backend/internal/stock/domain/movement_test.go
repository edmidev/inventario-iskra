package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/edgar/inventario/backend/internal/stock/domain"
)

func p64(v int64) *int64 { return &v }

func TestDecide(t *testing.T) {
	cons := domain.ItemState{Kind: domain.KindConsumable, StockCurrent: 10, LocationID: p64(1)}
	asset := domain.ItemState{Kind: domain.KindAsset, StockCurrent: 1, LocationID: p64(1)}

	tests := []struct {
		name      string
		state     domain.ItemState
		req       domain.Request
		wantErr   error
		wantField string
		qty       int
		after     int
		loc       *int64
	}{
		{name: "in adds", state: cons, req: domain.Request{Type: domain.TypeIn, Qty: 5}, qty: 5, after: 15, loc: p64(1)},
		{name: "out subtracts", state: cons, req: domain.Request{Type: domain.TypeOut, Qty: 4}, qty: -4, after: 6, loc: p64(1)},
		{name: "out to exactly zero", state: cons, req: domain.Request{Type: domain.TypeOut, Qty: 10}, qty: -10, after: 0, loc: p64(1)},
		{name: "out below zero", state: cons, req: domain.Request{Type: domain.TypeOut, Qty: 11}, wantErr: domain.ErrInsufficientStock},
		{name: "out zero qty", state: cons, req: domain.Request{Type: domain.TypeOut, Qty: 0}, wantField: "qty"},
		{name: "in negative qty", state: cons, req: domain.Request{Type: domain.TypeIn, Qty: -1}, wantField: "qty"},
		{name: "in overflow", state: cons, req: domain.Request{Type: domain.TypeIn, Qty: domain.MaxStock}, wantField: "qty"},
		{name: "adjust down records delta", state: cons, req: domain.Request{Type: domain.TypeAdjust, Qty: 7, Note: "count"}, qty: -3, after: 7, loc: p64(1)},
		{name: "adjust up records delta", state: cons, req: domain.Request{Type: domain.TypeAdjust, Qty: 12, Note: "count"}, qty: 2, after: 12, loc: p64(1)},
		{name: "adjust to zero", state: cons, req: domain.Request{Type: domain.TypeAdjust, Qty: 0, Note: "empty shelf"}, qty: -10, after: 0, loc: p64(1)},
		{name: "adjust without note", state: cons, req: domain.Request{Type: domain.TypeAdjust, Qty: 3, Note: "  "}, wantField: "note"},
		{name: "adjust negative", state: cons, req: domain.Request{Type: domain.TypeAdjust, Qty: -1, Note: "x"}, wantField: "qty"},
		{name: "transfer consumable", state: cons, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2)}, qty: 0, after: 10, loc: p64(2)},
		{name: "transfer asset", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2)}, qty: 0, after: 1, loc: p64(2)},
		{name: "transfer without destination", state: asset, req: domain.Request{Type: domain.TypeTransfer}, wantField: "to_location_id"},
		{name: "transfer to same location", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(1)}, wantField: "to_location_id"},
		{name: "transfer with qty", state: asset, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2), Qty: 3}, wantField: "qty"},
		{name: "transfer from nowhere", state: domain.ItemState{Kind: domain.KindAsset, StockCurrent: 1}, req: domain.Request{Type: domain.TypeTransfer, ToLocationID: p64(2)}, qty: 0, after: 1, loc: p64(2)},
		{name: "in on asset", state: asset, req: domain.Request{Type: domain.TypeIn, Qty: 1}, wantErr: domain.ErrNotApplicable},
		{name: "out on asset", state: asset, req: domain.Request{Type: domain.TypeOut, Qty: 1}, wantErr: domain.ErrNotApplicable},
		{name: "adjust on asset", state: asset, req: domain.Request{Type: domain.TypeAdjust, Qty: 1, Note: "x"}, wantErr: domain.ErrNotApplicable},
		{name: "opening not requestable", state: cons, req: domain.Request{Type: domain.TypeOpening, Qty: 1}, wantField: "type"},
		{name: "unknown type", state: cons, req: domain.Request{Type: "burn", Qty: 1}, wantField: "type"},
		{name: "destination only for transfer", state: cons, req: domain.Request{Type: domain.TypeIn, Qty: 1, ToLocationID: p64(2)}, wantField: "to_location_id"},
		{name: "note too long", state: cons, req: domain.Request{Type: domain.TypeIn, Qty: 1, Note: strings.Repeat("x", 501)}, wantField: "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.Decide(tc.state, tc.req)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			case tc.wantField != "":
				var ve *domain.ValidationError
				if !errors.As(err, &ve) || ve.Fields[tc.wantField] == "" {
					t.Fatalf("err = %v, want validation error on %q", err, tc.wantField)
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			m := got.Movement
			if m.Qty != tc.qty || m.StockAfter != tc.after || got.StockCurrent != tc.after {
				t.Errorf("qty/after = %d/%d (state %d), want %d/%d", m.Qty, m.StockAfter, got.StockCurrent, tc.qty, tc.after)
			}
			if m.StockBefore != tc.state.StockCurrent || m.StockAfter != m.StockBefore+m.Qty {
				t.Errorf("ledger does not balance: %+v", m)
			}
			if (got.LocationID == nil) != (tc.loc == nil) || (tc.loc != nil && *got.LocationID != *tc.loc) {
				t.Errorf("location = %v, want %v", got.LocationID, tc.loc)
			}
			if tc.req.Type == domain.TypeTransfer && (m.ToLocationID == nil || *m.ToLocationID != *tc.loc) {
				t.Errorf("transfer to = %v", m.ToLocationID)
			}
		})
	}
}

func TestInsufficientStockCarriesNumbers(t *testing.T) {
	_, err := domain.Decide(domain.ItemState{Kind: domain.KindConsumable, StockCurrent: 2}, domain.Request{Type: domain.TypeOut, Qty: 5})
	var ie *domain.InsufficientStockError
	if !errors.As(err, &ie) || ie.Available != 2 || ie.Requested != 5 {
		t.Fatalf("err = %v", err)
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
