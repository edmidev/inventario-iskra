package postgres_test

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"
	"testing"

	"github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/catalog/app"
	"github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ptr[T any](v T) *T { return &v }

type fixture struct {
	repo *postgres.Repo
	svc  *app.Service
	pool *pgxpool.Pool
	cat  domain.Category
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
	// Only catalog tables; FK order is handled by TRUNCATE ... CASCADE.
	if _, err := pool.Exec(ctx, `TRUNCATE items, code_sequences, locations, categories RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	repo := postgres.New(pool)
	svc := app.NewService(repo.Categories(), repo.Locations(), repo.Items())
	cat, err := svc.CreateCategory(ctx, "Herramientas", "HER")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{repo: repo, svc: svc, pool: pool, cat: cat}
}

func (f fixture) create(t *testing.T, in domain.NewItemInput, code string) domain.Item {
	t.Helper()
	if in.CategoryID == 0 {
		in.CategoryID = f.cat.ID
	}
	it, err := f.svc.CreateItem(t.Context(), app.CreateItemInput{Code: code, NewItemInput: in})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestCategoryConstraints(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	if _, err := f.svc.CreateCategory(ctx, "HERRAMIENTAS", "OTR"); !errors.Is(err, domain.ErrCategoryNameTaken) {
		t.Errorf("name (case-insensitive): %v", err)
	}
	if _, err := f.svc.CreateCategory(ctx, "Otra", "HER"); !errors.Is(err, domain.ErrCategoryPrefixTaken) {
		t.Errorf("prefix: %v", err)
	}
	// The database refuses malformed prefixes even if the app were bypassed.
	if _, err := f.pool.Exec(ctx, `INSERT INTO categories (name, code_prefix) VALUES ('X', 'a-b')`); err == nil {
		t.Error("db accepted a bad prefix")
	}
	other, _ := f.svc.CreateCategory(ctx, "Cables", "CAB")
	if _, err := f.svc.UpdateCategory(ctx, other.ID, ptr("herramientas"), nil); !errors.Is(err, domain.ErrCategoryNameTaken) {
		t.Errorf("rename: %v", err)
	}
	if _, err := f.svc.UpdateCategory(ctx, 9999, ptr("x"), nil); !errors.Is(err, domain.ErrCategoryNotFound) {
		t.Errorf("missing: %v", err)
	}
	cats, _ := f.svc.ListCategories(ctx)
	if len(cats) != 2 || cats[0].Name != "Cables" {
		t.Errorf("list order: %+v", cats)
	}
}

func TestCategoryDeleteBlockedByItemsIncludingDeleted(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	it := f.create(t, domain.NewItemInput{Description: "Taladro", Kind: domain.KindAsset}, "")
	if err := f.svc.DeleteCategory(ctx, f.cat.ID); !errors.Is(err, domain.ErrCategoryInUse) {
		t.Fatalf("with item: %v", err)
	}
	if err := f.svc.DeleteItem(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCategory(ctx, f.cat.ID); !errors.Is(err, domain.ErrCategoryInUse) {
		t.Fatalf("with soft-deleted item: %v", err)
	}
	empty, _ := f.svc.CreateCategory(ctx, "Vacía", "VAC")
	if err := f.svc.DeleteCategory(ctx, empty.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCategory(ctx, empty.ID); !errors.Is(err, domain.ErrCategoryNotFound) {
		t.Fatalf("again: %v", err)
	}
}

func TestLocationsHierarchyAndDelete(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	root, err := f.svc.CreateLocation(ctx, "Bodega", domain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, _ := f.svc.CreateLocation(ctx, "Maleta de Alejandro", domain.LocationContainer, &root.ID)
	if _, err := f.svc.CreateLocation(ctx, "x", domain.LocationStorage, ptr(int64(9999))); err == nil {
		t.Error("missing parent accepted")
	}
	cycle := app.LocationPatch{ParentID: domain.Nullable[int64]{Set: true, Value: child.ID}}
	if _, err := f.svc.UpdateLocation(ctx, root.ID, cycle); !errors.Is(err, domain.ErrLocationCycle) {
		t.Errorf("cycle: %v", err)
	}
	if err := f.svc.DeleteLocation(ctx, root.ID); !errors.Is(err, domain.ErrLocationInUse) {
		t.Errorf("parent delete: %v", err)
	}
	f.create(t, domain.NewItemInput{Description: "Taladro", Kind: domain.KindAsset, LocationID: &child.ID}, "")
	if err := f.svc.DeleteLocation(ctx, child.ID); !errors.Is(err, domain.ErrLocationInUse) {
		t.Errorf("with items: %v", err)
	}
	leaf, _ := f.svc.CreateLocation(ctx, "Hoja", domain.LocationStorage, &root.ID)
	if err := f.svc.DeleteLocation(ctx, leaf.ID); err != nil {
		t.Errorf("leaf: %v", err)
	}
	if err := f.svc.DeleteLocation(ctx, leaf.ID); !errors.Is(err, domain.ErrLocationNotFound) {
		t.Errorf("again: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE locations SET parent_id = id WHERE id = $1`, root.ID); err == nil {
		t.Error("db accepted a self-parent")
	}
}

func TestItemRoundTripAndKindConstraints(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	loc, _ := f.svc.CreateLocation(ctx, "Bodega", domain.LocationStorage, nil)
	it := f.create(t, domain.NewItemInput{
		Description: "Taladro", Kind: domain.KindAsset, LocationID: &loc.ID, MaintenanceIntervalDays: ptr(90),
		Attributes: map[string]any{"marca": "Bosch", "nivel": float64(3)},
	}, "")
	if it.Code != "ISK-HER-001" || it.ID == "" || it.CreatedAt.IsZero() || it.StockCurrent != 1 {
		t.Fatalf("created: %+v", it)
	}
	got, err := f.svc.GetItem(ctx, it.ID)
	if err != nil || got.Attributes["marca"] != "Bosch" || got.Attributes["nivel"] != float64(3) ||
		*got.LocationID != loc.ID || *got.MaintenanceIntervalDays != 90 {
		t.Fatalf("get: %+v %v", got, err)
	}
	byCode, err := f.svc.GetItemByCode(ctx, "ISK-HER-001")
	if err != nil || byCode.ID != it.ID {
		t.Fatalf("by code: %+v %v", byCode, err)
	}
	if _, err := f.svc.GetItem(ctx, "not-a-uuid"); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("bad id: %v", err)
	}

	upd, err := f.svc.UpdateItem(ctx, it.ID, domain.ItemPatch{Status: ptr(domain.StatusInRepair), Notes: ptr("falla"),
		Attributes: map[string]any{"marca": "Makita"}})
	if err != nil || upd.Status != domain.StatusInRepair || upd.Attributes["marca"] != "Makita" || !upd.UpdatedAt.After(it.UpdatedAt) {
		t.Fatalf("update: %+v %v", upd, err)
	}

	// The database enforces the kind rules independently of the application.
	if _, err := f.pool.Exec(ctx, `UPDATE items SET stock_current = 5 WHERE id = $1`, it.ID); err == nil {
		t.Error("db accepted an asset with stock 5")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE items SET kind = 'consumable' WHERE id = $1`, it.ID); err == nil {
		t.Error("db accepted a consumable without unit")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE items SET attributes = '[]' WHERE id = $1`, it.ID); err == nil {
		t.Error("db accepted non-object attributes")
	}
}

func TestUpdateNeverTouchesStockOrCode(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	cons := f.create(t, domain.NewItemInput{Description: "Cinta", Kind: domain.KindConsumable, Unit: ptr("rollo"), StockCurrent: ptr(8)}, "")
	// Simulate a stock movement (T3) landing between read and write.
	if _, err := f.pool.Exec(ctx, `UPDATE items SET stock_current = 20 WHERE id = $1`, cons.ID); err != nil {
		t.Fatal(err)
	}
	stale := cons
	stale.StockMin = 4
	stale.Code, stale.StockCurrent = "ISK-HACK-1", 8
	if err := f.repo.Items().Update(ctx, &stale); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.GetItem(ctx, cons.ID)
	if got.StockCurrent != 20 || got.Code != cons.Code || got.StockMin != 4 {
		t.Fatalf("got %+v", got)
	}
}

func TestForeignKeysAndCodeUniqueness(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	repo := f.repo.Items()
	bad := domain.Item{Code: "ISK-X-1", Description: "x", CategoryID: 9999, Kind: domain.KindAsset, Status: domain.StatusFunctional, StockCurrent: 1, Attributes: map[string]any{}}
	if err := repo.Create(ctx, &bad); !errors.Is(err, domain.ErrCategoryNotFound) {
		t.Errorf("category fk: %v", err)
	}
	bad.CategoryID, bad.LocationID = f.cat.ID, ptr(int64(9999))
	if err := repo.Create(ctx, &bad); !errors.Is(err, domain.ErrLocationNotFound) {
		t.Errorf("location fk: %v", err)
	}
	it := f.create(t, domain.NewItemInput{Description: "Taladro", Kind: domain.KindAsset}, "ISK-OLD-1")
	dup := domain.Item{Code: "ISK-OLD-1", Description: "x", CategoryID: f.cat.ID, Kind: domain.KindAsset, Status: domain.StatusFunctional, StockCurrent: 1, Attributes: map[string]any{}}
	if err := repo.Create(ctx, &dup); !errors.Is(err, domain.ErrCodeTaken) {
		t.Errorf("dup code: %v", err)
	}
	if err := f.svc.DeleteItem(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, &dup); !errors.Is(err, domain.ErrCodeTaken) {
		t.Errorf("deleted code must stay reserved: %v", err)
	}
	if _, err := f.svc.UpdateItem(ctx, it.ID, domain.ItemPatch{}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("update deleted: %v", err)
	}
	if err := f.svc.DeleteItem(ctx, it.ID); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("delete again: %v", err)
	}
	live := f.create(t, domain.NewItemInput{Description: "Martillo", Kind: domain.KindAsset}, "")
	_, err := f.svc.UpdateItem(ctx, live.ID, domain.ItemPatch{LocationID: domain.Nullable[int64]{Set: true, Value: 9999}})
	if err == nil {
		t.Error("update accepted a missing location")
	}
}

func TestConcurrentCodeGenerationYieldsUniqueSequentialCodes(t *testing.T) {
	f := setup(t)
	const n = 30
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
	sort.Strings(codes)
	for i, c := range codes {
		want := domain.GenerateCode("HER", i+1)
		if c != want {
			t.Fatalf("codes[%d] = %q want %q (all: %v)", i, c, want, codes)
		}
	}
}

func TestGeneratedCodeSkipsSuppliedCodes(t *testing.T) {
	f := setup(t)
	f.create(t, domain.NewItemInput{Description: "a", Kind: domain.KindAsset}, "ISK-HER-001")
	f.create(t, domain.NewItemInput{Description: "b", Kind: domain.KindAsset}, "ISK-HER-002")
	if got := f.create(t, domain.NewItemInput{Description: "c", Kind: domain.KindAsset}, ""); got.Code != "ISK-HER-003" {
		t.Fatalf("got %s", got.Code)
	}
}

func TestSearchFiltersSortAndPaging(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	cab, _ := f.svc.CreateCategory(ctx, "Cables", "CAB")
	bodega, _ := f.svc.CreateLocation(ctx, "Bodega", domain.LocationStorage, nil)
	f.create(t, domain.NewItemInput{Description: "Taladro percutor", Kind: domain.KindAsset, LocationID: &bodega.ID}, "")
	f.create(t, domain.NewItemInput{Description: "Martillo", Kind: domain.KindAsset, Status: domain.StatusInRepair}, "")
	f.create(t, domain.NewItemInput{Description: "Cable HDMI", CategoryID: cab.ID, Kind: domain.KindConsumable, Unit: ptr("unidad"), StockCurrent: ptr(1), StockMin: 2}, "")
	f.create(t, domain.NewItemInput{Description: "Cinta aislante", CategoryID: cab.ID, Kind: domain.KindConsumable, Unit: ptr("rollo"), StockCurrent: ptr(30), StockMin: 2}, "")
	f.create(t, domain.NewItemInput{Description: "Rollo 100%_x", CategoryID: cab.ID, Kind: domain.KindConsumable, Unit: ptr("rollo"), StockCurrent: ptr(2)}, "")
	gone := f.create(t, domain.NewItemInput{Description: "Taladro viejo", Kind: domain.KindAsset}, "")
	if err := f.svc.DeleteItem(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	list := func(fl domain.ItemFilter) ([]domain.Item, int) {
		items, total, _, err := f.svc.ListItems(ctx, fl)
		if err != nil {
			t.Fatal(err)
		}
		return items, total
	}
	cases := []struct {
		name string
		f    domain.ItemFilter
		want int
	}{
		{"all (deleted excluded)", domain.ItemFilter{}, 5},
		{"q description, case-insensitive", domain.ItemFilter{Q: "TALADRO"}, 1},
		{"q code", domain.ItemFilter{Q: "isk-cab"}, 3},
		{"q escapes % and _", domain.ItemFilter{Q: "100%_x"}, 1},
		{"q bare % matches literally", domain.ItemFilter{Q: "%"}, 1},
		{"q bare _ matches literally", domain.ItemFilter{Q: "_"}, 1},
		{"category", domain.ItemFilter{CategoryID: &cab.ID}, 3},
		{"status", domain.ItemFilter{Status: domain.StatusInRepair}, 1},
		{"kind", domain.ItemFilter{Kind: domain.KindConsumable}, 3},
		{"available", domain.ItemFilter{Availability: domain.Available}, 4},
		{"unavailable", domain.ItemFilter{Availability: domain.Unavailable}, 1},
		{"location", domain.ItemFilter{LocationID: &bodega.ID}, 1},
		{"low stock (needs min > 0)", domain.ItemFilter{LowStock: true}, 1},
		{"combined", domain.ItemFilter{CategoryID: &cab.ID, Kind: domain.KindConsumable, LowStock: true, Q: "hdmi"}, 1},
	}
	for _, tc := range cases {
		nf, _ := tc.f.Normalize()
		items, total := list(nf)
		if total != tc.want || len(items) != tc.want {
			t.Errorf("%s: total %d len %d want %d", tc.name, total, len(items), tc.want)
		}
	}

	items, _ := list(domain.ItemFilter{Sort: domain.SortDescription})
	if items[0].Description != "Cable HDMI" || items[len(items)-1].Description != "Taladro percutor" {
		t.Errorf("description asc: %v", descs(items))
	}
	items, _ = list(domain.ItemFilter{Sort: domain.SortCode, Desc: true})
	if items[0].Code != "ISK-HER-002" {
		t.Errorf("code desc: %v", codesOf(items))
	}
	page, total := list(domain.ItemFilter{Limit: 2, Offset: 4})
	if total != 5 || len(page) != 1 {
		t.Errorf("paging: total %d len %d", total, len(page))
	}
	page, total = list(domain.ItemFilter{Offset: 99})
	if total != 5 || len(page) != 0 {
		t.Errorf("past end: total %d len %d", total, len(page))
	}
	if _, err := f.svc.UpdateItem(ctx, items[0].ID, domain.ItemPatch{Notes: ptr("tocado")}); err != nil {
		t.Fatal(err)
	}
	newest, _ := list(domain.ItemFilter{Sort: domain.SortUpdatedAt, Desc: true, Limit: 1})
	if newest[0].Notes != "tocado" {
		t.Errorf("updated_at desc: %+v", newest[0])
	}
}

func descs(items []domain.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Description
	}
	return out
}

func codesOf(items []domain.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Code
	}
	return out
}
