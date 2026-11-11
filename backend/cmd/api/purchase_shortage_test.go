package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	executionpg "github.com/edgar/inventario/backend/internal/executions/adapters/postgres"
	executionapp "github.com/edgar/inventario/backend/internal/executions/app"
	executiondomain "github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	projectpg "github.com/edgar/inventario/backend/internal/projects/adapters/postgres"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
	purchasingdomain "github.com/edgar/inventario/backend/internal/purchasing/domain"
	stockpg "github.com/edgar/inventario/backend/internal/stock/adapters/postgres"
	stockapp "github.com/edgar/inventario/backend/internal/stock/app"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
)

func TestLinkedPurchaseReceiptReservesForItsExecution(t *testing.T) {
	e, ctx := setupReal(t)
	item, err := e.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Steel sheet", CategoryID: e.conCat, Kind: catalogdomain.KindConsumable, Unit: ptr("sheet")}})
	if err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Receipt customer') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	projects := projectpg.New(e.pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Receipt vessel", Characteristics: []string{}, Tools: []string{}, Consumables: []projectdomain.Material{{ItemID: item.ID, Quantity: 1500}}, EstimatedDays: 2}}
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	executions := executionapp.NewService(executionpg.New(e.pool), projects)
	create := func() executiondomain.Execution {
		out, err := executions.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 1, CustomerIDs: []int64{customerID}, PrincipalUserID: e.userID})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	older, target := create(), create()
	if older.ShortageRequestID == 0 || target.ShortageRequestID == 0 {
		t.Fatalf("expected two shortage requests: older=%d target=%d", older.ShortageRequestID, target.ShortageRequestID)
	}
	purchases := newPurchasingService(e.pool)
	order, err := purchases.Create(ctx, e.userID, purchasingdomain.Input{SupplierID: e.supplier, Currency: purchasingdomain.USD,
		Lines: []purchasingdomain.LineInput{{ItemID: item.ID, Quantity: quantity.Quantity(1500), UnitCostMinor: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	if order, err = purchases.Place(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := executions.LinkPurchaseOrderLine(ctx, target.ID, order.Lines[0].ID, quantity.Quantity(1500)); err != nil {
		t.Fatal(err)
	}
	first := purchasingdomain.ReceiveRequest{LocationID: e.loc, ClientKey: "receipt-for-target-1", Lines: []purchasingdomain.ReceiveLine{{LineID: order.Lines[0].ID, Quantity: quantity.Quantity(500)}}}
	if _, err := purchases.Receive(ctx, order.ID, e.userID, first); err != nil {
		t.Fatal(err)
	}
	partialTarget, err := executions.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if partialTarget.Materials[0].ReservedQuantity != quantity.Quantity(500) || partialTarget.Materials[0].PurchasedPendingQuantity != quantity.Quantity(1000) {
		t.Fatalf("partial receipt did not reserve 0.5 for target: %+v", partialTarget.Materials[0])
	}
	req := purchasingdomain.ReceiveRequest{LocationID: e.loc, ClientKey: "receipt-for-target-2", Lines: []purchasingdomain.ReceiveLine{{LineID: order.Lines[0].ID, Quantity: quantity.Quantity(1000)}}}
	if _, err := purchases.Receive(ctx, order.ID, e.userID, req); err != nil {
		t.Fatal(err)
	}
	gotOlder, err := executions.Get(ctx, older.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotTarget, err := executions.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOlder.Materials[0].ReservedQuantity != 0 || gotTarget.Materials[0].ReservedQuantity != quantity.Quantity(1500) {
		t.Fatalf("linked receipt allocation older=%+v target=%+v", gotOlder.Materials[0], gotTarget.Materials[0])
	}
	if replay, err := purchases.Receive(ctx, order.ID, e.userID, req); err != nil || !replay.Replayed {
		t.Fatalf("receipt replay: result=%+v err=%v", replay, err)
	}
	stock := stockapp.NewService(stockpg.New(e.pool))
	if _, err := stock.Record(ctx, item.ID, e.userID, stockdomain.Request{Type: stockdomain.TypeIn, Qty: 500, LocationID: &e.loc}); err != nil {
		t.Fatal(err)
	}
	gotOlder, err = executions.Get(ctx, older.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotTarget, err = executions.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOlder.Materials[0].ReservedQuantity != 500 || gotTarget.Materials[0].ReservedQuantity != 1500 {
		t.Fatalf("later free inbound lost earmark: older=%+v target=%+v", gotOlder.Materials[0], gotTarget.Materials[0])
	}
	if _, err := stock.Record(ctx, item.ID, e.userID, stockdomain.Request{Type: stockdomain.TypeAdjust, Qty: 1000, LocationID: &e.loc, Note: "physical count"}); err != nil {
		t.Fatal(err)
	}
	gotOlder, err = executions.Get(ctx, older.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotTarget, err = executions.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOlder.Materials[0].ReservedQuantity != 0 || gotTarget.Materials[0].ReservedQuantity != 1000 {
		t.Fatalf("physical count did not honor linked receipt priority: older=%+v target=%+v", gotOlder.Materials[0], gotTarget.Materials[0])
	}

	// Linking and receiving the same ordered line may race. Either the link
	// wins or the late link is rejected, but neither path may deadlock.
	raceOrder, err := purchases.Create(ctx, e.userID, purchasingdomain.Input{SupplierID: e.supplier, Currency: purchasingdomain.USD,
		Lines: []purchasingdomain.LineInput{{ItemID: item.ID, Quantity: 500, UnitCostMinor: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	if raceOrder, err = purchases.Place(ctx, raceOrder.ID); err != nil {
		t.Fatal(err)
	}
	raceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var linkErr, receiptErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, linkErr = executions.LinkPurchaseOrderLine(raceCtx, target.ID, raceOrder.Lines[0].ID, 500)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, receiptErr = purchases.Receive(raceCtx, raceOrder.ID, e.userID, purchasingdomain.ReceiveRequest{
			LocationID: e.loc, ClientKey: "racing-receipt", Lines: []purchasingdomain.ReceiveLine{{LineID: raceOrder.Lines[0].ID, Quantity: 500}}})
	}()
	close(start)
	wg.Wait()
	if receiptErr != nil || (linkErr != nil && !errors.Is(linkErr, executiondomain.ErrInvalidInput)) {
		t.Fatalf("link/receipt race: link=%v receipt=%v", linkErr, receiptErr)
	}
}

func TestReceiptAfterExecutionCancellationBecomesGeneralStock(t *testing.T) {
	e, ctx := setupReal(t)
	item, err := e.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Cancelled receipt steel", CategoryID: e.conCat, Kind: catalogdomain.KindConsumable, Unit: ptr("sheet")}})
	if err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Cancelled receipt customer') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	projects := projectpg.New(e.pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Cancelled vessel", Characteristics: []string{}, Tools: []string{}, Consumables: []projectdomain.Material{{ItemID: item.ID, Quantity: 1500}}, EstimatedDays: 2}}
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	executions := executionapp.NewService(executionpg.New(e.pool), projects)
	target, err := executions.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 1, CustomerIDs: []int64{customerID}, PrincipalUserID: e.userID})
	if err != nil {
		t.Fatal(err)
	}
	purchases := newPurchasingService(e.pool)
	order, err := purchases.Create(ctx, e.userID, purchasingdomain.Input{SupplierID: e.supplier, Currency: purchasingdomain.USD,
		Lines: []purchasingdomain.LineInput{{ItemID: item.ID, Quantity: 1500, UnitCostMinor: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	if order, err = purchases.Place(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := executions.LinkPurchaseOrderLine(ctx, target.ID, order.Lines[0].ID, 1500); err != nil {
		t.Fatal(err)
	}
	if _, err := executionpg.New(e.pool).Transition(ctx, target.ID, executiondomain.StatusRegistered, executiondomain.StatusCancelled); err != nil {
		t.Fatal(err)
	}
	result, err := purchases.Receive(ctx, order.ID, e.userID, purchasingdomain.ReceiveRequest{
		LocationID: e.loc, ClientKey: "receipt-after-cancel", Lines: []purchasingdomain.ReceiveLine{{LineID: order.Lines[0].ID, Quantity: 1500}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Order.Status != purchasingdomain.StatusReceived {
		t.Fatalf("execution cancellation changed supplier order status: %s", result.Order.Status)
	}
	got, err := executions.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Materials[0].ReservedQuantity != 0 {
		t.Fatalf("cancelled execution retained inbound reservation: %+v", got.Materials[0])
	}
	var current quantity.Quantity
	if err := e.pool.QueryRow(ctx, `SELECT stock_current FROM items WHERE id=$1::uuid`, item.ID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != 1500 {
		t.Fatalf("late receipt stock=%v, want 1.5 general inventory", current)
	}
}

func TestConsumedEarmarkDoesNotCaptureFreeInbound(t *testing.T) {
	e, ctx := setupReal(t)
	item, err := e.catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "Steel sheet", CategoryID: e.conCat, Kind: catalogdomain.KindConsumable, Unit: ptr("sheet")}})
	if err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Receipt customer') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	projects := projectpg.New(e.pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Receipt vessel", Characteristics: []string{}, Tools: []string{}, Consumables: []projectdomain.Material{{ItemID: item.ID, Quantity: 3000}}, EstimatedDays: 2}}
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	executions := executionapp.NewService(executionpg.New(e.pool), projects)
	create := func() executiondomain.Execution {
		out, err := executions.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 1, CustomerIDs: []int64{customerID}, PrincipalUserID: e.userID})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	older, target := create(), create()
	if older.ShortageRequestID == 0 || target.ShortageRequestID == 0 {
		t.Fatalf("expected two shortage requests: older=%d target=%d", older.ShortageRequestID, target.ShortageRequestID)
	}
	purchases := newPurchasingService(e.pool)
	order, err := purchases.Create(ctx, e.userID, purchasingdomain.Input{SupplierID: e.supplier, Currency: purchasingdomain.USD,
		Lines: []purchasingdomain.LineInput{{ItemID: item.ID, Quantity: quantity.Quantity(1500), UnitCostMinor: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	if order, err = purchases.Place(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := executions.LinkPurchaseOrderLine(ctx, target.ID, order.Lines[0].ID, quantity.Quantity(1500)); err != nil {
		t.Fatal(err)
	}

	req := purchasingdomain.ReceiveRequest{LocationID: e.loc, ClientKey: "verifier-consumed", Lines: []purchasingdomain.ReceiveLine{{LineID: order.Lines[0].ID, Quantity: 1500}}}
	if _, err := purchases.Receive(ctx, order.ID, e.userID, req); err != nil {
		t.Fatal(err)
	}
	stock := stockapp.NewService(stockpg.New(e.pool))
	deliveries := executionapp.NewService(executionpg.New(e.pool), projects, stock)
	if _, err := deliveries.DeliverMaterial(ctx, target.ID, item.ID, e.userID, "f81d4fae-7dec-41d0-a765-00a0c91e6bf6", 1500, e.loc, "consume earmarked delivery"); err != nil {
		t.Fatal(err)
	}
	if _, err := stock.Record(ctx, item.ID, e.userID, stockdomain.Request{Type: stockdomain.TypeIn, Qty: 1500, LocationID: &e.loc}); err != nil {
		t.Fatal(err)
	}
	gotOlder, err := executions.Get(ctx, older.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotTarget, err := executions.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOlder.Materials[0].ReservedQuantity != 1500 || gotTarget.Materials[0].ReservedQuantity != 0 {
		t.Fatalf("consumed earmark hijacked free inbound: older=%s target=%s; expected FIFO older=1.5 target=0", gotOlder.Materials[0].ReservedQuantity, gotTarget.Materials[0].ReservedQuantity)
	}
}
