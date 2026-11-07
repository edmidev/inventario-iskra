package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	executionpg "github.com/edgar/inventario/backend/internal/executions/adapters/postgres"
	executionapp "github.com/edgar/inventario/backend/internal/executions/app"
	executiondomain "github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	projectpg "github.com/edgar/inventario/backend/internal/projects/adapters/postgres"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
	stockpg "github.com/edgar/inventario/backend/internal/stock/adapters/postgres"
	stockapp "github.com/edgar/inventario/backend/internal/stock/app"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/edgar/inventario/backend/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func setupExecutions(t *testing.T) (*pgxpool.Pool, context.Context) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE manufacturing_execution_material_operations, manufacturing_execution_materials, manufacturing_execution_workers, manufacturing_execution_customers, manufacturing_executions, project_templates, customers, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return pool, ctx
}

func TestExecutionMaterialsReserveFIFOAndRecordPhysicalMovements(t *testing.T) {
	pool, ctx := setupExecutions(t)
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Responsable','t5-owner@example.com','x','operator') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Cliente T5') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	codePrefix := fmt.Sprintf("T5%05X", time.Now().UnixNano()%0xFFFFF)
	base := catalogpg.New(pool)
	catalog := catalogapp.NewService(base.Categories(), base.Locations(), base.Items())
	category, err := catalog.CreateCategory(ctx, "T5 materials "+codePrefix, codePrefix)
	if err != nil {
		t.Fatal(err)
	}
	location, err := catalog.CreateLocation(ctx, "T5 storage "+codePrefix, catalogdomain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	unit := "kg"
	item, err := catalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{
		Description: "T5 material " + codePrefix, CategoryID: category.ID, Kind: catalogdomain.KindConsumable, Unit: &unit,
	}})
	if err != nil {
		t.Fatal(err)
	}
	stock := stockapp.NewService(stockpg.New(pool))
	if _, err := stock.Record(ctx, item.ID, userID, stockdomain.Request{Type: stockdomain.TypeIn, Qty: quantity.Quantity(4000), LocationID: &location.ID}); err != nil {
		t.Fatal(err)
	}
	templateRepo := projectpg.New(pool)
	project := &projectdomain.Project{Input: projectdomain.Input{
		Name: "T5 template " + codePrefix, Characteristics: []string{},
		Consumables: []projectdomain.Material{{ItemID: item.ID, Quantity: quantity.Quantity(1250)}},
		Tools:       []string{}, EstimatedDays: 1,
	}}
	if err := templateRepo.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	svc := executionapp.NewService(executionpg.New(pool), templateRepo, stock)
	create := func() executiondomain.Execution {
		t.Helper()
		created, createErr := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 2,
			CustomerIDs: []int64{customerID}, PrincipalUserID: userID})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return created
	}
	older, newer := create(), create()
	if older.Materials[0].RequiredQuantity != quantity.Quantity(2500) || older.Materials[0].ReservedQuantity != quantity.Quantity(2500) {
		t.Fatalf("older execution allocation = %+v", older.Materials)
	}
	if newer.Materials[0].RequiredQuantity != quantity.Quantity(2500) || newer.Materials[0].ReservedQuantity != quantity.Quantity(1500) || newer.Materials[0].DeficitQuantity != quantity.Quantity(1000) {
		t.Fatalf("newer FIFO allocation = %+v", newer.Materials)
	}
	if _, err := stock.Record(ctx, item.ID, userID, stockdomain.Request{Type: stockdomain.TypeOut, Qty: quantity.Quantity(1), LocationID: &location.ID}); !errors.Is(err, stockdomain.ErrInsufficientStock) {
		t.Fatalf("ordinary stock out consumed reserved material: %v", err)
	}
	if _, err := svc.DeliverMaterial(ctx, newer.ID, item.ID, userID, "00000000-0000-4000-8000-000000000002", quantity.Quantity(1750), location.ID, "over own reservation"); !errors.Is(err, stockdomain.ErrReservationUnavailable) {
		t.Fatalf("delivery consumed another execution's reservation: %v", err)
	}
	delivered, err := svc.DeliverMaterial(ctx, older.ID, item.ID, userID, "00000000-0000-4000-8000-000000000001", quantity.Quantity(1000), location.ID, "to workshop")
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Materials[0].DeliveredQuantity != quantity.Quantity(1000) || delivered.Materials[0].ReservedQuantity != quantity.Quantity(1500) {
		t.Fatalf("delivery state = %+v", delivered.Materials)
	}
	replayed, err := svc.DeliverMaterial(ctx, older.ID, item.ID, userID, "00000000-0000-4000-8000-000000000001", quantity.Quantity(1000), location.ID, "to workshop")
	if err != nil || replayed.Materials[0].DeliveredQuantity != quantity.Quantity(1000) {
		t.Fatalf("idempotent delivery replay = %+v err=%v", replayed.Materials, err)
	}
	if _, err := svc.DeliverMaterial(ctx, older.ID, item.ID, userID, "00000000-0000-4000-8000-000000000001", quantity.Quantity(500), location.ID, "changed replay"); !errors.Is(err, stockdomain.ErrIdempotencyConflict) {
		t.Fatalf("reused request id with changed payload: %v", err)
	}
	returned, err := svc.ReturnMaterial(ctx, older.ID, item.ID, userID, "00000000-0000-4000-8000-000000000003", quantity.Quantity(250), location.ID, "unused")
	if err != nil {
		t.Fatal(err)
	}
	if returned.Materials[0].ReturnedQuantity != quantity.Quantity(250) || returned.Materials[0].DeficitQuantity != 0 {
		t.Fatalf("return state = %+v", returned.Materials)
	}
	returnErrors := make(chan error, 2)
	var returns sync.WaitGroup
	for range 2 {
		returns.Add(1)
		go func() {
			defer returns.Done()
			_, returnErr := svc.ReturnMaterial(ctx, older.ID, item.ID, userID, "00000000-0000-4000-8000-000000000005", quantity.Quantity(100), location.ID, "replayed return")
			returnErrors <- returnErr
		}()
	}
	returns.Wait()
	close(returnErrors)
	for returnErr := range returnErrors {
		if returnErr != nil {
			t.Fatalf("concurrent idempotent return: %v", returnErr)
		}
	}
	var replayCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM manufacturing_execution_material_operations WHERE request_id='00000000-0000-4000-8000-000000000005'`).Scan(&replayCount); err != nil || replayCount != 1 {
		t.Fatalf("concurrent replay stored %d operations, error=%v", replayCount, err)
	}
	if _, err := stock.Record(ctx, item.ID, userID, stockdomain.Request{Type: stockdomain.TypeAdjust, Qty: quantity.Quantity(2000), LocationID: &location.ID, Note: "physical recount"}); err != nil {
		t.Fatal(err)
	}
	older, err = svc.Get(ctx, older.ID)
	if err != nil {
		t.Fatal(err)
	}
	newer, err = svc.Get(ctx, newer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if older.Materials[0].ReservedQuantity != quantity.Quantity(1500) || newer.Materials[0].ReservedQuantity != quantity.Quantity(500) || newer.Materials[0].DeficitQuantity != quantity.Quantity(2000) {
		t.Fatalf("physical count did not rebalance FIFO: older=%+v newer=%+v", older.Materials, newer.Materials)
	}
	if _, err := stock.Record(ctx, item.ID, userID, stockdomain.Request{Type: stockdomain.TypeIn, Qty: quantity.Quantity(1000), LocationID: &location.ID}); err != nil {
		t.Fatal(err)
	}
	newer, err = svc.Get(ctx, newer.ID)
	if err != nil || newer.Materials[0].ReservedQuantity != quantity.Quantity(1500) || newer.Materials[0].DeficitQuantity != quantity.Quantity(1000) {
		t.Fatalf("inbound stock did not fill the oldest remaining deficit: newer=%+v err=%v", newer.Materials, err)
	}
	if _, err := svc.ReturnMaterial(ctx, older.ID, item.ID, userID, "00000000-0000-4000-8000-000000000004", quantity.Quantity(751), location.ID, "too much returned"); !errors.Is(err, stockdomain.ErrReservationUnavailable) {
		t.Fatalf("return exceeded delivered balance: %v", err)
	}

	createErrors := make(chan error, 2)
	var creates sync.WaitGroup
	for range 2 {
		creates.Add(1)
		go func() {
			defer creates.Done()
			_, createErr := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 2,
				CustomerIDs: []int64{customerID}, PrincipalUserID: userID})
			createErrors <- createErr
		}()
	}
	creates.Wait()
	close(createErrors)
	for createErr := range createErrors {
		if createErr != nil {
			t.Fatalf("concurrent execution registration: %v", createErr)
		}
	}
	var stockCurrent, reserved quantity.Quantity
	if err := pool.QueryRow(ctx, `SELECT i.stock_current,COALESCE(sum(m.quantity_reserved),0)
FROM items i LEFT JOIN manufacturing_execution_materials m ON m.item_id=i.id WHERE i.id=$1::uuid GROUP BY i.id`, item.ID).Scan(&stockCurrent, &reserved); err != nil {
		t.Fatal(err)
	}
	if reserved > stockCurrent {
		t.Fatalf("concurrent registrations reserved %s against physical stock %s", reserved, stockCurrent)
	}
	beforeReserved := reserved
	sqlDB, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 16); err == nil {
		t.Fatal("reservation migration rolled back while execution history exists")
	}
	var afterReserved quantity.Quantity
	var tableExists bool
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(quantity_reserved),0) FROM manufacturing_execution_materials WHERE item_id=$1::uuid`, item.ID).Scan(&afterReserved); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('manufacturing_execution_materials') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatal(err)
	}
	if !tableExists || afterReserved != beforeReserved {
		t.Fatalf("rejected rollback mutated reservations: table=%v before=%s after=%s", tableExists, beforeReserved, afterReserved)
	}
}

func TestCreateStoresIndependentSnapshotAndAllowsPartialStart(t *testing.T) {
	pool, ctx := setupExecutions(t)
	var principal, worker string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Ana','ana-execution@example.com','x','operator') RETURNING id::text`).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Luis','luis-execution@example.com','x','worker') RETURNING id::text`).Scan(&worker); err != nil {
		t.Fatal(err)
	}
	var customer1, customer2 int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Cliente Norte') RETURNING id`).Scan(&customer1); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Cliente Sur') RETURNING id`).Scan(&customer2); err != nil {
		t.Fatal(err)
	}
	codePrefix := fmt.Sprintf("T4%05X", time.Now().UnixNano()%0xFFFFF)
	var categoryID int64
	if err := pool.QueryRow(ctx, `INSERT INTO categories(name,code_prefix) VALUES($1,$2) RETURNING id`, "T4 materials "+codePrefix, codePrefix).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	var materialID string
	if err := pool.QueryRow(ctx, `INSERT INTO items(code,description,category_id,kind,unit,stock_current,stock_min)
VALUES($1,'T4 material',$2,'consumable','u',0,0) RETURNING id::text`, "ISK-"+codePrefix+"-001", categoryID).Scan(&materialID); err != nil {
		t.Fatal(err)
	}
	templates := projectpg.New(pool)
	project := projectdomain.Input{
		Name: "Marmita", Description: "Acero inoxidable", Characteristics: []string{"Aislamiento"},
		Consumables: []projectdomain.Material{{ItemID: materialID, Quantity: quantity.Quantity(1250)}},
		Tools:       []string{"asset-9"}, EstimatedDays: 12,
	}
	createdTemplate := &projectdomain.Project{Input: project}
	if err := templates.Create(ctx, createdTemplate); err != nil {
		t.Fatal(err)
	}
	repo := executionpg.New(pool)
	svc := executionapp.NewService(repo, templates)
	execution, err := svc.Create(ctx, executiondomain.CreateInput{
		ProjectID: createdTemplate.ID, Units: 3, CustomerIDs: []int64{customer1, customer2},
		PrincipalUserID: principal, WorkerIDs: []string{worker},
	})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != executiondomain.StatusRegistered || execution.EstimatedDays != 12 || execution.Units != 3 {
		t.Fatalf("created execution = %#v", execution)
	}
	project.Name = "Edited template"
	project.Consumables[0].Quantity = quantity.Quantity(9000)
	project.Characteristics[0] = "Edited"
	if err := templates.Update(ctx, &projectdomain.Project{ID: createdTemplate.ID, Input: project}); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Marmita" || loaded.Consumables[0].QuantityPerUnit != quantity.Quantity(1250) || loaded.Characteristics[0] != "Aislamiento" {
		t.Fatalf("execution snapshot changed with template: %#v", loaded)
	}
	if len(loaded.CustomerIDs) != 2 || loaded.PrincipalUserID != principal || len(loaded.WorkerIDs) != 1 {
		t.Fatalf("assignments were not persisted: %#v", loaded)
	}
	listed, total, _, err := svc.List(ctx, executiondomain.Filter{Q: "Marmita"})
	if err != nil || total != 1 || len(listed) != 1 || listed[0].ID != loaded.ID {
		t.Fatalf("list query = %#v, total %d, error %v", listed, total, err)
	}
	startResults := make(chan error, 2)
	var starts sync.WaitGroup
	starts.Add(2)
	for range 2 {
		go func() {
			defer starts.Done()
			_, startErr := svc.Start(ctx, loaded.ID)
			startResults <- startErr
		}()
	}
	starts.Wait()
	close(startResults)
	successes, invalidTransitions := 0, 0
	for startErr := range startResults {
		switch {
		case startErr == nil:
			successes++
		case errors.Is(startErr, executiondomain.ErrInvalidState):
			invalidTransitions++
		default:
			t.Fatalf("partial start without inventory reservation failed: %v", startErr)
		}
	}
	if successes != 1 || invalidTransitions != 1 {
		t.Fatalf("concurrent starts: successes=%d invalid=%d, want one each", successes, invalidTransitions)
	}
	started, err := svc.Get(ctx, loaded.ID)
	if err != nil || started.Status != executiondomain.StatusInProgress {
		t.Fatalf("started execution=%#v, error=%v", started, err)
	}
}

func TestCreateRejectsInactiveAssignedUserAndCustomer(t *testing.T) {
	pool, ctx := setupExecutions(t)
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role,active) VALUES('Inactivo','inactive-execution@example.com','x','worker',false) RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name,active) VALUES('Cliente Inactivo',false) RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	templateRepo := projectpg.New(pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Calentador", Characteristics: []string{}, Consumables: []projectdomain.Material{}, Tools: []string{}, EstimatedDays: 1}}
	if err := templateRepo.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	svc := executionapp.NewService(executionpg.New(pool), templateRepo)
	_, err := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 1, CustomerIDs: []int64{customerID}, PrincipalUserID: userID})
	if !errors.Is(err, executiondomain.ErrInactiveAssignment) {
		t.Fatalf("Create() error = %v, want inactive assignment", err)
	}
	var executions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM manufacturing_executions`).Scan(&executions); err != nil {
		t.Fatal(err)
	}
	if executions != 0 {
		t.Fatalf("rejected creation persisted %d executions", executions)
	}
}
