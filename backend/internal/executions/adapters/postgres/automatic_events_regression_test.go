package postgres_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	executionpg "github.com/edgar/inventario/backend/internal/executions/adapters/postgres"
	executionapp "github.com/edgar/inventario/backend/internal/executions/app"
	d "github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	projectpg "github.com/edgar/inventario/backend/internal/projects/adapters/postgres"
	pd "github.com/edgar/inventario/backend/internal/projects/domain"
)

func TestIndependentAutomaticEvents(t *testing.T) {
	pool, ctx := setupExecutions(t)
	var actor string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Actual Actor','auto-probe@test.local','x','worker') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	var customer int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Probe Client') RETURNING id`).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	pr := projectpg.New(pool)
	project := &pd.Project{Input: pd.Input{Name: "Probe", Characteristics: []string{}, Consumables: []pd.Material{}, Tools: []string{}, EstimatedDays: 1}}
	if err := pr.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	svc := executionapp.NewService(executionpg.New(pool), pr)
	e, err := svc.Create(ctx, d.CreateInput{ProjectID: project.ID, Units: 4, CustomerIDs: []int64{customer}, PrincipalUserID: actor})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Start(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	candidate := os.Getenv("AUTO_VERIFY_BASE") == ""
	hash := func() [32]byte {
		var value string
		err := pool.QueryRow(ctx, `SELECT jsonb_build_array(
   (SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM manufacturing_execution_events e),
   (SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM manufacturing_executions e),
   (SELECT jsonb_agg(to_jsonb(e) ORDER BY request_id) FROM manufacturing_production_batches e),
   (SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM finished_product_units e),
   (SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM manufacturing_deliveries e),
   (SELECT jsonb_agg(to_jsonb(e) ORDER BY unit_id) FROM manufacturing_delivery_units e))::text`).Scan(&value)
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256([]byte(value))
	}
	checkEvents := func(want int) {
		events, err := svc.ListEvents(ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !candidate {
			want = 0
		}
		if len(events) != want {
			t.Fatalf("events=%+v want %d", events, want)
		}
		for _, event := range events {
			if event.ActorID != actor || event.ActorName != "Actual Actor" || event.OccurredAt.IsZero() {
				t.Fatalf("actor/time=%+v", event)
			}
		}
	}
	// Joined transaction must contain event and units, neither may leak after rollback.
	before := hash()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	txctx := db.WithTx(ctx, tx)
	_, err = svc.Produce(txctx, e.ID, actor, d.ProduceInput{RequestID: "00000000-0000-4000-8000-000000009001", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	var inside int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM manufacturing_execution_events`).Scan(&inside); err != nil {
		t.Fatal(err)
	}
	if candidate && inside != 1 {
		t.Fatalf("inside production events=%d", inside)
	}
	checkEvents(0)
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if before != hash() {
		t.Fatal("production rollback changed persisted hash")
	}
	// Twelve concurrent exact replays must return the identical result and one event.
	p := d.ProduceInput{RequestID: "00000000-0000-4000-8000-000000009002", Quantity: 2}
	var wg sync.WaitGroup
	results := make(chan []d.FinishedUnit, 12)
	errs := make(chan error, 12)
	start := make(chan struct{})
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			u, err := svc.Produce(ctx, e.ID, actor, p)
			results <- u
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var units []d.FinishedUnit
	for u := range results {
		if units == nil {
			units = u
		}
		if !reflect.DeepEqual(units, u) {
			t.Fatal("production replay output differs")
		}
	}
	checkEvents(1)
	before = hash()
	_, err = svc.Produce(ctx, e.ID, actor, d.ProduceInput{RequestID: p.RequestID, Quantity: 1})
	if err != d.ErrProductionReplay {
		t.Fatalf("replay rejection=%v", err)
	}
	if before != hash() {
		t.Fatal("rejected production changed hash")
	}
	before = hash()
	_, err = svc.Produce(ctx, e.ID, actor, d.ProduceInput{RequestID: "00000000-0000-4000-8000-000000009003", Quantity: 3})
	if err != d.ErrProductionLimit {
		t.Fatalf("capacity rejection=%v", err)
	}
	if before != hash() {
		t.Fatal("rejected overproduction changed hash")
	}
	delivery := d.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000009004", CustomerID: customer, UnitIDs: []int64{units[0].ID}}
	before = hash()
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.DeliverFinishedUnits(db.WithTx(ctx, tx), e.ID, actor, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM manufacturing_execution_events`).Scan(&inside); err != nil {
		t.Fatal(err)
	}
	if candidate && inside != 2 {
		t.Fatalf("inside delivery events=%d", inside)
	}
	checkEvents(1)
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if before != hash() {
		t.Fatal("delivery rollback changed persisted hash")
	}
	receipts := make(chan d.DeliveryReceipt, 12)
	errs = make(chan error, 12)
	start = make(chan struct{})
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := svc.DeliverFinishedUnits(ctx, e.ID, actor, delivery)
			receipts <- r
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(receipts)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var receipt d.DeliveryReceipt
	for r := range receipts {
		if receipt.ID == 0 {
			receipt = r
		}
		if !reflect.DeepEqual(receipt, r) {
			t.Fatal("delivery replay output differs")
		}
	}
	checkEvents(2)
	before = hash()
	_, err = svc.DeliverFinishedUnits(ctx, e.ID, actor, d.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000009005", CustomerID: customer, UnitIDs: delivery.UnitIDs})
	if err != d.ErrUnitDelivered {
		t.Fatalf("duplicate rejection=%v", err)
	}
	if before != hash() {
		t.Fatal("rejected delivery changed hash")
	}
	before = hash()
	_, err = svc.DeliverFinishedUnits(ctx, e.ID, actor, d.DeliveryInput{RequestID: delivery.RequestID, CustomerID: customer, UnitIDs: []int64{units[1].ID}})
	if err != d.ErrDeliveryReplay {
		t.Fatalf("changed delivery=%v", err)
	}
	if before != hash() {
		t.Fatal("rejected delivery replay changed hash")
	}
	fmt.Printf("DIFFERENTIAL produced=%d codes=%s,%s receipt=%s customer=%s unit_codes=%v rejected=%v,%v,%v\n", len(units), units[0].Code, units[1].Code, receipt.Number, receipt.CustomerName, receipt.UnitCodes, d.ErrProductionReplay, d.ErrProductionLimit, d.ErrUnitDelivered)
	_ = context.Background()
}
