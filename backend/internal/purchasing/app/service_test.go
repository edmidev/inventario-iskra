package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/purchasing/adapters/memory"
	"github.com/edgar/inventario/backend/internal/purchasing/app"
	"github.com/edgar/inventario/backend/internal/purchasing/domain"
)

// fakeCatalog implements app.ItemCatalog in memory.
type fakeCatalog struct {
	items     map[string]app.ItemInfo
	locations map[int64]bool
	created   []app.CreatedItem
	creates   int
	failOnNth int // 1-based CreateItem call that fails; 0 never
}

func newCatalog() *fakeCatalog {
	return &fakeCatalog{
		items: map[string]app.ItemInfo{
			"cons-1":  {Kind: "consumable", Description: "Papel A4"},
			"asset-1": {Kind: "asset", Description: "Proyector"},
		},
		locations: map[int64]bool{1: true},
	}
}

func (c *fakeCatalog) ValidateNewItem(_ context.Context, s app.NewItemSpec) error {
	if s.CategoryID <= 0 {
		return domain.FieldError("category_id", "category does not exist")
	}
	return nil
}

func (c *fakeCatalog) CreateItem(_ context.Context, _ app.NewItemSpec, _ int64) (app.CreatedItem, error) {
	c.creates++
	if c.failOnNth == c.creates {
		return app.CreatedItem{}, errors.New("catalog down")
	}
	it := app.CreatedItem{ID: fmt.Sprintf("new-%d", c.creates), Code: fmt.Sprintf("ISK-T-%03d", c.creates)}
	c.created = append(c.created, it)
	return it, nil
}

func (c *fakeCatalog) ItemInfo(_ context.Context, id string) (app.ItemInfo, error) {
	if i, ok := c.items[id]; ok {
		return i, nil
	}
	return app.ItemInfo{}, app.ErrItemNotFound
}

func (c *fakeCatalog) LocationExists(_ context.Context, id int64) (bool, error) {
	return c.locations[id], nil
}

type stockCall struct {
	ItemID, UserID, Note string
	Loc                  int64
	Qty                  int
}

type fakeStock struct {
	calls []stockCall
	fail  error
}

func (s *fakeStock) ReceiveIn(_ context.Context, itemID, userID string, loc int64, qty int, note string) error {
	if s.fail != nil {
		return s.fail
	}
	s.calls = append(s.calls, stockCall{itemID, userID, note, loc, qty})
	return nil
}

type fakeSuppliers map[int64]struct {
	name   string
	active bool
}

func (f fakeSuppliers) Get(_ context.Context, id int64) (string, bool, error) {
	s, ok := f[id]
	if !ok {
		return "", false, app.ErrSupplierNotFound
	}
	return s.name, s.active, nil
}

type env struct {
	svc   *app.Service
	store *memory.Store
	cat   *fakeCatalog
	stock *fakeStock
	sups  fakeSuppliers
}

func newEnv() env {
	st := memory.NewStore()
	st.SetSupplierName(1, "Ferretería Sur")
	cat, stock := newCatalog(), &fakeStock{}
	sups := fakeSuppliers{
		1: {"Ferretería Sur", true},
		2: {"Norte", false},
	}
	clock := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	return env{svc: app.NewService(st, cat, stock, sups, func() time.Time { return clock }), store: st, cat: cat, stock: stock, sups: sups}
}

var ctx = context.Background()

func laptops(n int) domain.LineInput {
	return domain.LineInput{
		NewItem:  &domain.NewItem{Description: "Laptop", CategoryID: 1, Kind: domain.KindAsset},
		Quantity: n, UnitCostMinor: 90000,
	}
}

func inkNew(n int) domain.LineInput {
	return domain.LineInput{
		NewItem:  &domain.NewItem{Description: "Tinta", CategoryID: 1, Kind: domain.KindConsumable, Unit: "u"},
		Quantity: n, UnitCostMinor: 1200,
	}
}

func paper(n int) domain.LineInput {
	return domain.LineInput{ItemID: "cons-1", Quantity: n, UnitCostMinor: 450}
}

func (e env) order(t *testing.T, lines ...domain.LineInput) domain.Order {
	t.Helper()
	o, err := e.svc.Create(ctx, "u-1", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: lines})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (e env) placed(t *testing.T, lines ...domain.LineInput) domain.Order {
	t.Helper()
	o := e.order(t, lines...)
	o, err := e.svc.Place(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func fieldsOf(t *testing.T, err error) (string, map[string]string) {
	t.Helper()
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	return ve.Code, ve.Fields
}

func TestCreateDraft(t *testing.T) {
	e := newEnv()
	o := e.order(t, paper(10), laptops(2))
	if o.Status != domain.StatusDraft || o.SupplierName != "Ferretería Sur" || o.CreatedBy != "u-1" {
		t.Fatalf("draft: %+v", o)
	}
	if o.Lines[0].Description != "Papel A4" || o.Lines[1].Description != "Laptop" {
		t.Fatalf("description snapshots: %q %q", o.Lines[0].Description, o.Lines[1].Description)
	}
	if o.Total() != 10*450+2*90000 {
		t.Fatalf("total %d", o.Total())
	}
}

func TestCreateValidation(t *testing.T) {
	e := newEnv()
	tests := []struct {
		name  string
		in    domain.Input
		code  string
		field string
	}{
		{"unknown supplier", domain.Input{SupplierID: 99, Currency: domain.USD}, "", "supplier_id"},
		{"inactive supplier", domain.Input{SupplierID: 2, Currency: domain.USD}, domain.CodeSupplierInactive, "supplier_id"},
		{"existing asset item", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{{ItemID: "asset-1", Quantity: 1}}}, "", "lines[0].item_id"},
		{"unknown item", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{{ItemID: "nope", Quantity: 1}}}, "", "lines[0].item_id"},
		{"invalid new_item surfaces early", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{
			{NewItem: &domain.NewItem{Description: "x", Kind: "asset", CategoryID: 0}, Quantity: 1}}}, "", "lines[0].new_item.category_id"},
		{"asset over the cap", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{laptops(101)}}, "", "lines[0].quantity"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Create(ctx, "", tc.in)
			code, fields := fieldsOf(t, err)
			if code != tc.code {
				t.Fatalf("code = %q, want %q", code, tc.code)
			}
			if _, ok := fields[tc.field]; !ok {
				t.Fatalf("field %s missing: %v", tc.field, fields)
			}
		})
	}
	if _, f := fieldsOf(t, func() error {
		_, err := e.svc.Create(ctx, "", domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{{ItemID: "asset-1", Quantity: 1}}})
		return err
	}()); f["lines[0].item_id"] != "existing_item_must_be_consumable" {
		t.Fatalf("message: %v", f)
	}
}

func TestUpdateOnlyInDraftAndReplacesLines(t *testing.T) {
	e := newEnv()
	o := e.order(t, paper(1))
	rate := 40.5
	notes := "urgente"
	cur := domain.VES
	o, err := e.svc.Update(ctx, o.ID, app.Patch{
		Currency: &cur, ExchangeRate: app.Opt[float64]{Set: true, Val: &rate}, Notes: &notes,
		Lines: []domain.LineInput{paper(7), laptops(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if o.Currency != domain.VES || *o.ExchangeRate != 40.5 || o.Notes != "urgente" || len(o.Lines) != 2 || o.Lines[0].Quantity != 7 {
		t.Fatalf("patched: %+v", o)
	}
	// Without lines the set is untouched; null clears the rate.
	o, err = e.svc.Update(ctx, o.ID, app.Patch{ExchangeRate: app.Opt[float64]{Set: true}})
	if err != nil || o.ExchangeRate != nil || len(o.Lines) != 2 {
		t.Fatalf("clear rate: %+v %v", o, err)
	}
	if _, err := e.svc.Place(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Update(ctx, o.ID, app.Patch{Notes: &notes})
	var ise *domain.InvalidStateError
	if !errors.As(err, &ise) || ise.Status != domain.StatusOrdered {
		t.Fatalf("edit after place: %v", err)
	}
	if _, err := e.svc.Update(ctx, 999, app.Patch{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing order: %v", err)
	}
}

func TestUpdateRevalidatesSupplierAndLines(t *testing.T) {
	e := newEnv()
	o := e.order(t, paper(1))
	inactive := int64(2)
	if _, err := e.svc.Update(ctx, o.ID, app.Patch{SupplierID: &inactive}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("inactive supplier on update: %v", err)
	}
	if _, err := e.svc.Update(ctx, o.ID, app.Patch{Lines: []domain.LineInput{{ItemID: "asset-1", Quantity: 1}}}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("asset item on update: %v", err)
	}
	got, _ := e.svc.Get(ctx, o.ID)
	if got.SupplierID != 1 || got.Lines[0].ItemID != "cons-1" {
		t.Fatalf("failed update changed the order: %+v", got)
	}
}

func TestPlace(t *testing.T) {
	e := newEnv()
	empty := e.order(t)
	_, err := e.svc.Place(ctx, empty.ID)
	if _, f := fieldsOf(t, err); f["lines"] == "" {
		t.Fatalf("empty order: %v", err)
	}

	o := e.order(t, paper(1))
	e.sups[1] = struct {
		name   string
		active bool
	}{"Ferretería Sur", false}
	_, err = e.svc.Place(ctx, o.ID)
	if code, _ := fieldsOf(t, err); code != domain.CodeSupplierInactive {
		t.Fatalf("supplier deactivated after draft: %v", err)
	}
	delete(e.sups, 1)
	_, err = e.svc.Place(ctx, o.ID)
	if code, _ := fieldsOf(t, err); code != domain.CodeSupplierInactive {
		t.Fatalf("supplier removed after draft: %v", err)
	}
	e.sups[1] = struct {
		name   string
		active bool
	}{"Ferretería Sur", true}
	o, err = e.svc.Place(ctx, o.ID)
	if err != nil || o.Status != domain.StatusOrdered || o.OrderedAt == nil {
		t.Fatalf("place: %+v %v", o, err)
	}
	if _, err := e.svc.Place(ctx, o.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("place twice: %v", err)
	}
}

func TestCancel(t *testing.T) {
	e := newEnv()
	d := e.order(t, paper(1))
	if o, err := e.svc.Cancel(ctx, d.ID); err != nil || o.Status != domain.StatusCancelled {
		t.Fatalf("cancel draft: %v %v", o.Status, err)
	}
	if _, err := e.svc.Cancel(ctx, d.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("cancel twice: %v", err)
	}
	p := e.placed(t, paper(5))
	if _, err := e.svc.Receive(ctx, p.ID, "u-1", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: p.Lines[0].ID, Quantity: 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Cancel(ctx, p.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("cancel after partial receipt: %v", err)
	}
	q := e.placed(t, paper(5))
	if o, err := e.svc.Cancel(ctx, q.ID); err != nil || o.Status != domain.StatusCancelled {
		t.Fatalf("cancel ordered: %v %v", o.Status, err)
	}
}

func TestReceiveAssetLineCreatesOneItemPerUnitWithDistinctCodes(t *testing.T) {
	e := newEnv()
	o := e.placed(t, laptops(3))
	res, err := e.svc.Receive(ctx, o.ID, "u-1", domain.ReceiveRequest{
		LocationID: 1, Lines: []domain.ReceiveLine{{LineID: o.Lines[0].ID, Quantity: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	items := res.Receipt.Lines[0].Items
	if res.Replayed || len(items) != 3 || res.Order.Status != domain.StatusReceived {
		t.Fatalf("result: %+v", res)
	}
	codes := map[string]bool{}
	for _, it := range items {
		codes[it.Code] = true
		if it.Quantity != 1 {
			t.Fatalf("asset item quantity %d", it.Quantity)
		}
	}
	if len(codes) != 3 {
		t.Fatalf("codes not distinct: %v", codes)
	}
	if res.Order.Lines[0].ItemID != items[0].ItemID || res.Order.Lines[0].NewItem == nil {
		t.Fatalf("line should point at the first created item and keep new_item: %+v", res.Order.Lines[0])
	}
	if len(e.stock.calls) != 0 {
		t.Fatalf("assets must not record stock: %v", e.stock.calls)
	}
}

func TestPartialReceiptOfAssetsCreatesOnlyReceivedCount(t *testing.T) {
	e := newEnv()
	o := e.placed(t, laptops(3))
	req := func(q int) domain.ReceiveRequest {
		return domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: o.Lines[0].ID, Quantity: q}}}
	}
	r1, err := e.svc.Receive(ctx, o.ID, "u-1", req(1))
	if err != nil || r1.Order.Status != domain.StatusPartiallyReceived || len(e.cat.created) != 1 {
		t.Fatalf("first: %v %v created=%d", r1.Order.Status, err, len(e.cat.created))
	}
	r2, err := e.svc.Receive(ctx, o.ID, "u-1", req(2))
	if err != nil || r2.Order.Status != domain.StatusReceived || len(e.cat.created) != 3 {
		t.Fatalf("second: %v %v created=%d", r2.Order.Status, err, len(e.cat.created))
	}
	if r2.Order.Lines[0].ItemID != r1.Receipt.Lines[0].Items[0].ItemID {
		t.Fatal("line item_id must stay the first created item")
	}
}

func TestNewConsumableIsCreatedOnceAndStockedEachReceipt(t *testing.T) {
	e := newEnv()
	o := e.placed(t, inkNew(10), paper(4))
	line := func(i, q int) domain.ReceiveLine { return domain.ReceiveLine{LineID: o.Lines[i].ID, Quantity: q} }

	r1, err := e.svc.Receive(ctx, o.ID, "u-9", domain.ReceiveRequest{LocationID: 1, Note: "camión 1", Lines: []domain.ReceiveLine{line(0, 6), line(1, 4)}})
	if err != nil || r1.Order.Status != domain.StatusPartiallyReceived {
		t.Fatalf("r1: %v %v", r1.Order.Status, err)
	}
	if len(e.cat.created) != 1 {
		t.Fatalf("new consumable created %d times", len(e.cat.created))
	}
	r2, err := e.svc.Receive(ctx, o.ID, "u-9", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{line(0, 4)}})
	if err != nil || r2.Order.Status != domain.StatusReceived {
		t.Fatalf("r2: %v %v", r2.Order.Status, err)
	}
	if len(e.cat.created) != 1 {
		t.Fatalf("second partial receipt created another item: %d", len(e.cat.created))
	}
	want := []stockCall{
		{"new-1", "u-9", "Purchase order #1: camión 1", 1, 6},
		{"cons-1", "u-9", "Purchase order #1: camión 1", 1, 4},
		{"new-1", "u-9", "Purchase order #1", 1, 4},
	}
	if len(e.stock.calls) != len(want) {
		t.Fatalf("stock calls: %+v", e.stock.calls)
	}
	for i, w := range want {
		if e.stock.calls[i] != w {
			t.Fatalf("stock call %d = %+v, want %+v", i, e.stock.calls[i], w)
		}
	}
}

func TestReceiveRejections(t *testing.T) {
	e := newEnv()
	d := e.order(t, paper(5))
	rec := func(o domain.Order, loc int64, q int) (app.ReceiveResult, error) {
		return e.svc.Receive(ctx, o.ID, "u", domain.ReceiveRequest{LocationID: loc, Lines: []domain.ReceiveLine{{LineID: o.Lines[0].ID, Quantity: q}}})
	}
	if _, err := rec(d, 1, 1); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("receive a draft: %v", err)
	}
	o := e.placed(t, paper(5))
	if code, f := fieldsOf(t, func() error { _, err := rec(o, 1, 6); return err }()); code != domain.CodeOverReceipt || f["lines[0].quantity"] == "" {
		t.Fatalf("over receipt: %q %v", code, f)
	}
	if _, f := fieldsOf(t, func() error { _, err := rec(o, 42, 1); return err }()); f["location_id"] == "" {
		t.Fatalf("unknown location: %v", f)
	}
	if _, err := e.svc.Receive(ctx, 999, "u", domain.ReceiveRequest{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing order: %v", err)
	}
	if len(e.stock.calls) != 0 {
		t.Fatalf("rejected receipts moved stock: %v", e.stock.calls)
	}
}

func TestReceiveFailureLeavesTheOrderUntouched(t *testing.T) {
	e := newEnv()
	o := e.placed(t, laptops(3))
	e.cat.failOnNth = 2
	_, err := e.svc.Receive(ctx, o.ID, "u", domain.ReceiveRequest{
		LocationID: 1, Lines: []domain.ReceiveLine{{LineID: o.Lines[0].ID, Quantity: 3}}})
	if err == nil {
		t.Fatal("want failure")
	}
	got, _ := e.svc.Get(ctx, o.ID)
	if got.Status != domain.StatusOrdered || got.Lines[0].ReceivedQty != 0 || len(got.Receipts) != 0 || got.Lines[0].ItemID != "" {
		t.Fatalf("order changed by a failed receipt: %+v", got)
	}

	e.stock.fail = errors.New("stock down")
	p := e.placed(t, paper(2))
	if _, err := e.svc.Receive(ctx, p.ID, "u", domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: p.Lines[0].ID, Quantity: 2}}}); err == nil {
		t.Fatal("want stock failure")
	}
	got, _ = e.svc.Get(ctx, p.ID)
	if got.Status != domain.StatusOrdered || len(got.Receipts) != 0 {
		t.Fatalf("order changed by a failed stock movement: %+v", got)
	}
}

func TestReceiveIsIdempotentPerClientKey(t *testing.T) {
	e := newEnv()
	o := e.placed(t, laptops(2), paper(5))
	req := domain.ReceiveRequest{ClientKey: "k1", LocationID: 1, Lines: []domain.ReceiveLine{{LineID: o.Lines[0].ID, Quantity: 2}}}
	first, err := e.svc.Receive(ctx, o.ID, "u", req)
	if err != nil || first.Replayed {
		t.Fatalf("first: %v %+v", err, first)
	}
	created, calls := len(e.cat.created), len(e.stock.calls)
	again, err := e.svc.Receive(ctx, o.ID, "u", req)
	if err != nil || !again.Replayed || again.Receipt.ID != first.Receipt.ID {
		t.Fatalf("replay: %v %+v", err, again)
	}
	if len(e.cat.created) != created || len(e.stock.calls) != calls || len(again.Order.Receipts) != 1 {
		t.Fatal("replay had side effects")
	}
	// A different key is a different delivery; an empty key never dedupes.
	req.ClientKey = "k2"
	req.Lines = []domain.ReceiveLine{{LineID: o.Lines[1].ID, Quantity: 1}}
	if r, err := e.svc.Receive(ctx, o.ID, "u", req); err != nil || r.Replayed {
		t.Fatalf("new key: %v %+v", err, r)
	}
}

func TestListFilters(t *testing.T) {
	e := newEnv()
	a := e.order(t, paper(1))
	b := e.placed(t, paper(1))
	_ = a
	os, total, f, err := e.svc.List(ctx, domain.Filter{Status: domain.StatusOrdered})
	if err != nil || total != 1 || os[0].ID != b.ID || f.Limit != domain.DefaultLimit {
		t.Fatalf("status filter: %v %d %v", os, total, err)
	}
	if _, total, _, _ := e.svc.List(ctx, domain.Filter{Q: "sur"}); total != 2 {
		t.Fatalf("supplier name q: %d", total)
	}
	if _, total, _, _ := e.svc.List(ctx, domain.Filter{Q: "#2"}); total != 1 {
		t.Fatalf("id q: %d", total)
	}
	if _, _, _, err := e.svc.List(ctx, domain.Filter{Limit: 1000}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("bad limit: %v", err)
	}
}
