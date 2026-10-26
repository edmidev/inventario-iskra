package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/purchasing/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/purchasing/app"
	"github.com/edgar/inventario/backend/internal/purchasing/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sqlCatalog is an app.ItemCatalog that writes items with plain SQL through the
// transaction carried by the context, so the tests can prove that what the
// receive flow creates commits or rolls back together with the order. It can be
// told to fail on the Nth CreateItem call.
type sqlCatalog struct {
	pool       *pgxpool.Pool
	categoryID int64
	seq        atomic.Int64
	calls      atomic.Int64
	failOnNth  atomic.Int64
}

func (c *sqlCatalog) ValidateNewItem(context.Context, app.NewItemSpec) error { return nil }

func (c *sqlCatalog) CreateItem(ctx context.Context, spec app.NewItemSpec, loc int64) (app.CreatedItem, error) {
	if n := c.failOnNth.Load(); n != 0 && c.calls.Add(1) == n {
		return app.CreatedItem{}, errors.New("injected catalog failure")
	}
	tx, commit, rollback, err := db.Begin(ctx, c.pool)
	if err != nil {
		return app.CreatedItem{}, err
	}
	defer rollback()
	code := fmt.Sprintf("ISK-PO-%03d", c.seq.Add(1))
	var id string
	if spec.Kind == domain.KindAsset {
		err = tx.QueryRow(ctx,
			`INSERT INTO items (code, description, category_id, kind, stock_current, location_id)
			 VALUES ($1, $2, $3, 'asset', 1, $4) RETURNING id::text`, code, spec.Description, c.categoryID, loc).Scan(&id)
	} else {
		err = tx.QueryRow(ctx,
			`INSERT INTO items (code, description, category_id, kind, unit, stock_current)
			 VALUES ($1, $2, $3, 'consumable', 'u', 0) RETURNING id::text`, code, spec.Description, c.categoryID).Scan(&id)
	}
	if err != nil {
		return app.CreatedItem{}, err
	}
	return app.CreatedItem{ID: id, Code: code}, commit()
}

func (c *sqlCatalog) ItemInfo(ctx context.Context, id string) (app.ItemInfo, error) {
	var i app.ItemInfo
	err := c.pool.QueryRow(ctx, `SELECT kind, description FROM items WHERE id = $1::uuid`, id).Scan(&i.Kind, &i.Description)
	if err != nil {
		return app.ItemInfo{}, app.ErrItemNotFound
	}
	return i, nil
}

func (c *sqlCatalog) LocationExists(ctx context.Context, id int64) (bool, error) {
	var ok bool
	err := c.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM locations WHERE id = $1)`, id).Scan(&ok)
	return ok, err
}

// sqlStock records `in` movements with plain SQL inside the ambient transaction.
type sqlStock struct {
	pool *pgxpool.Pool
	fail atomic.Bool
}

func (s *sqlStock) ReceiveIn(ctx context.Context, itemID, userID string, loc int64, qty int, note string) error {
	if s.fail.Load() {
		return errors.New("injected stock failure")
	}
	tx, commit, rollback, err := db.Begin(ctx, s.pool)
	if err != nil {
		return err
	}
	defer rollback()
	var before int
	if err := tx.QueryRow(ctx, `SELECT stock_current FROM items WHERE id = $1::uuid FOR UPDATE`, itemID).Scan(&before); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE items SET stock_current = $2 WHERE id = $1::uuid`, itemID, before+qty); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO item_stocks (item_id, location_id, quantity) VALUES ($1::uuid, $2, $3)
		 ON CONFLICT (item_id, location_id) DO UPDATE SET quantity = item_stocks.quantity + EXCLUDED.quantity`,
		itemID, loc, qty); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, location_id, user_id, note)
		 VALUES ($1::uuid, 'in', $2, $3, $4, $5, NULLIF($6, '')::uuid, $7)`,
		itemID, qty, before, before+qty, loc, userID, note); err != nil {
		return err
	}
	return commit()
}

// sqlSuppliers reads suppliers straight from the table, honoring soft delete.
type sqlSuppliers struct{ pool *pgxpool.Pool }

func (s sqlSuppliers) Get(ctx context.Context, id int64) (string, bool, error) {
	var name string
	var active bool
	err := s.pool.QueryRow(ctx, `SELECT name, active FROM suppliers WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&name, &active)
	if err != nil {
		return "", false, app.ErrSupplierNotFound
	}
	return name, active, nil
}

type fixture struct {
	pool     *pgxpool.Pool
	svc      *app.Service
	cat      *sqlCatalog
	stock    *sqlStock
	userID   string
	supplier int64 // active
	inactive int64
	loc      int64
	consID   string // an existing consumable
	assetID  string // an existing asset
}

func (f fixture) q(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
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
	if _, err := pool.Exec(ctx, `TRUNCATE suppliers, items, code_sequences, locations, categories, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	f := fixture{pool: pool}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO users (name, email, password_hash, role) VALUES ('Ana', 'ana@example.com', 'x', 'operator') RETURNING id::text`).Scan(&f.userID))
	must(pool.QueryRow(ctx, `INSERT INTO suppliers (name, active) VALUES ('Ferretería Sur', TRUE) RETURNING id`).Scan(&f.supplier))
	must(pool.QueryRow(ctx, `INSERT INTO suppliers (name, active) VALUES ('Norte', FALSE) RETURNING id`).Scan(&f.inactive))
	must(pool.QueryRow(ctx, `INSERT INTO locations (name, kind) VALUES ('Bodega', 'storage') RETURNING id`).Scan(&f.loc))
	var cat int64
	must(pool.QueryRow(ctx, `INSERT INTO categories (name, code_prefix) VALUES ('PO', 'PO') RETURNING id`).Scan(&cat))
	must(pool.QueryRow(ctx, `INSERT INTO items (code, description, category_id, kind, unit, stock_current) VALUES ('ISK-X-001', 'Papel', $1, 'consumable', 'u', 0) RETURNING id::text`, cat).Scan(&f.consID))
	must(pool.QueryRow(ctx, `INSERT INTO items (code, description, category_id, kind, stock_current, location_id) VALUES ('ISK-X-002', 'Proyector', $1, 'asset', 1, $2) RETURNING id::text`, cat, f.loc).Scan(&f.assetID))

	f.cat = &sqlCatalog{pool: pool, categoryID: cat}
	f.stock = &sqlStock{pool: pool}
	f.svc = app.NewService(postgres.New(pool), f.cat, f.stock, sqlSuppliers{pool}, nil)
	return f
}

func laptops(n int) domain.LineInput {
	return domain.LineInput{NewItem: &domain.NewItem{Description: "Laptop", CategoryID: 1, Kind: domain.KindAsset}, Quantity: n, UnitCostMinor: 90000}
}

func ink(n int) domain.LineInput {
	return domain.LineInput{NewItem: &domain.NewItem{Description: "Tinta", CategoryID: 1, Kind: domain.KindConsumable, Unit: "u"}, Quantity: n, UnitCostMinor: 1200}
}

func (f fixture) paper(n int) domain.LineInput {
	return domain.LineInput{ItemID: f.consID, Quantity: n, UnitCostMinor: 450}
}

func (f fixture) placed(t *testing.T, lines ...domain.LineInput) domain.Order {
	t.Helper()
	ctx := context.Background()
	o, err := f.svc.Create(ctx, f.userID, domain.Input{SupplierID: f.supplier, Currency: domain.USD, Lines: lines})
	if err != nil {
		t.Fatal(err)
	}
	if o, err = f.svc.Place(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	return o
}

func (f fixture) receive(o domain.Order, key string, parts ...int) (app.ReceiveResult, error) {
	req := domain.ReceiveRequest{LocationID: f.loc, ClientKey: key}
	for i, q := range parts {
		if q > 0 {
			req.Lines = append(req.Lines, domain.ReceiveLine{LineID: o.Lines[i].ID, Quantity: q})
		}
	}
	return f.svc.Receive(context.Background(), o.ID, f.userID, req)
}

func TestFullLifecycleWithPartialReceipts(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	rate := 36.5
	exp := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	o, err := f.svc.Create(ctx, f.userID, domain.Input{
		SupplierID: f.supplier, Currency: domain.VES, ExchangeRate: &rate, ExpectedAt: &exp, Notes: " urgente ",
		Lines: []domain.LineInput{f.paper(10), laptops(2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != domain.StatusDraft || o.SupplierName != "Ferretería Sur" || *o.ExchangeRate != 36.5 ||
		!o.ExpectedAt.Equal(exp) || o.Notes != "urgente" || o.CreatedBy != f.userID || len(o.Lines) != 2 {
		t.Fatalf("created: %+v", o)
	}
	if o.Lines[0].Description != "Papel" || o.Lines[0].Position != 1 || o.Lines[1].NewItem == nil || o.Lines[1].NewItem.Kind != "asset" {
		t.Fatalf("lines: %+v", o.Lines)
	}
	if o.Total() != 10*450+2*90000 {
		t.Fatalf("total %d", o.Total())
	}

	// Editing the draft replaces the lines and keeps ids consistent.
	notes := "cambiado"
	o, err = f.svc.Update(ctx, o.ID, app.Patch{Notes: &notes, Lines: []domain.LineInput{f.paper(10), laptops(2), ink(5)}})
	if err != nil || len(o.Lines) != 3 || o.Notes != "cambiado" {
		t.Fatalf("update: %+v %v", o, err)
	}
	if o, err = f.svc.Place(ctx, o.ID); err != nil || o.Status != domain.StatusOrdered || o.OrderedAt == nil {
		t.Fatalf("place: %+v %v", o, err)
	}

	r1, err := f.receive(o, "", 4, 1, 0)
	if err != nil || r1.Replayed || r1.Order.Status != domain.StatusPartiallyReceived {
		t.Fatalf("r1: %+v %v", r1, err)
	}
	if len(r1.Receipt.Lines) != 2 || r1.Receipt.Lines[1].Items[0].Code == "" {
		t.Fatalf("receipt: %+v", r1.Receipt)
	}
	r2, err := f.receive(r1.Order, "", 6, 1, 5)
	if err != nil || r2.Order.Status != domain.StatusReceived {
		t.Fatalf("r2: %+v %v", r2, err)
	}

	got, err := f.svc.Get(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusReceived || len(got.Receipts) != 2 || got.Receipts[0].ReceivedBy != f.userID || got.Receipts[0].LocationID != f.loc {
		t.Fatalf("got: %+v", got)
	}
	for _, l := range got.Lines {
		if l.ReceivedQty != l.Quantity {
			t.Fatalf("line %d: %d/%d", l.ID, l.ReceivedQty, l.Quantity)
		}
	}
	if n := f.q(t, `SELECT stock_current FROM items WHERE id = $1::uuid`, f.consID); n != 10 {
		t.Fatalf("stock of the ordered consumable = %d", n)
	}
	if n := f.q(t, `SELECT count(*) FROM stock_movements WHERE type = 'in' AND note LIKE $1`, fmt.Sprintf("Purchase order #%d%%", o.ID)); n != 3 {
		t.Fatalf("in movements = %d", n)
	}
	if _, err := f.svc.Cancel(ctx, o.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("cancel received: %v", err)
	}
}

func TestOverReceiptLeavesEverythingUntouched(t *testing.T) {
	f := setup(t)
	o := f.placed(t, f.paper(5))
	_, err := f.receive(o, "", 6)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Code != domain.CodeOverReceipt {
		t.Fatalf("over receipt: %v", err)
	}
	if _, err := f.receive(o, "", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := f.receive(o, "", 1); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("receive on a received order: %v", err)
	}
	if n := f.q(t, `SELECT stock_current FROM items WHERE id = $1::uuid`, f.consID); n != 5 {
		t.Fatalf("stock = %d", n)
	}
}

func TestAssetLineCreatesDistinctItems(t *testing.T) {
	f := setup(t)
	o := f.placed(t, laptops(3))
	res, err := f.receive(o, "", 3)
	if err != nil {
		t.Fatal(err)
	}
	items := res.Receipt.Lines[0].Items
	if len(items) != 3 {
		t.Fatalf("items: %+v", items)
	}
	got, _ := f.svc.Get(context.Background(), o.ID)
	codes := map[string]bool{}
	for _, it := range got.Receipts[0].Lines[0].Items {
		codes[it.Code] = true
	}
	if len(codes) != 3 {
		t.Fatalf("codes not distinct: %v", codes)
	}
	if n := f.q(t, `SELECT count(*) FROM items WHERE description = 'Laptop' AND kind = 'asset' AND location_id = $1`, f.loc); n != 3 {
		t.Fatalf("laptops in catalog at the receipt location: %d", n)
	}
	if got.Lines[0].ItemID != items[0].ItemID || got.Lines[0].NewItem == nil {
		t.Fatalf("line should keep new_item and point at the first item: %+v", got.Lines[0])
	}
	if n := f.q(t, `SELECT count(*) FROM stock_movements`); n != 0 {
		t.Fatalf("assets recorded %d movements", n)
	}
}

func TestFailedReceiptRollsBackItemsAndStock(t *testing.T) {
	f := setup(t)
	o := f.placed(t, laptops(3), ink(10), f.paper(2))

	// The second asset creation fails: the first one must disappear too.
	f.cat.failOnNth.Store(2)
	if _, err := f.receive(o, "k", 3, 0, 0); err == nil {
		t.Fatal("want failure")
	}
	f.cat.failOnNth.Store(0)
	if n := f.q(t, `SELECT count(*) FROM items WHERE description = 'Laptop'`); n != 0 {
		t.Fatalf("orphan assets: %d", n)
	}

	// The stock port fails after a new consumable and an asset were created.
	f.stock.fail.Store(true)
	if _, err := f.receive(o, "k", 2, 5, 0); err == nil {
		t.Fatal("want failure")
	}
	f.stock.fail.Store(false)
	for _, q := range []string{
		`SELECT count(*) FROM items WHERE description IN ('Laptop', 'Tinta')`,
		`SELECT count(*) FROM stock_movements`,
		`SELECT count(*) FROM po_receipts`,
		`SELECT count(*) FROM po_receipt_items`,
		`SELECT coalesce(sum(received_qty), 0) FROM purchase_order_lines`,
	} {
		if n := f.q(t, q); n != 0 {
			t.Fatalf("%s = %d after rollback", q, n)
		}
	}
	got, _ := f.svc.Get(context.Background(), o.ID)
	if got.Status != domain.StatusOrdered || got.Lines[0].ItemID != "" || got.Lines[1].ItemID != "" {
		t.Fatalf("order changed by rolled back receipts: %+v", got)
	}

	// With the faults gone the same request succeeds, and with the same key.
	if res, err := f.receive(o, "k", 3, 5, 2); err != nil || res.Replayed {
		t.Fatalf("retry: %+v %v", res, err)
	}
}

func TestNewConsumableCreatedOnceAcrossPartialReceipts(t *testing.T) {
	f := setup(t)
	o := f.placed(t, ink(10))
	if _, err := f.receive(o, "", 6); err != nil {
		t.Fatal(err)
	}
	if _, err := f.receive(o, "", 4); err != nil {
		t.Fatal(err)
	}
	if n := f.q(t, `SELECT count(*) FROM items WHERE description = 'Tinta'`); n != 1 {
		t.Fatalf("Tinta items = %d", n)
	}
	if n := f.q(t, `SELECT stock_current FROM items WHERE description = 'Tinta'`); n != 10 {
		t.Fatalf("Tinta stock = %d", n)
	}
	got, _ := f.svc.Get(context.Background(), o.ID)
	if got.Lines[0].ItemID == "" || len(got.Receipts) != 2 ||
		got.Receipts[0].Lines[0].Items[0].ItemID != got.Receipts[1].Lines[0].Items[0].ItemID {
		t.Fatalf("receipts must reference the same item: %+v", got)
	}
	if n := f.q(t, `SELECT count(*) FROM stock_movements WHERE type = 'in'`); n != 2 {
		t.Fatalf("movements = %d", n)
	}
}

func TestConcurrentDoubleReceiveYieldsOneSuccess(t *testing.T) {
	f := setup(t)
	o := f.placed(t, laptops(2), f.paper(5))
	const workers = 4
	var wg sync.WaitGroup
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.receive(o, "", 2, 5)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range results {
		var ve *domain.ValidationError
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrInvalidState), errors.As(err, &ve) && ve.Code == domain.CodeOverReceipt:
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d concurrent receipts succeeded, want exactly 1 (%v)", ok, results)
	}
	if n := f.q(t, `SELECT count(*) FROM items WHERE description = 'Laptop'`); n != 2 {
		t.Fatalf("laptops = %d", n)
	}
	if n := f.q(t, `SELECT stock_current FROM items WHERE id = $1::uuid`, f.consID); n != 5 {
		t.Fatalf("stock = %d", n)
	}
}

func TestClientKeyReplayHasNoSideEffects(t *testing.T) {
	f := setup(t)
	o := f.placed(t, laptops(2), f.paper(5))
	first, err := f.receive(o, "key-1", 2, 3)
	if err != nil || first.Replayed {
		t.Fatalf("first: %+v %v", first, err)
	}
	items, movements := f.q(t, `SELECT count(*) FROM items`), f.q(t, `SELECT count(*) FROM stock_movements`)
	again, err := f.receive(o, "key-1", 2, 3)
	if err != nil || !again.Replayed || again.Receipt.ID != first.Receipt.ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if f.q(t, `SELECT count(*) FROM items`) != items || f.q(t, `SELECT count(*) FROM stock_movements`) != movements ||
		f.q(t, `SELECT count(*) FROM po_receipts`) != 1 {
		t.Fatal("replay had side effects")
	}
	if len(again.Receipt.Lines) != 2 || len(again.Receipt.Lines[0].Items) != 2 {
		t.Fatalf("replayed receipt lost its items: %+v", again.Receipt)
	}
	// Keys are scoped per order.
	other := f.placed(t, f.paper(1))
	if res, err := f.receive(other, "key-1", 1); err != nil || res.Replayed {
		t.Fatalf("same key on another order: %+v %v", res, err)
	}
}

func TestCancelRules(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	d, _ := f.svc.Create(ctx, "", domain.Input{SupplierID: f.supplier, Currency: domain.USD})
	if o, err := f.svc.Cancel(ctx, d.ID); err != nil || o.Status != domain.StatusCancelled || o.CancelledAt == nil {
		t.Fatalf("cancel draft: %+v %v", o, err)
	}
	p := f.placed(t, f.paper(5))
	if _, err := f.receive(p, "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Cancel(ctx, p.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("cancel after receipt: %v", err)
	}
	q := f.placed(t, f.paper(5))
	if o, err := f.svc.Cancel(ctx, q.ID); err != nil || o.Status != domain.StatusCancelled {
		t.Fatalf("cancel ordered: %+v %v", o, err)
	}
	if _, err := f.receive(q, "", 1); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("receive cancelled: %v", err)
	}
	if _, err := f.svc.Get(ctx, 9999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestSupplierRules(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in := domain.Input{SupplierID: f.inactive, Currency: domain.USD}
	if _, err := f.svc.Create(ctx, "", in); !isCode(err, domain.CodeSupplierInactive) {
		t.Fatalf("inactive supplier: %v", err)
	}
	in.SupplierID = 9999
	if _, err := f.svc.Create(ctx, "", in); !errors.Is(err, domain.ErrInvalidInput) || isCode(err, domain.CodeSupplierInactive) {
		t.Fatalf("unknown supplier: %v", err)
	}

	o, err := f.svc.Create(ctx, "", domain.Input{SupplierID: f.supplier, Currency: domain.USD, Lines: []domain.LineInput{f.paper(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE suppliers SET deleted_at = now() WHERE id = $1`, f.supplier); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Place(ctx, o.ID); !isCode(err, domain.CodeSupplierInactive) {
		t.Fatalf("place with a deleted supplier: %v", err)
	}
	got, err := f.svc.Get(ctx, o.ID)
	if err != nil || got.SupplierName != "Ferretería Sur" {
		t.Fatalf("a deleted supplier's orders stay readable: %+v %v", got, err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE suppliers SET deleted_at = NULL, active = FALSE WHERE id = $1`, f.supplier); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Place(ctx, o.ID); !isCode(err, domain.CodeSupplierInactive) {
		t.Fatalf("place with an inactive supplier: %v", err)
	}
}

func isCode(err error, code string) bool {
	var ve *domain.ValidationError
	return errors.As(err, &ve) && ve.Code == code
}

func TestItemReferencesAreChecked(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.Create(ctx, "", domain.Input{SupplierID: f.supplier, Currency: domain.USD, Lines: []domain.LineInput{
		{ItemID: f.assetID, Quantity: 1}, {ItemID: "not-a-uuid", Quantity: 1}}})
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || ve.Fields["lines[0].item_id"] != "existing_item_must_be_consumable" || ve.Fields["lines[1].item_id"] == "" {
		t.Fatalf("item checks: %v", err)
	}
	// An unknown location is rejected before anything is created.
	o := f.placed(t, laptops(1))
	_, err = f.svc.Receive(ctx, o.ID, f.userID, domain.ReceiveRequest{LocationID: 9999, Lines: []domain.ReceiveLine{{LineID: o.Lines[0].ID, Quantity: 1}}})
	if !errors.As(err, &ve) || ve.Fields["location_id"] == "" {
		t.Fatalf("unknown location: %v", err)
	}
	if n := f.q(t, `SELECT count(*) FROM items WHERE description = 'Laptop'`); n != 0 {
		t.Fatalf("items created for a rejected receipt: %d", n)
	}
}

func TestListFiltersPagingAndSearch(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var second int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO suppliers (name, active) VALUES ('Papelería Norte', TRUE) RETURNING id`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	a := f.placed(t, f.paper(1), laptops(1))
	b, _ := f.svc.Create(ctx, "", domain.Input{SupplierID: second, Currency: domain.VES})
	c := f.placed(t, f.paper(2))

	page := func(fl domain.Filter) ([]domain.Order, int) {
		t.Helper()
		os, total, _, err := f.svc.List(ctx, fl)
		if err != nil {
			t.Fatal(err)
		}
		return os, total
	}
	if os, total := page(domain.Filter{}); total != 3 || os[0].ID != c.ID || os[2].ID != a.ID || len(os[2].Lines) != 2 {
		t.Fatalf("all, newest first: %d %+v", total, os)
	}
	if os, total := page(domain.Filter{Limit: 1, Offset: 1}); total != 3 || len(os) != 1 || os[0].ID != b.ID {
		t.Fatalf("paging: %d %+v", total, os)
	}
	if _, total := page(domain.Filter{Status: domain.StatusDraft}); total != 1 {
		t.Fatalf("status: %d", total)
	}
	if os, total := page(domain.Filter{SupplierID: second}); total != 1 || os[0].SupplierName != "Papelería Norte" {
		t.Fatalf("supplier: %d", total)
	}
	if _, total := page(domain.Filter{Q: "sur"}); total != 2 {
		t.Fatalf("q by supplier name: %d", total)
	}
	if os, total := page(domain.Filter{Q: fmt.Sprintf("#%d", b.ID)}); total != 1 || os[0].ID != b.ID {
		t.Fatalf("q by id: %d", total)
	}
	if _, total := page(domain.Filter{Q: "%"}); total != 0 {
		t.Fatalf("wildcards must be literal: %d", total)
	}
}
