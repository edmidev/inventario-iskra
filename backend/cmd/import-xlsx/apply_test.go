package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	identitypg "github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	identitydomain "github.com/edgar/inventario/backend/internal/identity/domain"
	importerpg "github.com/edgar/inventario/backend/internal/importer/adapters/postgres"
	maintenancepg "github.com/edgar/inventario/backend/internal/maintenance/adapters/postgres"
	maintenanceapp "github.com/edgar/inventario/backend/internal/maintenance/app"
	maintdomain "github.com/edgar/inventario/backend/internal/maintenance/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	stockpg "github.com/edgar/inventario/backend/internal/stock/adapters/postgres"
	stockapp "github.com/edgar/inventario/backend/internal/stock/app"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

type pgEnv struct {
	url  string
	pool *pgxpool.Pool
}

// setupDB connects to TEST_DATABASE_URL, migrates and empties every table the
// importer touches, and creates an admin, an operator, a viewer and an
// inactive operator.
func setupDB(t *testing.T) pgEnv {
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, code_sequences, locations, categories, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	users := identitypg.New(pool)
	for _, u := range []identitydomain.User{
		{Name: "Admin", Email: "admin@example.test", PasswordHash: "x", Role: identitydomain.RoleAdmin, Active: true},
		{Name: "Operator", Email: "op@example.test", PasswordHash: "x", Role: identitydomain.RoleOperator, Active: true},
		{Name: "Viewer", Email: "viewer@example.test", PasswordHash: "x", Role: identitydomain.RoleViewer, Active: true},
		{Name: "Gone", Email: "gone@example.test", PasswordHash: "x", Role: identitydomain.RoleOperator, Active: false},
	} {
		u := u
		if err := users.Create(ctx, &u); err != nil {
			t.Fatal(err)
		}
	}
	return pgEnv{url: url, pool: pool}
}

func (e pgEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e pgEnv) cli(args ...string) (int, string, string) {
	var out, errb strings.Builder
	code := run(args, &out, &errb, func(k string) string {
		if k == "DATABASE_URL" {
			return e.url
		}
		return ""
	})
	return code, out.String(), errb.String()
}

// synthetic builds a workbook with the quirks of the real one: header
// offsets, fused cell, shifted consumables, a repeated block (the workshop
// stock of the same consumables, with one different quantity and one item
// only listed there), serial
// and text dates, noisy categories and an inactive sheet.
func synthetic(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	put := func(sheet string, row int, vals ...any) {
		cell, _ := excelize.CoordinatesToCellName(1, row)
		if err := f.SetSheetRow(sheet, cell, &vals); err != nil {
			t.Fatal(err)
		}
	}
	s1, s2, s3 := "Activos y Disponibles", "Maleta y caja de alejandro", "Inactivos y Reparación"
	_ = f.SetSheetName("Sheet1", s1)
	_, _ = f.NewSheet(s2)
	_, _ = f.NewSheet(s3)
	put(s1, 1, nil, "Código", "Descripción", "Categoría", "Estado", "Disponibilidad", "Stock Actual", "Último Mantenimiento")
	put(s1, 3, nil, "ISK-AAA-001", "soldadora", "maquinaria ", "funcional", "disponible ", nil, 46283)
	put(s1, 4, nil, "ISK-AAA-002", "cortadora", "maquinaria/", "inoperativo", "no disponible")
	put(s1, 5, nil, "ISK-AAA-003", "Cilindro argon", "maquinaria", "LLENO", "disponible")
	put(s1, 6, nil, "ISK-AAA-004", "martillo", "herramientas", "funcional", "disponible")
	put(s2, 2, "Código", "Descripción", "Categoría", "Estado", "Disponibilidad", "Stock Actual", "Último Mantenimiento")
	put(s2, 3, "ISK-M1-LLTU", "llave de tubo", nil, nil, nil, 1)
	put(s2, 4, "ISK-M16-PAR        pinza para anillos", nil, nil, nil, 1)
	put(s2, 5, nil, "repuestos de segueta", nil, nil, nil, 3)
	put(s2, 6, nil, "CONSUMIBLES ")
	put(s2, 7, "ISK-MD1-PT", "PASTA TERMICA", nil, nil, nil, 2)
	put(s2, 8, "capuchones amarillos", "electricidad", nil, nil, 6)
	put(s2, 9, "terminales azul", nil, nil, nil, 74)
	put(s2, 10, "ISK-MD1-PT", "PASTA TERMICA", nil, nil, nil, 2)
	put(s2, 11, "capuchones amarillos", "electricidad", nil, nil, 6)
	put(s2, 12, "terminales azul", nil, nil, nil, 70)
	put(s2, 13, "solo en taller", nil, nil, nil, 5)
	put(s3, 1, "Código", "Descripción", "Categoría", "Estado", "Fecha de Falla", "Destino Inactivos")
	put(s3, 2, "ISK-HER-013", "Llave de pipa (Fisura)", "Herramientas", "No Funcional", "28/08/2026", "Taller de Reparación Interno")
	put(s3, 3, "ISK-HER-014", "Pinza de presión (Tornillo barrido)", "Herramientas", "No Funcional", "02/09/2026", "Espera de Repuestos")
	path := filepath.Join(t.TempDir(), "synthetic.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

// Totals of the synthetic workbook: 4+2 coded assets on sheets 1/2 plus the
// two inactive ones = 8 assets; consumables: repuestos (maleta 3), pasta
// (maleta 2 + taller 2), capuchones (6 + 6), terminales (74 + 70) and
// "solo en taller" (taller 5) = 5 items, 8 stock rows, 172 units.
const (
	wantAssets        = 8
	wantConsumables   = 5
	wantStockRows     = 8
	wantConsumableQty = 3 + 4 + 12 + 144 + 5
)

func TestApplyCreatesEverythingThroughTheServices(t *testing.T) {
	e := setupDB(t)
	wb := synthetic(t)

	code, _, errs := e.cli("--file", wb, "--apply", "--user-email", "Op@Example.test", "--timezone", "UTC")
	if code != 0 {
		t.Fatalf("apply exit %d: %s", code, errs)
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE kind='asset'`); n != wantAssets {
		t.Errorf("assets = %d, want %d", n, wantAssets)
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE kind='consumable'`); n != wantConsumables {
		t.Errorf("consumables = %d, want %d", n, wantConsumables)
	}
	if n := e.count(t, `SELECT count(*) FROM categories`); n != 4 {
		t.Errorf("categories = %d, want 4", n)
	}
	if n := e.count(t, `SELECT count(*) FROM locations WHERE (name='Taller' AND kind='storage') OR (name='Maleta y caja de Alejandro' AND kind='container')`); n != 2 {
		t.Errorf("locations = %d, want 2", n)
	}
	// Legacy codes are kept, fused cell split, noisy category normalized.
	if n := e.count(t, `SELECT count(*) FROM items i JOIN categories c ON c.id=i.category_id WHERE i.code='ISK-AAA-002' AND c.name='Maquinaria' AND i.status='inoperative'`); n != 1 {
		t.Error("ISK-AAA-002 not imported as inoperative Maquinaria")
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE code='ISK-M16-PAR' AND description='pinza para anillos'`); n != 1 {
		t.Error("fused code+description not split")
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE code='ISK-AAA-003' AND attributes->>'nivel'='lleno'`); n != 1 {
		t.Error("cylinder level missing")
	}
	// Provenance on every item.
	if n := e.count(t, `SELECT count(*) FROM items WHERE attributes->>'import_source' LIKE 'xlsx:%:r%'`); n != wantAssets+wantConsumables {
		t.Errorf("items with import_source = %d", n)
	}
	// Generated codes come from the category prefix (uncoded rows).
	if n := e.count(t, `SELECT count(*) FROM items WHERE code IN ('ISK-CON-001','ISK-CON-002','ISK-CON-003','ISK-ELE-001')`); n != 4 {
		t.Errorf("generated codes = %d, want 4 (one per item, not per occurrence)", n)
	}
	// last_maintenance_at from the Excel serial (midnight UTC here).
	if n := e.count(t, `SELECT count(*) FROM items WHERE code='ISK-AAA-001' AND last_maintenance_at = '2026-09-18T00:00:00Z'`); n != 1 {
		t.Error("last_maintenance_at not set from the serial date")
	}
	// Open corrective records for the inactive sheet, statuses by destination.
	if n := e.count(t, `SELECT count(*) FROM maintenance_records WHERE kind='corrective' AND status='open'`); n != 2 {
		t.Errorf("open failure records = %d, want 2", n)
	}
	if n := e.count(t, `SELECT count(*) FROM items i JOIN maintenance_records m ON m.item_id=i.id WHERE i.code='ISK-HER-013' AND i.status='in_repair' AND m.reported_at='2026-08-28T00:00:00Z' AND m.destination='Taller de Reparación Interno'`); n != 1 {
		t.Error("ISK-HER-013 record/status wrong")
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE code='ISK-HER-014' AND status='inoperative'`); n != 1 {
		t.Error("ISK-HER-014 should be inoperative")
	}
	// Opening movements for consumables, attributed to the operator.
	if n := e.count(t, `SELECT count(*) FROM stock_movements m JOIN users u ON u.id=m.user_id WHERE m.type='opening' AND u.email='op@example.test'`); n != wantStockRows {
		t.Errorf("opening movements = %d, want %d (one per stock row)", n, wantStockRows)
	}
	if n := e.count(t, `SELECT coalesce(sum(stock_current),0) FROM items WHERE kind='consumable'`); n != wantConsumableQty {
		t.Errorf("consumable stock = %d, want %d", n, wantConsumableQty)
	}
	if n := e.count(t, `SELECT count(*) FROM maintenance_records m JOIN users u ON u.id=m.reported_by WHERE u.email='op@example.test'`); n != 2 {
		t.Error("maintenance reporter should be the operator")
	}
}

func TestRepeatedConsumablesBecomeOneItemWithOneStockRowPerLocation(t *testing.T) {
	e := setupDB(t)
	code, out, errs := e.cli("--file", synthetic(t), "--apply", "--user-email", "admin@example.test", "--timezone", "UTC")
	if code != 0 {
		t.Fatalf("apply exit %d: %s", code, errs)
	}
	// terminales: one item, a stock row per location, total = sum, two openings.
	const terminales = `FROM items i WHERE i.description = 'terminales azul'`
	if n := e.count(t, `SELECT count(*) `+terminales); n != 1 {
		t.Fatalf("terminales items = %d, want 1", n)
	}
	if n := e.count(t, `SELECT count(*) FROM item_stocks s JOIN items i ON i.id=s.item_id JOIN locations l ON l.id=s.location_id
		WHERE i.description='terminales azul' AND ((l.name='Maleta y caja de Alejandro' AND s.quantity=74) OR (l.name='Taller' AND s.quantity=70))`); n != 2 {
		t.Errorf("terminales stock rows = %d, want 2", n)
	}
	if n := e.count(t, `SELECT stock_current `+terminales); n != 144 {
		t.Errorf("terminales stock_current = %d, want 144", n)
	}
	if n := e.count(t, `SELECT count(*) FROM stock_movements m JOIN items i ON i.id=m.item_id WHERE i.description='terminales azul' AND m.type='opening'`); n != 2 {
		t.Errorf("terminales opening movements = %d, want 2", n)
	}
	if n := e.count(t, `SELECT count(*) FROM items i WHERE i.code='ISK-MD1-PT' AND attributes->>'import_source'='xlsx:Maleta y caja de alejandro:r7' AND attributes->>'import_source_repeat'='xlsx:Maleta y caja de alejandro:r10'`); n != 1 {
		t.Error("provenance of both occurrences not kept on ISK-MD1-PT")
	}
	// An item only listed in the repeated block lives only in the workshop.
	if n := e.count(t, `SELECT count(*) FROM item_stocks s JOIN items i ON i.id=s.item_id JOIN locations l ON l.id=s.location_id WHERE i.description='solo en taller' AND l.name='Taller' AND s.quantity=5`); n != 1 {
		t.Error("second-only item not stocked at Taller")
	}
	// Per-location totals (the invariant stock_current = sum is also enforced by a trigger).
	if n := e.count(t, `SELECT coalesce(sum(s.quantity),0) FROM item_stocks s JOIN locations l ON l.id=s.location_id WHERE l.name='Maleta y caja de Alejandro'`); n != 3+2+6+74 {
		t.Errorf("maleta units = %d", n)
	}
	if n := e.count(t, `SELECT coalesce(sum(s.quantity),0) FROM item_stocks s JOIN locations l ON l.id=s.location_id WHERE l.name='Taller'`); n != 2+6+70+5 {
		t.Errorf("taller units = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM locations`); n != 2 {
		t.Errorf("locations = %d, want 2 (the workshop is created once and shared with sheets 1 and 3)", n)
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE kind='asset' AND location_id=(SELECT id FROM locations WHERE name='Taller')`); n != 6 {
		t.Errorf("sheet 1 and 3 assets at the default location Taller = %d, want 6", n)
	}
	if !strings.Contains(out, "Items created: 13 (assets 8, consumables 5)") || !strings.Contains(out, "items with two stock rows: 3") {
		t.Errorf("report does not summarize the split:\n%s", out)
	}
}

func TestPartiallyImportedItemGetsOnlyItsMissingStockRow(t *testing.T) {
	e := setupDB(t)
	ctx := httpx.WithPrincipal(context.Background(), httpx.Principal{UserID: "", Role: "admin"})
	deps := importerpg.NewDeps(e.pool)
	cat, err := deps.Catalog.CreateCategory(ctx, "Consumibles", "CON")
	if err != nil {
		t.Fatal(err)
	}
	maleta, err := deps.Catalog.CreateLocation(ctx, "Maleta y caja de Alejandro", catalogdomain.LocationContainer, nil)
	if err != nil {
		t.Fatal(err)
	}
	// What an import made before the repeated block was understood left behind.
	unit := "unidad"
	if _, err := deps.Catalog.CreateItem(ctx, catalogapp.CreateItemInput{Code: "ISK-MD1-PT", NewItemInput: catalogdomain.NewItemInput{
		Description: "PASTA TERMICA", CategoryID: cat.ID, Kind: catalogdomain.KindConsumable, Unit: &unit,
		Stocks:     []catalogdomain.StockInput{{LocationID: maleta.ID, Quantity: 2}},
		Attributes: map[string]any{"import_source": "xlsx:Maleta y caja de alejandro:r7"},
	}}); err != nil {
		t.Fatal(err)
	}

	code, out, errs := e.cli("--file", synthetic(t), "--apply", "--user-email", "admin@example.test")
	if code != 0 {
		t.Fatalf("apply: %d %s", code, errs)
	}
	if !strings.Contains(out, "needs stock row at Taller") || !strings.Contains(errs, "1 stock rows added to existing items") {
		t.Errorf("missing-stock-row not reported:\n%s\n%s", out, errs)
	}
	if n := e.count(t, `SELECT count(*) FROM item_stocks s JOIN items i ON i.id=s.item_id JOIN locations l ON l.id=s.location_id WHERE i.code='ISK-MD1-PT' AND l.name='Taller' AND s.quantity=2`); n != 1 {
		t.Error("workshop stock row not added")
	}
	if n := e.count(t, `SELECT count(*) FROM stock_movements m JOIN items i ON i.id=m.item_id WHERE i.code='ISK-MD1-PT' AND m.type='in' AND m.note LIKE 'Excel import:%'`); n != 1 {
		t.Error("the added row must be recorded as an in movement")
	}
	if n := e.count(t, `SELECT stock_current FROM items WHERE code='ISK-MD1-PT'`); n != 4 {
		t.Errorf("stock_current = %d, want 4", n)
	}
	// Consuming everything at the workshop must not make the next run add it back.
	var itemID string
	_ = e.pool.QueryRow(context.Background(), `SELECT id::text FROM items WHERE code='ISK-MD1-PT'`).Scan(&itemID)
	var adminID string
	_ = e.pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE email='admin@example.test'`).Scan(&adminID)
	var tallerID int64
	_ = e.pool.QueryRow(context.Background(), `SELECT id FROM locations WHERE name='Taller'`).Scan(&tallerID)
	if _, err := stockapp.NewService(stockpg.New(e.pool)).Record(context.Background(), itemID, adminID, stockdomain.Request{Type: stockdomain.TypeOut, Qty: 2, LocationID: &tallerID}); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := e.cli("--file", synthetic(t), "--apply", "--user-email", "admin@example.test"); code != 0 {
		t.Fatalf("rerun: %d %s", code, errs)
	}
	if n := e.count(t, `SELECT stock_current FROM items WHERE code='ISK-MD1-PT'`); n != 2 {
		t.Errorf("a consumed stock row was resurrected: stock_current = %d, want 2", n)
	}
}

func TestSecondApplyCreatesNothingAndNeverReopensAClosedRepair(t *testing.T) {
	e := setupDB(t)
	wb := synthetic(t)
	if code, _, errs := e.cli("--file", wb, "--apply", "--user-email", "admin@example.test"); code != 0 {
		t.Fatalf("first apply: %d %s", code, errs)
	}
	// The user closes one repair between the runs.
	ctx := context.Background()
	var recID int64
	if err := e.pool.QueryRow(ctx, `SELECT id FROM maintenance_records WHERE status='open' LIMIT 1`).Scan(&recID); err != nil {
		t.Fatal(err)
	}
	var adminID string
	_ = e.pool.QueryRow(ctx, `SELECT id::text FROM users WHERE email='admin@example.test'`).Scan(&adminID)
	maint := maintenanceapp.NewService(maintenancepg.New(e.pool), nil)
	if _, err := maint.Close(ctx, recID, adminID, maintdomain.CloseRequest{ResolutionNote: "fixed"}); err != nil {
		t.Fatal(err)
	}

	items := e.count(t, `SELECT count(*) FROM items`)
	moves := e.count(t, `SELECT count(*) FROM stock_movements`)
	recs := e.count(t, `SELECT count(*) FROM maintenance_records`)
	code, out, errs := e.cli("--file", wb, "--apply", "--user-email", "admin@example.test")
	if code != 0 {
		t.Fatalf("second apply: %d %s", code, errs)
	}
	if !strings.Contains(errs, "0 items created") || !strings.Contains(errs, "13 already imported") {
		t.Errorf("summary = %q", errs)
	}
	if !strings.Contains(out, "Items created: 0") || !strings.Contains(out, "already_imported") {
		t.Errorf("report does not show the idempotent run")
	}
	if e.count(t, `SELECT count(*) FROM items`) != items || e.count(t, `SELECT count(*) FROM stock_movements`) != moves ||
		e.count(t, `SELECT count(*) FROM item_stocks`) != wantStockRows || e.count(t, `SELECT count(*) FROM locations`) != 2 ||
		e.count(t, `SELECT count(*) FROM maintenance_records`) != recs || e.count(t, `SELECT count(*) FROM categories`) != 4 {
		t.Error("second apply changed the database")
	}
}

func TestCodeConflictsWithExistingItemsAreReportedAndSkipped(t *testing.T) {
	e := setupDB(t)
	ctx := context.Background()
	deps := importerpg.NewDeps(e.pool)
	cat, err := deps.Catalog.CreateCategory(ctx, "Herramientas", "HER")
	if err != nil {
		t.Fatal(err)
	}
	// Same code as workbook row ISK-AAA-004, but a different thing.
	if _, err := deps.Catalog.CreateItem(ctx, createItem("ISK-AAA-004", "serrucho manual", cat.ID)); err != nil {
		t.Fatal(err)
	}

	code, out, errs := e.cli("--file", synthetic(t), "--apply", "--user-email", "admin@example.test")
	if code != 0 {
		t.Fatalf("apply: %d %s", code, errs)
	}
	if !strings.Contains(errs, "1 conflicts") || !strings.Contains(out, "already belongs to another item in the database") {
		t.Errorf("conflict not reported: %q", errs)
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE code='ISK-AAA-004' AND description='serrucho manual'`); n != 1 {
		t.Error("existing item was touched")
	}
	if n := e.count(t, `SELECT count(*) FROM items`); n != wantAssets+wantConsumables { // 12 new + 1 existing
		t.Errorf("items = %d", n)
	}
}

func TestApplyRequiresAnActiveAdminOrOperatorAndWritesNothingOtherwise(t *testing.T) {
	e := setupDB(t)
	wb := synthetic(t)
	if code, _, errs := e.cli("--file", wb, "--apply"); code != 2 || !strings.Contains(errs, "--user-email") {
		t.Errorf("missing email: exit %d %q", code, errs)
	}
	for _, email := range []string{"nobody@example.test", "gone@example.test", "viewer@example.test", "not-an-email"} {
		if code, _, errs := e.cli("--file", wb, "--apply", "--user-email", email); code != 1 || errs == "" {
			t.Errorf("%s: exit %d %q, want 1", email, code, errs)
		}
	}
	var out, errb strings.Builder
	if code := run([]string{"--file", wb, "--apply", "--user-email", "admin@example.test"}, &out, &errb, func(string) string { return "" }); code != 2 {
		t.Errorf("apply without DATABASE_URL: exit %d", code)
	}
	if n := e.count(t, `SELECT count(*) FROM items`) + e.count(t, `SELECT count(*) FROM categories`); n != 0 {
		t.Errorf("rejected runs wrote %d rows", n)
	}
}

func TestDryRunWithDatabaseWritesNothingButSeesExistingState(t *testing.T) {
	e := setupDB(t)
	ctx := context.Background()
	deps := importerpg.NewDeps(e.pool)
	if _, err := deps.Catalog.CreateCategory(ctx, "maquinaria", "MAQ"); err != nil {
		t.Fatal(err)
	}
	code, out, errs := e.cli("--file", synthetic(t), "--timezone", "UTC")
	if code != 0 {
		t.Fatalf("dry-run: %d %s", code, errs)
	}
	if !strings.Contains(out, "Database consulted read-only") || !strings.Contains(out, "| category | Maquinaria | MAQ | no (already exists) |") ||
		!strings.Contains(out, "| category | Herramientas | HER | yes |") {
		t.Errorf("report does not reflect the database:\n%s", out)
	}
	if n := e.count(t, `SELECT count(*) FROM items`) + e.count(t, `SELECT count(*) FROM locations`) + e.count(t, `SELECT count(*) FROM stock_movements`); n != 0 {
		t.Errorf("dry-run wrote %d rows", n)
	}
	if n := e.count(t, `SELECT count(*) FROM categories`); n != 1 {
		t.Errorf("dry-run changed categories: %d", n)
	}
}

func TestEmptyDefaultLocationLeavesSheetOneAndThreeItemsWithoutLocation(t *testing.T) {
	e := setupDB(t)
	if code, _, errs := e.cli("--file", synthetic(t), "--apply", "--user-email", "admin@example.test", "--default-location", ""); code != 0 {
		t.Fatalf("apply: %d %s", code, errs)
	}
	if n := e.count(t, `SELECT count(*) FROM items WHERE kind = 'asset' AND location_id IS NULL`); n != 6 { // 4 on sheet 1 + 2 on sheet 3
		t.Errorf("items without location = %d, want 6", n)
	}
	if n := e.count(t, `SELECT count(*) FROM locations`); n != 2 { // maleta and taller (from the repeated block)
		t.Errorf("locations = %d, want 2", n)
	}
}

func createItem(code, desc string, categoryID int64) catalogapp.CreateItemInput {
	return catalogapp.CreateItemInput{Code: code, NewItemInput: catalogdomain.NewItemInput{
		Description: desc, CategoryID: categoryID, Kind: catalogdomain.KindAsset,
	}}
}
