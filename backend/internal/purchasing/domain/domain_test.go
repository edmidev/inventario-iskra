package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/purchasing/domain"
)

var now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func consumableLine(qty int) domain.LineInput {
	return domain.LineInput{ItemID: "11111111-1111-1111-1111-111111111111", Quantity: qty, UnitCostMinor: 250}
}

func assetLine(qty int) domain.LineInput {
	return domain.LineInput{
		NewItem:  &domain.NewItem{Description: "Laptop", CategoryID: 1, Kind: domain.KindAsset},
		Quantity: qty, UnitCostMinor: 90000,
	}
}

func draft(t *testing.T, lines ...domain.LineInput) domain.Order {
	t.Helper()
	in, err := domain.Input{SupplierID: 1, Currency: domain.USD, Lines: lines}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	o := domain.NewDraft(in, "", now)
	o.ID = 7
	for i := range o.Lines {
		o.Lines[i].ID = int64(100 + i)
	}
	return o
}

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	return ve.Fields
}

func TestInputNormalize(t *testing.T) {
	neg := -1.0
	tests := []struct {
		name  string
		in    domain.Input
		field string // expected offending field, "" when valid
	}{
		{"ok", domain.Input{SupplierID: 1, Currency: domain.VES, Lines: []domain.LineInput{consumableLine(2)}}, ""},
		{"draft without lines is fine", domain.Input{SupplierID: 1, Currency: domain.USD}, ""},
		{"supplier required", domain.Input{Currency: domain.USD}, "supplier_id"},
		{"bad currency", domain.Input{SupplierID: 1, Currency: "EUR"}, "currency"},
		{"missing currency", domain.Input{SupplierID: 1}, "currency"},
		{"bad rate", domain.Input{SupplierID: 1, Currency: domain.USD, ExchangeRate: &neg}, "exchange_rate"},
		{"neither item nor new_item", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{{Quantity: 1}}}, "lines[0].item_id"},
		{"both item and new_item", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{
			{ItemID: "x", NewItem: &domain.NewItem{Description: "d", Kind: "asset"}, Quantity: 1}}}, "lines[0].item_id"},
		{"zero quantity", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{consumableLine(0)}}, "lines[0].quantity"},
		{"negative cost", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{
			{ItemID: "x", Quantity: 1, UnitCostMinor: -1}}}, "lines[0].unit_cost_minor"},
		{"asset at the cap", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{assetLine(domain.MaxAssetUnitsPerLine)}}, ""},
		{"asset over the cap", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{assetLine(domain.MaxAssetUnitsPerLine + 1)}}, "lines[0].quantity"},
		{"new consumable may exceed the asset cap", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{
			{NewItem: &domain.NewItem{Description: "Tinta", Kind: domain.KindConsumable, Unit: "u"}, Quantity: 500}}}, ""},
		{"new_item bad kind", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{
			{NewItem: &domain.NewItem{Description: "d", Kind: "thing"}, Quantity: 1}}}, "lines[0].new_item.kind"},
		{"new_item blank description", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{
			{NewItem: &domain.NewItem{Description: "  ", Kind: "asset"}, Quantity: 1}}}, "lines[0].new_item.description"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.in.Normalize()
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("want invalid input, got %v", err)
			}
			if _, ok := fields(t, err)[tc.field]; !ok {
				t.Fatalf("field %s not reported: %v", tc.field, err)
			}
		})
	}
}

func TestNormalizeSnapshotsNewItemDescription(t *testing.T) {
	in, err := domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{
		{NewItem: &domain.NewItem{Description: "  Laptop  ", Kind: "asset"}, Quantity: 2}}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if in.Lines[0].Description != "Laptop" || in.Lines[0].NewItem.Description != "Laptop" {
		t.Fatalf("description not trimmed/snapshotted: %+v", in.Lines[0])
	}
}

func TestTotalIsInMinorUnits(t *testing.T) {
	o := draft(t, consumableLine(4), assetLine(2))
	if got, want := o.Total(), int64(4*250+2*90000); got != want {
		t.Fatalf("total = %d, want %d", got, want)
	}
}

func receive(o *domain.Order, qty int) error {
	req := domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: qty}}}
	if _, _, err := o.PlanReceipt(req); err != nil {
		return err
	}
	o.ApplyReceipt(domain.Receipt{LocationID: 1, Lines: []domain.ReceiptLine{{LineID: 100, Quantity: qty}}}, now)
	return nil
}

func TestStateMachine(t *testing.T) {
	type action struct {
		name string
		do   func(o *domain.Order) error
	}
	place := action{"place", func(o *domain.Order) error { return o.Place(now) }}
	cancel := action{"cancel", func(o *domain.Order) error { return o.Cancel(now) }}
	edit := action{"edit", func(o *domain.Order) error {
		return o.Edit(domain.Input{SupplierID: 2, Currency: domain.VES, Notes: "x"}, now)
	}}
	recv1 := action{"receive 1", func(o *domain.Order) error { return receive(o, 1) }}
	recvAll := action{"receive rest", func(o *domain.Order) error { return receive(o, o.Lines[0].Remaining()) }}

	tests := []struct {
		name    string
		history []action // applied first, all must succeed
		act     action
		want    domain.Status // status after act; the current one when rejected
		reject  bool
	}{
		{"draft edit", nil, edit, domain.StatusDraft, false},
		{"draft place", nil, place, domain.StatusOrdered, false},
		{"draft cancel", nil, cancel, domain.StatusCancelled, false},
		{"draft cannot receive", nil, recv1, domain.StatusDraft, true},
		{"ordered cannot edit", []action{place}, edit, domain.StatusOrdered, true},
		{"ordered cannot place again", []action{place}, place, domain.StatusOrdered, true},
		{"ordered cancel", []action{place}, cancel, domain.StatusCancelled, false},
		{"ordered partial receipt", []action{place}, recv1, domain.StatusPartiallyReceived, false},
		{"ordered full receipt", []action{place}, recvAll, domain.StatusReceived, false},
		{"partial receives more", []action{place, recv1}, recv1, domain.StatusPartiallyReceived, false},
		{"partial completes", []action{place, recv1}, recvAll, domain.StatusReceived, false},
		{"partial cannot cancel", []action{place, recv1}, cancel, domain.StatusPartiallyReceived, true},
		{"partial cannot edit", []action{place, recv1}, edit, domain.StatusPartiallyReceived, true},
		{"received cannot receive", []action{place, recvAll}, recv1, domain.StatusReceived, true},
		{"received cannot cancel", []action{place, recvAll}, cancel, domain.StatusReceived, true},
		{"cancelled cannot place", []action{cancel}, place, domain.StatusCancelled, true},
		{"cancelled cannot receive", []action{place, cancel}, recv1, domain.StatusCancelled, true},
		{"cancelled cannot cancel", []action{cancel}, cancel, domain.StatusCancelled, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := draft(t, consumableLine(3))
			for _, h := range tc.history {
				if err := h.do(&o); err != nil {
					t.Fatalf("history %s: %v", h.name, err)
				}
			}
			err := tc.act.do(&o)
			if tc.reject {
				var ise *domain.InvalidStateError
				if !errors.As(err, &ise) || !errors.Is(err, domain.ErrInvalidState) {
					t.Fatalf("want InvalidStateError, got %v", err)
				}
				if ise.Status != o.Status {
					t.Fatalf("error status %s, order %s", ise.Status, o.Status)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if o.Status != tc.want {
				t.Fatalf("status = %s, want %s", o.Status, tc.want)
			}
		})
	}
}

func TestPlaceNeedsALine(t *testing.T) {
	o := draft(t)
	err := o.Place(now)
	if _, ok := fields(t, err)["lines"]; !ok {
		t.Fatalf("want lines error, got %v", err)
	}
	if o.Status != domain.StatusDraft {
		t.Fatalf("status changed to %s", o.Status)
	}
}

func TestPlaceAndCancelStampTimes(t *testing.T) {
	o := draft(t, consumableLine(1))
	_ = o.Place(now)
	if o.OrderedAt == nil || !o.OrderedAt.Equal(now) {
		t.Fatalf("ordered_at = %v", o.OrderedAt)
	}
	_ = o.Cancel(now)
	if o.CancelledAt == nil {
		t.Fatal("cancelled_at not set")
	}
}

func TestEditReplacesLinesOnlyWhenGiven(t *testing.T) {
	o := draft(t, consumableLine(1))
	o.LinesDirty = false
	if err := o.Edit(domain.Input{SupplierID: 1, Currency: domain.USD, Notes: "n"}, now); err != nil {
		t.Fatal(err)
	}
	if o.LinesDirty || len(o.Lines) != 1 {
		t.Fatalf("lines touched without being given: dirty=%v lines=%d", o.LinesDirty, len(o.Lines))
	}
	if err := o.Edit(domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{}}, now); err != nil {
		t.Fatal(err)
	}
	if !o.LinesDirty || len(o.Lines) != 0 {
		t.Fatalf("empty lines must clear the set: dirty=%v lines=%d", o.LinesDirty, len(o.Lines))
	}
}

func TestPlanReceiptRules(t *testing.T) {
	ordered := func() domain.Order {
		o := draft(t, consumableLine(5), assetLine(2))
		_ = o.Place(now)
		return o
	}
	tests := []struct {
		name  string
		req   domain.ReceiveRequest
		code  string
		field string
	}{
		{"ok partial", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 2}}}, "", ""},
		{"ok both lines", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 5}, {LineID: 101, Quantity: 2}}}, "", ""},
		{"location required", domain.ReceiveRequest{Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 1}}}, domain.CodeValidation, "location_id"},
		{"no lines", domain.ReceiveRequest{LocationID: 1}, domain.CodeValidation, "lines"},
		{"unknown line", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 999, Quantity: 1}}}, domain.CodeValidation, "lines[0].line_id"},
		{"repeated line", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 1}, {LineID: 100, Quantity: 1}}}, domain.CodeValidation, "lines[1].line_id"},
		{"zero quantity", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 0}}}, domain.CodeValidation, "lines[0].quantity"},
		{"over receipt", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 6}}}, domain.CodeOverReceipt, "lines[0].quantity"},
		{"over receipt on second line", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 1}, {LineID: 101, Quantity: 3}}}, domain.CodeOverReceipt, "lines[1].quantity"},
		{"note too long", domain.ReceiveRequest{LocationID: 1, Note: string(make([]byte, 401)), Lines: []domain.ReceiveLine{{LineID: 100, Quantity: 1}}}, domain.CodeValidation, "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := ordered()
			req, idx, err := o.PlanReceipt(tc.req)
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				if len(idx) != len(req.Lines) || o.Lines[idx[0]].ID != req.Lines[0].LineID {
					t.Fatalf("bad line index mapping: %v", idx)
				}
				return
			}
			var ve *domain.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want ValidationError, got %v", err)
			}
			code := ve.Code
			if code == "" {
				code = domain.CodeValidation
			}
			if code != tc.code {
				t.Fatalf("code = %s, want %s", code, tc.code)
			}
			if _, ok := ve.Fields[tc.field]; !ok {
				t.Fatalf("field %s not reported: %v", tc.field, ve.Fields)
			}
		})
	}
}

func TestPlanReceiptTracksRemainingAcrossReceipts(t *testing.T) {
	o := draft(t, consumableLine(5))
	_ = o.Place(now)
	if err := receive(&o, 3); err != nil {
		t.Fatal(err)
	}
	err := receive(&o, 3)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Code != domain.CodeOverReceipt {
		t.Fatalf("second receipt of 3 with 2 remaining: %v", err)
	}
	if o.Lines[0].ReceivedQty != 3 {
		t.Fatalf("failed receipt changed received_qty to %d", o.Lines[0].ReceivedQty)
	}
}

func TestReceiptByKey(t *testing.T) {
	o := draft(t, consumableLine(5))
	o.Receipts = []domain.Receipt{{ID: 1, ClientKey: "a"}, {ID: 2}}
	if r, ok := o.ReceiptByKey("a"); !ok || r.ID != 1 {
		t.Fatalf("lookup a: %v %v", r, ok)
	}
	if _, ok := o.ReceiptByKey(""); ok {
		t.Fatal("empty key must never match")
	}
	if _, ok := o.ReceiptByKey("b"); ok {
		t.Fatal("unknown key matched")
	}
}

func TestFilterNormalize(t *testing.T) {
	f, err := domain.Filter{Q: " #12 "}.Normalize()
	if err != nil || f.Limit != domain.DefaultLimit || f.Q != "#12" {
		t.Fatalf("defaults: %+v %v", f, err)
	}
	for _, bad := range []domain.Filter{{Limit: 101}, {Limit: -1}, {Offset: -1}, {Status: "nope"}, {SupplierID: -3}} {
		if _, err := bad.Normalize(); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%+v accepted", bad)
		}
	}
}
