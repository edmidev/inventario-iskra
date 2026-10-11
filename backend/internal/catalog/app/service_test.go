package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/edgar/inventario/backend/internal/catalog/adapters/memory"
	"github.com/edgar/inventario/backend/internal/catalog/app"
	"github.com/edgar/inventario/backend/internal/catalog/domain"
)

func ptr[T any](v T) *T { return &v }

type fixture struct {
	svc   *app.Service
	store *memory.Store
	cat   domain.Category
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	st := memory.NewStore()
	svc := app.NewService(st.Categories(), st.Locations(), st.Items())
	cat, err := svc.CreateCategory(t.Context(), "Herramientas", "her")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{svc: svc, store: st, cat: cat}
}

func (f fixture) asset(t *testing.T, code string) domain.Item {
	t.Helper()
	it, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{Code: code, NewItemInput: domain.NewItemInput{
		Description: "Taladro", CategoryID: f.cat.ID, Kind: domain.KindAsset}})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestCategoryUniqueness(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	if f.cat.CodePrefix != "HER" {
		t.Fatalf("prefix = %q", f.cat.CodePrefix)
	}
	if _, err := f.svc.CreateCategory(ctx, "HERRAMIENTAS", "OTR"); !errors.Is(err, domain.ErrCategoryNameTaken) {
		t.Errorf("name dup (case-insensitive): %v", err)
	}
	if _, err := f.svc.CreateCategory(ctx, "Otra", "HER"); !errors.Is(err, domain.ErrCategoryPrefixTaken) {
		t.Errorf("prefix dup: %v", err)
	}
	other, _ := f.svc.CreateCategory(ctx, "Cables", "CAB")
	if _, err := f.svc.UpdateCategory(ctx, other.ID, ptr("herramientas"), nil); !errors.Is(err, domain.ErrCategoryNameTaken) {
		t.Errorf("rename dup: %v", err)
	}
	renamed, err := f.svc.UpdateCategory(ctx, other.ID, ptr("Cableado"), nil)
	if err != nil || renamed.Name != "Cableado" || renamed.CodePrefix != "CAB" {
		t.Fatalf("rename: %+v %v", renamed, err)
	}
	if _, err := f.svc.UpdateCategory(ctx, 999, ptr("x"), nil); !errors.Is(err, domain.ErrCategoryNotFound) {
		t.Errorf("missing: %v", err)
	}
}

func TestDeleteCategoryBlockedByItems(t *testing.T) {
	f := newFixture(t)
	it := f.asset(t, "")
	if err := f.svc.DeleteCategory(t.Context(), f.cat.ID); !errors.Is(err, domain.ErrCategoryInUse) {
		t.Fatalf("with item: %v", err)
	}
	// A soft-deleted item still references its category.
	if err := f.svc.DeleteItem(t.Context(), it.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCategory(t.Context(), f.cat.ID); !errors.Is(err, domain.ErrCategoryInUse) {
		t.Fatalf("with soft-deleted item: %v", err)
	}
	empty, _ := f.svc.CreateCategory(t.Context(), "Vacía", "VAC")
	if err := f.svc.DeleteCategory(t.Context(), empty.ID); err != nil {
		t.Fatal(err)
	}
}

func TestLocationHierarchyAndCycles(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	root, err := f.svc.CreateLocation(ctx, "Bodega", domain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, _ := f.svc.CreateLocation(ctx, "Maleta de Alejandro", domain.LocationContainer, &root.ID)
	grand, _ := f.svc.CreateLocation(ctx, "Bolsillo", domain.LocationContainer, &child.ID)

	if _, err := f.svc.CreateLocation(ctx, "x", domain.LocationStorage, ptr(int64(999))); fieldsMsg(err, "parent_id") == "" {
		t.Errorf("missing parent: %v", err)
	}
	if _, err := f.svc.CreateLocation(ctx, "x", "garage", nil); fieldsMsg(err, "kind") == "" {
		t.Errorf("bad kind: %v", err)
	}
	p := app.LocationPatch{ParentID: domain.Nullable[int64]{Set: true, Value: grand.ID}}
	if _, err := f.svc.UpdateLocation(ctx, root.ID, p); !errors.Is(err, domain.ErrLocationCycle) {
		t.Errorf("cycle: %v", err)
	}
	self := app.LocationPatch{ParentID: domain.Nullable[int64]{Set: true, Value: child.ID}}
	if _, err := f.svc.UpdateLocation(ctx, child.ID, self); !errors.Is(err, domain.ErrLocationCycle) {
		t.Errorf("self parent: %v", err)
	}
	detach := app.LocationPatch{ParentID: domain.Nullable[int64]{Set: true, Null: true}}
	got, err := f.svc.UpdateLocation(ctx, grand.ID, detach)
	if err != nil || got.ParentID != nil {
		t.Fatalf("detach: %+v %v", got, err)
	}
	renamed, err := f.svc.UpdateLocation(ctx, child.ID, app.LocationPatch{Name: ptr("Maleta 2")})
	if err != nil || renamed.Name != "Maleta 2" || renamed.ParentID == nil || *renamed.ParentID != root.ID {
		t.Fatalf("rename keeps parent: %+v %v", renamed, err)
	}
}

func fieldsMsg(err error, field string) string {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		return ve.Fields[field]
	}
	return ""
}

func TestDeleteLocationBlockedByChildrenAndItems(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	root, _ := f.svc.CreateLocation(ctx, "Bodega", domain.LocationStorage, nil)
	child, _ := f.svc.CreateLocation(ctx, "Estante", domain.LocationStorage, &root.ID)
	if err := f.svc.DeleteLocation(ctx, root.ID); !errors.Is(err, domain.ErrLocationInUse) {
		t.Fatalf("children: %v", err)
	}
	if _, err := f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "Taladro", CategoryID: f.cat.ID, Kind: domain.KindAsset, LocationID: &child.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteLocation(ctx, child.ID); !errors.Is(err, domain.ErrLocationInUse) {
		t.Fatalf("items: %v", err)
	}
	empty, _ := f.svc.CreateLocation(ctx, "Vacía", domain.LocationStorage, nil)
	if err := f.svc.DeleteLocation(ctx, empty.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteLocation(ctx, empty.ID); !errors.Is(err, domain.ErrLocationNotFound) {
		t.Fatalf("again: %v", err)
	}
}

func TestGeneratedCodesAreSequentialPerPrefix(t *testing.T) {
	f := newFixture(t)
	cab, _ := f.svc.CreateCategory(t.Context(), "Cables", "CAB")
	want := []string{"ISK-HER-001", "ISK-HER-002", "ISK-HER-003"}
	for _, w := range want {
		if got := f.asset(t, ""); got.Code != w {
			t.Fatalf("got %s want %s", got.Code, w)
		}
	}
	c, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "Cable", CategoryID: cab.ID, Kind: domain.KindConsumable, Unit: ptr("metro")}})
	if err != nil || c.Code != "ISK-CAB-001" {
		t.Fatalf("other prefix: %+v %v", c, err)
	}
}

func TestGeneratedCodeSkipsSuppliedCodes(t *testing.T) {
	f := newFixture(t)
	f.asset(t, "ISK-HER-001")
	f.asset(t, "ISK-HER-002")
	if got := f.asset(t, ""); got.Code != "ISK-HER-003" {
		t.Fatalf("got %s", got.Code)
	}
}

func TestSuppliedCodeRules(t *testing.T) {
	f := newFixture(t)
	f.asset(t, "ISK-OLD-7")
	_, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{Code: "ISK-OLD-7", NewItemInput: domain.NewItemInput{
		Description: "dup", CategoryID: f.cat.ID, Kind: domain.KindAsset}})
	if !errors.Is(err, domain.ErrCodeTaken) {
		t.Errorf("duplicate: %v", err)
	}
	_, err = f.svc.CreateItem(t.Context(), app.CreateItemInput{Code: "bad code", NewItemInput: domain.NewItemInput{
		Description: "x", CategoryID: f.cat.ID, Kind: domain.KindAsset}})
	if fieldsMsg(err, "code") == "" {
		t.Errorf("format: %v", err)
	}
}

func TestDeletedItemKeepsCodeReserved(t *testing.T) {
	f := newFixture(t)
	it := f.asset(t, "ISK-HER-050")
	if err := f.svc.DeleteItem(t.Context(), it.ID); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{Code: "ISK-HER-050", NewItemInput: domain.NewItemInput{
		Description: "again", CategoryID: f.cat.ID, Kind: domain.KindAsset}})
	if !errors.Is(err, domain.ErrCodeTaken) {
		t.Fatalf("reused deleted code: %v", err)
	}
	if _, err := f.svc.GetItem(t.Context(), it.ID); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("get deleted: %v", err)
	}
	if _, err := f.svc.GetItemByCode(t.Context(), "ISK-HER-050"); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("by code deleted: %v", err)
	}
	items, total, _, _ := f.svc.ListItems(t.Context(), domain.ItemFilter{})
	if total != 0 || len(items) != 0 {
		t.Errorf("deleted listed: %d", total)
	}
	if err := f.svc.DeleteItem(t.Context(), it.ID); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("double delete: %v", err)
	}
}

func TestConcurrentCodeGenerationIsUnique(t *testing.T) {
	f := newFixture(t)
	const n = 40
	codes := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			it, err := f.svc.CreateItem(context.Background(), app.CreateItemInput{NewItemInput: domain.NewItemInput{
				Description: "x", CategoryID: f.cat.ID, Kind: domain.KindAsset}})
			if err != nil {
				t.Error(err)
				return
			}
			codes[i] = it.Code
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, c := range codes {
		if c == "" || seen[c] {
			t.Fatalf("duplicate or empty code %q", c)
		}
		seen[c] = true
	}
}

func TestCreateItemReferenceValidation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	_, err := f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "x", CategoryID: 999, Kind: domain.KindAsset}})
	if fieldsMsg(err, "category_id") == "" {
		t.Errorf("category: %v", err)
	}
	_, err = f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "x", CategoryID: f.cat.ID, Kind: domain.KindAsset, LocationID: ptr(int64(999))}})
	if fieldsMsg(err, "location_id") == "" {
		t.Errorf("location: %v", err)
	}
	_, err = f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "", CategoryID: f.cat.ID, Kind: domain.KindConsumable}})
	if fieldsMsg(err, "description") == "" || fieldsMsg(err, "unit") == "" {
		t.Errorf("fields: %v", err)
	}
}

func TestUpdateItem(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	cons, err := f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "Cinta", CategoryID: f.cat.ID, Kind: domain.KindConsumable, Unit: ptr("rollo"), StockCurrent: ptr(8)}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.UpdateItem(ctx, cons.ID, domain.ItemPatch{StockMin: ptr(3), Status: ptr(domain.StatusInoperative)})
	if err != nil || got.StockMin != 3 || got.StockCurrent != 8 || got.Status != domain.StatusInoperative {
		t.Fatalf("update: %+v %v", got, err)
	}
	if _, err := f.svc.UpdateItem(ctx, cons.ID, domain.ItemPatch{Kind: ptr(domain.KindAsset)}); fieldsMsg(err, "kind") == "" {
		t.Errorf("kind change: %v", err)
	}
	if _, err := f.svc.UpdateItem(ctx, cons.ID, domain.ItemPatch{StockCurrent: ptr(100)}); fieldsMsg(err, "stock_current") == "" {
		t.Errorf("stock edit: %v", err)
	}
	if _, err := f.svc.UpdateItem(ctx, cons.ID, domain.ItemPatch{Code: ptr("ISK-X-1")}); fieldsMsg(err, "code") == "" {
		t.Errorf("code change: %v", err)
	}
	if _, err := f.svc.UpdateItem(ctx, cons.ID, domain.ItemPatch{CategoryID: ptr(int64(999))}); fieldsMsg(err, "category_id") == "" {
		t.Errorf("bad category: %v", err)
	}
	if _, err := f.svc.UpdateItem(ctx, "00000000-0000-4000-8000-0000000000ff", domain.ItemPatch{}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("missing: %v", err)
	}
	reread, _ := f.svc.GetItem(ctx, cons.ID)
	if reread.Kind != domain.KindConsumable || reread.StockCurrent != 8 || reread.Code != cons.Code {
		t.Fatalf("immutable fields changed: %+v", reread)
	}
}

func TestListItemsFiltersSortAndPaging(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	cab, _ := f.svc.CreateCategory(ctx, "Cables", "CAB")
	bodega, _ := f.svc.CreateLocation(ctx, "Bodega", domain.LocationStorage, nil)
	mk := func(desc string, cat int64, kind domain.Kind, mod func(*domain.NewItemInput)) {
		in := domain.NewItemInput{Description: desc, CategoryID: cat, Kind: kind}
		if kind == domain.KindConsumable {
			in.Unit = ptr("u")
		}
		if mod != nil {
			mod(&in)
		}
		if _, err := f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: in}); err != nil {
			t.Fatal(err)
		}
	}
	mk("Taladro", f.cat.ID, domain.KindAsset, func(i *domain.NewItemInput) { i.LocationID = &bodega.ID })
	mk("Martillo", f.cat.ID, domain.KindAsset, func(i *domain.NewItemInput) { i.Status = domain.StatusInRepair })
	mk("Cable HDMI", cab.ID, domain.KindConsumable, func(i *domain.NewItemInput) { i.StockCurrent = ptr(1); i.StockMin = 2 })
	mk("Cinta", cab.ID, domain.KindConsumable, func(i *domain.NewItemInput) { i.StockCurrent = ptr(50); i.StockMin = 2 })

	list := func(fl domain.ItemFilter) ([]domain.Item, int) {
		items, total, _, err := f.svc.ListItems(ctx, fl)
		if err != nil {
			t.Fatal(err)
		}
		return items, total
	}
	if _, total := list(domain.ItemFilter{}); total != 4 {
		t.Errorf("all: %d", total)
	}
	if items, _ := list(domain.ItemFilter{Q: "cable"}); len(items) != 1 || items[0].Description != "Cable HDMI" {
		t.Errorf("q: %+v", items)
	}
	if _, total := list(domain.ItemFilter{CategoryID: &cab.ID}); total != 2 {
		t.Errorf("category: %d", total)
	}
	if _, total := list(domain.ItemFilter{Status: domain.StatusInRepair}); total != 1 {
		t.Errorf("status: %d", total)
	}
	if _, total := list(domain.ItemFilter{Availability: domain.Available}); total != 3 {
		t.Errorf("available: %d", total)
	}
	if _, total := list(domain.ItemFilter{Availability: domain.Unavailable}); total != 1 {
		t.Errorf("unavailable: %d", total)
	}
	if _, total := list(domain.ItemFilter{LocationID: &bodega.ID}); total != 1 {
		t.Errorf("location: %d", total)
	}
	if items, _ := list(domain.ItemFilter{LowStock: true}); len(items) != 1 || items[0].Description != "Cable HDMI" {
		t.Errorf("low stock: %+v", items)
	}
	items, _ := list(domain.ItemFilter{Sort: domain.SortDescription})
	if items[0].Description != "Cable HDMI" || items[3].Description != "Taladro" {
		t.Errorf("sort asc: %v", descs(items))
	}
	items, _ = list(domain.ItemFilter{Sort: domain.SortDescription, Desc: true})
	if items[0].Description != "Taladro" {
		t.Errorf("sort desc: %v", descs(items))
	}
	page, total := list(domain.ItemFilter{Sort: domain.SortCode, Limit: 2, Offset: 3})
	if total != 4 || len(page) != 1 {
		t.Errorf("paging: total %d len %d", total, len(page))
	}
	if page, total = list(domain.ItemFilter{Offset: 50}); total != 4 || len(page) != 0 {
		t.Errorf("offset past end: %d %d", total, len(page))
	}
	if _, _, _, err := f.svc.ListItems(ctx, domain.ItemFilter{Limit: 1000}); err == nil {
		t.Error("limit over max accepted")
	}
}

func descs(items []domain.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Description
	}
	return out
}

func TestListItemsAvailabilityAccountsForLoans(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	free, lent := f.asset(t, "ISK-HER-001"), f.asset(t, "ISK-HER-002")
	f.store.SetLoan(lent.ID, &domain.LoanInfo{AssigneeLocationID: 3, AssigneeName: "Alejandro"})

	got, total, _, err := f.svc.ListItems(ctx, domain.ItemFilter{Availability: domain.Available})
	if err != nil || total != 1 || got[0].ID != free.ID {
		t.Fatalf("available: %v %d %+v", err, total, got)
	}
	got, _, _, _ = f.svc.ListItems(ctx, domain.ItemFilter{Availability: domain.Unavailable})
	if len(got) != 1 || got[0].ID != lent.ID || !got[0].OnLoan() {
		t.Fatalf("unavailable: %+v", got)
	}
	f.store.SetLoan(lent.ID, nil) // returned
	if _, total, _, _ := f.svc.ListItems(ctx, domain.ItemFilter{Availability: domain.Available}); total != 2 {
		t.Errorf("after return both are available, got %d", total)
	}
}

func TestOnLoanItemsRejectDeleteAndLocationChange(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	loc, err := f.svc.CreateLocation(ctx, "Bodega", domain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	it := f.asset(t, "ISK-HER-001")
	f.store.SetLoan(it.ID, &domain.LoanInfo{AssigneeLocationID: 3, AssigneeName: "Alejandro"})

	move := domain.ItemPatch{LocationID: domain.Nullable[int64]{Set: true, Value: loc.ID}}
	if _, err := f.svc.UpdateItem(ctx, it.ID, move); !errors.Is(err, domain.ErrItemOnLoan) {
		t.Errorf("move: %v", err)
	}
	if err := f.svc.DeleteItem(ctx, it.ID); !errors.Is(err, domain.ErrItemOnLoan) {
		t.Errorf("delete: %v", err)
	}
	if got, err := f.svc.UpdateItem(ctx, it.ID, domain.ItemPatch{Notes: ptr("note")}); err != nil || !got.OnLoan() {
		t.Errorf("unrelated edit keeps the loan: %v %+v", err, got)
	}
	f.store.SetLoan(it.ID, nil)
	if _, err := f.svc.UpdateItem(ctx, it.ID, move); err != nil {
		t.Errorf("move after return: %v", err)
	}
	if err := f.svc.DeleteItem(ctx, it.ID); err != nil {
		t.Errorf("delete after return: %v", err)
	}
}

func TestStatusChangeRejectedWhileFailureIsOpen(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	it := f.asset(t, "ISK-HER-001")
	f.store.SetOpenFailure(it.ID, true)

	if _, err := f.svc.UpdateItem(ctx, it.ID, domain.ItemPatch{Status: ptr(domain.StatusInRepair)}); !errors.Is(err, domain.ErrItemUnderRepair) {
		t.Fatalf("status change: %v", err)
	}
	if got, _ := f.svc.GetItem(ctx, it.ID); got.Status != domain.StatusFunctional {
		t.Errorf("rejected change must not persist, status %q", got.Status)
	}
	// Re-sending the same status and unrelated edits stay allowed.
	if _, err := f.svc.UpdateItem(ctx, it.ID, domain.ItemPatch{Status: ptr(domain.StatusFunctional), Notes: ptr("same")}); err != nil {
		t.Errorf("same status: %v", err)
	}
	f.store.SetOpenFailure(it.ID, false) // failure closed
	if got, err := f.svc.UpdateItem(ctx, it.ID, domain.ItemPatch{Status: ptr(domain.StatusInoperative)}); err != nil || got.Status != domain.StatusInoperative {
		t.Errorf("after close: %v %+v", err, got)
	}
}

func (f fixture) location(t *testing.T, name string) domain.Location {
	t.Helper()
	l, err := f.svc.CreateLocation(t.Context(), name, domain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestCreateConsumableWithOpeningStockInSeveralLocations(t *testing.T) {
	f := newFixture(t)
	a, b := f.location(t, "Taller"), f.location(t, "Maleta y caja de Alejandro")
	it, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "Tornillos", CategoryID: f.cat.ID, Kind: domain.KindConsumable, Unit: ptr("u"),
		Stocks: []domain.StockInput{{LocationID: a.ID, Quantity: 5}, {LocationID: b.ID, Quantity: 5}}}})
	if err != nil {
		t.Fatal(err)
	}
	if it.StockCurrent != 10 || len(it.Stocks) != 2 || it.Stocks[0].LocationName != "Maleta y caja de Alejandro" || it.Stocks[1].LocationName != "Taller" {
		t.Fatalf("created: %+v", it)
	}
	got, _ := f.svc.GetItem(t.Context(), it.ID)
	if len(got.Stocks) != 2 {
		t.Errorf("stored: %+v", got)
	}
}

func TestCreateConsumableUnknownStockLocationIsAFieldError(t *testing.T) {
	f := newFixture(t)
	a := f.location(t, "Taller")
	_, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "Tornillos", CategoryID: f.cat.ID, Kind: domain.KindConsumable, Unit: ptr("u"),
		Stocks: []domain.StockInput{{LocationID: a.ID, Quantity: 5}, {LocationID: 999, Quantity: 1}}}})
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Fields["stocks[1].location_id"] == "" {
		t.Fatalf("err = %v", err)
	}
	_, err = f.svc.CreateItem(t.Context(), app.CreateItemInput{NewItemInput: domain.NewItemInput{
		Description: "Tornillos", CategoryID: f.cat.ID, Kind: domain.KindConsumable, Unit: ptr("u"),
		StockCurrent: ptr(0), LocationID: ptr(int64(999))}})
	if !errors.As(err, &ve) || ve.Fields["location_id"] == "" {
		t.Errorf("legacy dangling location: %v", err)
	}
}

func TestLegacyCreateWithoutLocationUsesTheDefaultLocationOnce(t *testing.T) {
	f := newFixture(t)
	for range 2 {
		it, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{NewItemInput: domain.NewItemInput{
			Description: "Cinta", CategoryID: f.cat.ID, Kind: domain.KindConsumable, Unit: ptr("u"), StockCurrent: ptr(3)}})
		if err != nil || len(it.Stocks) != 1 || it.Stocks[0].LocationName != "Sin ubicación" {
			t.Fatalf("created: %+v %v", it, err)
		}
	}
	locs, _ := f.svc.ListLocations(t.Context())
	if len(locs) != 1 {
		t.Errorf("default location duplicated: %+v", locs)
	}
}

func TestListFiltersConsumablesByAnyStockLocationAndAssetsByLocation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	a, b, c := f.location(t, "A"), f.location(t, "B"), f.location(t, "C")
	multi, _ := f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: domain.NewItemInput{Description: "Tornillos", CategoryID: f.cat.ID,
		Kind: domain.KindConsumable, Unit: ptr("u"), Stocks: []domain.StockInput{{LocationID: a.ID, Quantity: 5}, {LocationID: b.ID, Quantity: 5}}}})
	asset, _ := f.svc.CreateItem(ctx, app.CreateItemInput{NewItemInput: domain.NewItemInput{Description: "Taladro", CategoryID: f.cat.ID,
		Kind: domain.KindAsset, LocationID: &b.ID}})
	for loc, want := range map[int64][]string{a.ID: {multi.ID}, b.ID: {asset.ID, multi.ID}, c.ID: nil} {
		items, total, _, err := f.svc.ListItems(ctx, domain.ItemFilter{LocationID: &loc, Sort: domain.SortDescription})
		if err != nil || total != len(want) {
			t.Fatalf("location %d: %d items %v", loc, total, err)
		}
		// Taladro sorts before Tornillos.
		for i, id := range want {
			if items[i].ID != id {
				t.Errorf("location %d item %d = %s, want %s", loc, i, items[i].Description, id)
			}
		}
	}
}

func TestUpdateConsumableLocationIsNotApplicable(t *testing.T) {
	f := newFixture(t)
	a := f.location(t, "A")
	it, _ := f.svc.CreateItem(t.Context(), app.CreateItemInput{NewItemInput: domain.NewItemInput{Description: "Tornillos", CategoryID: f.cat.ID,
		Kind: domain.KindConsumable, Unit: ptr("u"), Stocks: []domain.StockInput{{LocationID: a.ID, Quantity: 5}}}})
	if _, err := f.svc.UpdateItem(t.Context(), it.ID, domain.ItemPatch{LocationID: domain.Nullable[int64]{Set: true, Value: a.ID}}); !errors.Is(err, domain.ErrNotApplicable) {
		t.Fatalf("err = %v", err)
	}
	upd, err := f.svc.UpdateItem(t.Context(), it.ID, domain.ItemPatch{StockMin: ptr(2)})
	if err != nil || upd.StockMin != 2 || len(upd.Stocks) != 1 || upd.Stocks[0].Quantity != 5 {
		t.Errorf("unrelated edit must keep the stock rows: %+v %v", upd, err)
	}
}

func TestLocationWithStockCannotBeDeleted(t *testing.T) {
	f := newFixture(t)
	a, free := f.location(t, "A"), f.location(t, "Libre")
	if _, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{NewItemInput: domain.NewItemInput{Description: "Tornillos", CategoryID: f.cat.ID,
		Kind: domain.KindConsumable, Unit: ptr("u"), Stocks: []domain.StockInput{{LocationID: a.ID, Quantity: 5}}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteLocation(t.Context(), a.ID); !errors.Is(err, domain.ErrLocationInUse) {
		t.Errorf("location with stock: %v", err)
	}
	if err := f.svc.DeleteLocation(t.Context(), free.ID); err != nil {
		t.Errorf("unused location: %v", err)
	}
}
