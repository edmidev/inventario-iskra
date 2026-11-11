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
	if _, err := pool.Exec(ctx, `TRUNCATE manufacturing_execution_tool_additions, manufacturing_execution_supplemental_tools, manufacturing_execution_material_operations, manufacturing_execution_material_additions, manufacturing_execution_materials, manufacturing_execution_workers, manufacturing_execution_customers, manufacturing_executions, project_templates, customers, users RESTART IDENTITY CASCADE`); err != nil {
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

func TestRegistrationAddsSupplementalMaterialAsExecutionTotal(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(5000), quantity.Quantity(1250))
	created, err := f.service.Create(f.ctx, executiondomain.CreateInput{
		ProjectID: f.projectID, Units: 3, CustomerIDs: []int64{f.customerID}, PrincipalUserID: f.userID,
		AdditionalMaterials: []executiondomain.AdditionalMaterial{{ItemID: f.itemID, Quantity: quantity.Quantity(750)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Materials) != 1 || created.Materials[0].RequiredQuantity != quantity.Quantity(4500) || created.Materials[0].ReservedQuantity != quantity.Quantity(4500) {
		t.Fatalf("template demand should scale while supplemental quantity remains a total: %+v", created.Materials)
	}
	template, err := f.templates.Get(f.ctx, f.projectID)
	if err != nil || template.Consumables[0].Quantity != quantity.Quantity(1250) {
		t.Fatalf("execution addition mutated project template: template=%+v err=%v", template, err)
	}
}

func TestRegistrationAddsSupplementalToolsWithoutChangingTemplateOrStock(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(5000), quantity.Quantity(1250))
	created, err := f.service.Create(f.ctx, executiondomain.CreateInput{
		ProjectID: f.projectID, Units: 3, CustomerIDs: []int64{f.customerID}, PrincipalUserID: f.userID,
		AdditionalTools: []string{f.additionalToolID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tools) != 2 || created.Tools[0] != f.templateToolID || created.Tools[1] != f.additionalToolID {
		t.Fatalf("execution tools = %#v; want template and supplemental assets", created.Tools)
	}
	template, err := f.templates.Get(f.ctx, f.projectID)
	if err != nil || len(template.Tools) != 1 || template.Tools[0] != f.templateToolID {
		t.Fatalf("supplemental tool mutated project template: tools=%#v err=%v", template.Tools, err)
	}
	var materialRows, supplementalRows, stockMovements int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_materials WHERE execution_id=$1 AND item_id=$2::uuid`, created.ID, f.additionalToolID).Scan(&materialRows); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_supplemental_tools WHERE execution_id=$1 AND item_id=$2::uuid`, created.ID, f.additionalToolID).Scan(&supplementalRows); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM stock_movements WHERE item_id=$1::uuid`, f.additionalToolID).Scan(&stockMovements); err != nil {
		t.Fatal(err)
	}
	if materialRows != 0 || supplementalRows != 1 || stockMovements != 0 {
		t.Fatalf("informational tool affected materials or stock: materials=%d supplemental=%d movements=%d", materialRows, supplementalRows, stockMovements)
	}
}

func TestRegistrationRejectsConsumableAsSupplementalTool(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(5000), quantity.Quantity(1250))
	var beforeExecutions, beforeTools int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_executions`).Scan(&beforeExecutions); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_supplemental_tools`).Scan(&beforeTools); err != nil {
		t.Fatal(err)
	}

	_, err := f.service.Create(f.ctx, executiondomain.CreateInput{
		ProjectID: f.projectID, Units: 1, CustomerIDs: []int64{f.customerID}, PrincipalUserID: f.userID,
		AdditionalTools: []string{f.itemID},
	})
	if !errors.Is(err, executiondomain.ErrToolUnavailable) {
		t.Fatalf("consumable accepted as supplemental tool: %v", err)
	}
	var afterExecutions, afterTools int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_executions`).Scan(&afterExecutions); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_supplemental_tools`).Scan(&afterTools); err != nil {
		t.Fatal(err)
	}
	if afterExecutions != beforeExecutions || afterTools != beforeTools {
		t.Fatalf("rejected registration mutated state: executions %d -> %d, supplemental tools %d -> %d", beforeExecutions, afterExecutions, beforeTools, afterTools)
	}
}

func TestActiveExecutionCanAddSupplementalToolIdempotently(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(5000), quantity.Quantity(1250))
	execution := f.create(1)
	if _, err := f.service.Start(f.ctx, execution.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Pause(f.ctx, execution.ID); err != nil {
		t.Fatal(err)
	}
	input := executiondomain.AddToolInput{RequestID: "00000000-0000-4000-8000-000000000031", ItemID: f.additionalToolID}
	type addResult struct {
		execution executiondomain.Execution
		err       error
	}
	results := make(chan addResult, 2)
	var additions sync.WaitGroup
	for range 2 {
		additions.Add(1)
		go func() {
			defer additions.Done()
			added, addErr := f.service.AddToolRequirement(f.ctx, execution.ID, f.userID, input)
			results <- addResult{added, addErr}
		}()
	}
	additions.Wait()
	close(results)
	for result := range results {
		if result.err != nil || len(result.execution.Tools) != 2 || result.execution.Tools[1] != f.additionalToolID {
			t.Fatalf("concurrent tool addition replay = %#v, err=%v", result.execution.Tools, result.err)
		}
	}
	if _, err := f.service.AddToolRequirement(f.ctx, execution.ID, f.userID, executiondomain.AddToolInput{RequestID: input.RequestID, ItemID: f.templateToolID}); !errors.Is(err, stockdomain.ErrIdempotencyConflict) {
		t.Fatalf("changed request payload must conflict: %v", err)
	}
	other := f.create(1)
	if _, err := f.service.AddToolRequirement(f.ctx, other.ID, f.userID, executiondomain.AddToolInput{RequestID: "00000000-0000-4000-8000-000000000032", ItemID: f.additionalToolID}); err != nil {
		t.Fatalf("same tool should be shared across executions: %v", err)
	}
	var operationCount int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_tool_additions WHERE request_id=$1::uuid`, input.RequestID).Scan(&operationCount); err != nil || operationCount != 1 {
		t.Fatalf("stored idempotency operations = %d, want 1; err=%v", operationCount, err)
	}
	sqlDB, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(f.ctx, 18); err == nil {
		t.Fatal("migration rollback removed supplemental tool history")
	}
	var retained int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_tool_additions WHERE request_id=$1::uuid`, input.RequestID).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("rejected rollback changed tool addition history: count=%d err=%v", retained, err)
	}
}

func TestToolAdditionsRejectConsumablesAndClosedExecutions(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(5000), quantity.Quantity(1250))
	execution := f.create(1)
	if _, err := f.service.AddToolRequirement(f.ctx, execution.ID, f.userID, executiondomain.AddToolInput{RequestID: "00000000-0000-4000-8000-000000000041", ItemID: f.itemID}); !errors.Is(err, executiondomain.ErrToolUnavailable) {
		t.Fatalf("consumable accepted as a tool: %v", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE manufacturing_executions SET status='completed' WHERE id=$1`, execution.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AddToolRequirement(f.ctx, execution.ID, f.userID, executiondomain.AddToolInput{RequestID: "00000000-0000-4000-8000-000000000042", ItemID: f.additionalToolID}); !errors.Is(err, executiondomain.ErrToolAdditionClosed) {
		t.Fatalf("tool accepted after execution closed: %v", err)
	}
	current, err := f.service.Get(f.ctx, execution.ID)
	if err != nil || len(current.Tools) != 1 || current.Tools[0] != f.templateToolID {
		t.Fatalf("rejected tool additions changed execution: tools=%#v err=%v", current.Tools, err)
	}
	var additions, supplemental int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_tool_additions WHERE execution_id=$1`, execution.ID).Scan(&additions); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_supplemental_tools WHERE execution_id=$1`, execution.ID).Scan(&supplemental); err != nil {
		t.Fatal(err)
	}
	if additions != 0 || supplemental != 0 {
		t.Fatalf("rejected additions mutated tool history: operations=%d rows=%d", additions, supplemental)
	}
}

func TestSupplementalToolsMigrationRollsBackWhenHistoryIsEmpty(t *testing.T) {
	pool, ctx := setupExecutions(t)
	sqlDB, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 18); err != nil {
		t.Fatalf("empty supplemental-tools migration rollback: %v", err)
	}
	var tableExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('manufacturing_execution_supplemental_tools') IS NOT NULL`).Scan(&tableExists); err != nil || tableExists {
		t.Fatalf("empty rollback left tool table behind: exists=%v err=%v", tableExists, err)
	}
	if _, err := provider.UpTo(ctx, 19); err != nil {
		t.Fatalf("supplemental-tools migration re-apply: %v", err)
	}
}

func TestActiveExecutionCanAddMaterialAndReplayIdempotently(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(4000), quantity.Quantity(1000))
	older := f.create(3)
	newer := f.create(3)
	if older.Materials[0].ReservedQuantity != quantity.Quantity(3000) || newer.Materials[0].ReservedQuantity != quantity.Quantity(1000) {
		t.Fatalf("fixture did not allocate stock FIFO: older=%+v newer=%+v", older.Materials, newer.Materials)
	}
	if _, err := f.service.Start(f.ctx, newer.ID); err != nil {
		t.Fatal(err)
	}
	requestID := "00000000-0000-4000-8000-000000000021"
	added, err := f.service.AddMaterialRequirement(f.ctx, newer.ID, f.userID, executiondomain.AddMaterialInput{RequestID: requestID, ItemID: f.itemID, Quantity: quantity.Quantity(1000)})
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Materials) != 1 || added.Materials[0].RequiredQuantity != quantity.Quantity(4000) || added.Materials[0].ReservedQuantity != quantity.Quantity(1000) || added.Materials[0].DeficitQuantity != quantity.Quantity(3000) {
		t.Fatalf("active addition must increase total demand and preserve FIFO: %+v", added.Materials)
	}
	var duplicateErrors = make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, addErr := f.service.AddMaterialRequirement(f.ctx, newer.ID, f.userID, executiondomain.AddMaterialInput{RequestID: "00000000-0000-4000-8000-000000000022", ItemID: f.itemID, Quantity: quantity.Quantity(500)})
			duplicateErrors <- addErr
		}()
	}
	wg.Wait()
	close(duplicateErrors)
	for addErr := range duplicateErrors {
		if addErr != nil {
			t.Fatalf("concurrent identical additions: %v", addErr)
		}
	}
	newer, err = f.service.Get(f.ctx, newer.ID)
	if err != nil || newer.Materials[0].RequiredQuantity != quantity.Quantity(4500) {
		t.Fatalf("duplicate retry changed requirement more than once: %+v err=%v", newer.Materials, err)
	}
	if _, err := f.service.AddMaterialRequirement(f.ctx, newer.ID, f.userID, executiondomain.AddMaterialInput{RequestID: "00000000-0000-4000-8000-000000000022", ItemID: f.itemID, Quantity: quantity.Quantity(750)}); !errors.Is(err, stockdomain.ErrIdempotencyConflict) {
		t.Fatalf("changed request payload must conflict: %v", err)
	}
	var operationCount int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_material_additions WHERE request_id=$1::uuid`, "00000000-0000-4000-8000-000000000022").Scan(&operationCount); err != nil || operationCount != 1 {
		t.Fatalf("stored addition count = %d, want 1; err=%v", operationCount, err)
	}
	sqlDB, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(f.ctx, 17); err == nil {
		t.Fatal("migration rollback removed supplemental material idempotency history")
	}
	var retained int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_material_additions WHERE request_id=$1::uuid`, "00000000-0000-4000-8000-000000000022").Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("rejected rollback changed addition history: count=%d err=%v", retained, err)
	}
}

type supplementalMaterialFixture struct {
	pool             *pgxpool.Pool
	ctx              context.Context
	userID           string
	customerID       int64
	locationID       int64
	itemID           string
	templateToolID   string
	additionalToolID string
	projectID        int64
	templates        *projectpg.Repo
	service          *executionapp.Service
}

func setupSupplementalMaterialFixture(t *testing.T, stockQty, perUnit quantity.Quantity) supplementalMaterialFixture {
	t.Helper()
	pool, ctx := setupExecutions(t)
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Supplemental','supplemental-' || gen_random_uuid()::text || '@example.com','x','operator') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Supplemental customer ' || gen_random_uuid()::text) RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	code := fmt.Sprintf("S%07X", time.Now().UnixNano()%0x10000000)
	catalog := catalogpg.New(pool)
	category, err := catalogapp.NewService(catalog.Categories(), catalog.Locations(), catalog.Items()).CreateCategory(ctx, "Supplemental "+code, code)
	if err != nil {
		t.Fatal(err)
	}
	location, err := catalogapp.NewService(catalog.Categories(), catalog.Locations(), catalog.Items()).CreateLocation(ctx, "Supplemental "+code, catalogdomain.LocationStorage, nil)
	if err != nil {
		t.Fatal(err)
	}
	unit := "kg"
	item, err := catalogapp.NewService(catalog.Categories(), catalog.Locations(), catalog.Items()).CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{Description: "Supplemental " + code, CategoryID: category.ID, Kind: catalogdomain.KindConsumable, Unit: &unit}})
	if err != nil {
		t.Fatal(err)
	}
	toolCatalog := catalogapp.NewService(catalog.Categories(), catalog.Locations(), catalog.Items())
	templateTool, err := toolCatalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{Description: "Template tool " + code, CategoryID: category.ID, Kind: catalogdomain.KindAsset}})
	if err != nil {
		t.Fatal(err)
	}
	additionalTool, err := toolCatalog.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: catalogdomain.NewItemInput{Description: "Additional tool " + code, CategoryID: category.ID, Kind: catalogdomain.KindAsset}})
	if err != nil {
		t.Fatal(err)
	}
	userStock := stockapp.NewService(stockpg.New(pool))
	if _, err := userStock.Record(ctx, item.ID, userID, stockdomain.Request{Type: stockdomain.TypeIn, Qty: stockQty, LocationID: &location.ID}); err != nil {
		t.Fatal(err)
	}
	templates := projectpg.New(pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Supplemental " + code, Characteristics: []string{}, Consumables: []projectdomain.Material{{ItemID: item.ID, Quantity: perUnit}}, Tools: []string{templateTool.ID}, EstimatedDays: 1}}
	if err := templates.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	return supplementalMaterialFixture{pool: pool, ctx: ctx, userID: userID, customerID: customerID, locationID: location.ID, itemID: item.ID, templateToolID: templateTool.ID, additionalToolID: additionalTool.ID, projectID: project.ID, templates: templates, service: executionapp.NewService(executionpg.New(pool), templates, userStock)}
}

func (f supplementalMaterialFixture) create(units int) executiondomain.Execution {
	created, err := f.service.Create(f.ctx, executiondomain.CreateInput{ProjectID: f.projectID, Units: units, CustomerIDs: []int64{f.customerID}, PrincipalUserID: f.userID})
	if err != nil {
		panic(err)
	}
	return created
}

func TestExecutionShortageCreatesProcurementRequest(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(1000), quantity.Quantity(1250))
	execution := f.create(1)
	if execution.ShortageRequestID < 1 {
		t.Fatalf("short execution has no procurement request: %+v", execution)
	}
	var requestCount int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_shortage_requests WHERE execution_id=$1 AND id=$2`, execution.ID, execution.ShortageRequestID).Scan(&requestCount); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 {
		t.Fatalf("short execution has %d procurement requests, want one", requestCount)
	}

	sqlDB, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(f.ctx, 19); err == nil {
		t.Fatal("migration rollback removed shortage request history")
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_shortage_requests WHERE execution_id=$1`, execution.ID).Scan(&requestCount); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 {
		t.Fatalf("rejected rollback changed shortage requests: count=%d", requestCount)
	}
}

func TestNewReservationShortageCreatesSingleRequest(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(5000), quantity.Quantity(1250))
	first := f.create(1)
	if first.ShortageRequestID != 0 {
		t.Fatalf("fully reserved execution unexpectedly created shortage request %d", first.ShortageRequestID)
	}
	second := f.create(4)
	if second.ShortageRequestID < 1 {
		t.Fatalf("execution with a FIFO deficit has no shortage request: %+v", second)
	}
	var requestCount int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_shortage_requests WHERE execution_id=$1`, second.ID).Scan(&requestCount); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 {
		t.Fatalf("reservation updates created %d requests for one execution, want one", requestCount)
	}
}

func TestCancellingExecutionClosesItsShortageRequest(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(1000), quantity.Quantity(1250))
	execution := f.create(1)
	if execution.ShortageRequestID < 1 {
		t.Fatal("expected an initial shortage request")
	}
	if _, err := executionpg.New(f.pool).Transition(f.ctx, execution.ID, executiondomain.StatusRegistered, executiondomain.StatusCancelled); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := f.pool.QueryRow(f.ctx, `SELECT status FROM manufacturing_execution_shortage_requests WHERE execution_id=$1`, execution.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("shortage request status = %q, want cancelled", status)
	}
}

func TestLinkPurchaseLineTracksPendingShortageWithoutChangingStock(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(500), quantity.Quantity(2500))
	execution := f.create(1)
	if execution.ShortageRequestID == 0 {
		t.Fatal("expected a shortage request")
	}
	var supplierID, orderID, lineID int64
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO suppliers(name) VALUES($1) RETURNING id`, fmt.Sprintf("Shortage supplier %d", time.Now().UnixNano())).Scan(&supplierID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO purchase_orders(supplier_id,currency,status) VALUES($1,'USD','ordered') RETURNING id`, supplierID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO purchase_order_lines(order_id,position,item_id,quantity,unit_cost_minor) VALUES($1,1,$2::uuid,1500,100) RETURNING id`, orderID, f.itemID).Scan(&lineID); err != nil {
		t.Fatal(err)
	}
	repo := executionpg.New(f.pool)
	if _, err := repo.LinkPurchaseOrderLine(f.ctx, execution.ID, lineID, quantity.Quantity(1500)); err != nil {
		t.Fatal(err)
	}
	linked, err := repo.Get(f.ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(linked.Materials) != 1 || linked.Materials[0].PurchasedPendingQuantity != quantity.Quantity(1500) || linked.Materials[0].UnpurchasedQuantity != quantity.Quantity(500) {
		t.Fatalf("purchase tracking = %+v", linked.Materials)
	}
	if _, err := repo.LinkPurchaseOrderLine(f.ctx, execution.ID, lineID, quantity.Quantity(1500)); err != nil {
		t.Fatalf("identical link replay: %v", err)
	}
	var linkCount int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM manufacturing_execution_purchase_links WHERE shortage_request_id=$1`, execution.ShortageRequestID).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 1 {
		t.Fatalf("replayed link created %d records", linkCount)
	}
	if _, err := repo.LinkPurchaseOrderLine(f.ctx, execution.ID, lineID, quantity.Quantity(1000)); !errors.Is(err, executiondomain.ErrInvalidInput) {
		t.Fatalf("changed link replay error = %v, want invalid input", err)
	}
	var excessLineID int64
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO purchase_order_lines(order_id,position,item_id,quantity,unit_cost_minor) VALUES($1,2,$2::uuid,1000,100) RETURNING id`, orderID, f.itemID).Scan(&excessLineID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LinkPurchaseOrderLine(f.ctx, execution.ID, excessLineID, quantity.Quantity(1000)); !errors.Is(err, executiondomain.ErrInvalidInput) {
		t.Fatalf("over-purchase link error = %v, want invalid input", err)
	}
	var stock quantity.Quantity
	if err := f.pool.QueryRow(f.ctx, `SELECT stock_current FROM items WHERE id=$1::uuid`, f.itemID).Scan(&stock); err != nil {
		t.Fatal(err)
	}
	if stock != quantity.Quantity(500) {
		t.Fatalf("linking a purchase changed physical stock to %v", stock)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE purchase_orders SET status='cancelled' WHERE id=$1`, orderID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := repo.Get(f.ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Materials[0].PurchasedPendingQuantity != 0 || cancelled.Materials[0].UnpurchasedQuantity != quantity.Quantity(2000) {
		t.Fatalf("cancelled order still covers a shortage: %+v", cancelled.Materials[0])
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

func TestProduceFinishedUnitsIsPartialSerializedAndIdempotent(t *testing.T) {
	pool, ctx := setupExecutions(t)
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Maker','maker-t7@example.com','x','worker') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Customer T7') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	templateRepo := projectpg.New(pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Marmita T7", Characteristics: []string{}, Consumables: []projectdomain.Material{}, Tools: []string{}, EstimatedDays: 2}}
	if err := templateRepo.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	svc := executionapp.NewService(executionpg.New(pool), templateRepo)
	execution, err := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 3, CustomerIDs: []int64{customerID}, PrincipalUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	request := executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000071", Quantity: 2}
	if _, err := svc.Produce(ctx, execution.ID, userID, request); !errors.Is(err, executiondomain.ErrInvalidState) {
		t.Fatalf("registered execution produced units: %v", err)
	}
	if _, err := svc.Start(ctx, execution.ID); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Produce(ctx, execution.ID, userID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].Code != "PRD-000001" || first[1].Code != "PRD-000002" || first[0].ID == first[1].ID {
		t.Fatalf("produced units = %+v", first)
	}
	replayed, err := svc.Produce(ctx, execution.ID, userID, request)
	if err != nil || len(replayed) != 2 || replayed[0].ID != first[0].ID || replayed[1].ID != first[1].ID {
		t.Fatalf("production replay = %+v, err=%v", replayed, err)
	}
	events, err := svc.ListEvents(ctx, execution.ID)
	if err != nil || len(events) != 1 || events[0].Type != "production" || events[0].ActorID != userID || events[0].ActorName != "Maker" {
		t.Fatalf("production events = %+v, err=%v", events, err)
	}
	if _, err := svc.Produce(ctx, execution.ID, userID, executiondomain.ProduceInput{RequestID: request.RequestID, Quantity: 1}); !errors.Is(err, executiondomain.ErrProductionReplay) {
		t.Fatalf("changed replay = %v", err)
	}
	if _, err := svc.Produce(ctx, execution.ID, userID, executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000072", Quantity: 2}); !errors.Is(err, executiondomain.ErrProductionLimit) {
		t.Fatalf("overproduction = %v", err)
	}
	current, err := svc.Get(ctx, execution.ID)
	if err != nil || current.ProducedUnits != 2 {
		t.Fatalf("produced count = %d, err=%v", current.ProducedUnits, err)
	}
	listed, err := svc.ListUnits(ctx, execution.ID)
	if err != nil || len(listed) != 2 || listed[0].Code != first[0].Code {
		t.Fatalf("listed units = %+v, err=%v", listed, err)
	}
	final, err := svc.Produce(ctx, execution.ID, userID, executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000073", Quantity: 1})
	if err != nil || len(final) != 1 || final[0].Code != "PRD-000003" {
		t.Fatalf("final unit = %+v, err=%v", final, err)
	}
	var batchCount, unitCount int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM manufacturing_production_batches),(SELECT count(*) FROM finished_product_units)`).Scan(&batchCount, &unitCount); err != nil || batchCount != 2 || unitCount != 3 {
		t.Fatalf("persisted production = batches %d units %d err=%v", batchCount, unitCount, err)
	}
	second, err := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 2, CustomerIDs: []int64{customerID}, PrincipalUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Produce(ctx, second.ID, userID, request); !errors.Is(err, executiondomain.ErrProductionReplay) {
		t.Fatalf("cross-execution request replay = %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, requestID := range []string{"00000000-0000-4000-8000-000000000074", "00000000-0000-4000-8000-000000000075"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, produceErr := svc.Produce(ctx, second.ID, userID, executiondomain.ProduceInput{RequestID: id, Quantity: 2})
			results <- produceErr
		}(requestID)
	}
	wg.Wait()
	close(results)
	success, limited := 0, 0
	for result := range results {
		switch {
		case result == nil:
			success++
		case errors.Is(result, executiondomain.ErrProductionLimit):
			limited++
		default:
			t.Fatalf("concurrent production error = %v", result)
		}
	}
	if success != 1 || limited != 1 {
		t.Fatalf("concurrent production outcomes = %d success, %d limited", success, limited)
	}
	secondUnits, err := svc.ListUnits(ctx, second.ID)
	if err != nil || len(secondUnits) != 2 || secondUnits[0].Code == secondUnits[1].Code {
		t.Fatalf("concurrent finished units = %+v, err=%v", secondUnits, err)
	}
	sqlDB, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 22); err == nil {
		t.Fatal("finished-product history was removed by rollback")
	}
	var retainedUnits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM finished_product_units`).Scan(&retainedUnits); err != nil || retainedUnits != 5 {
		t.Fatalf("rejected rollback changed finished-product stock: units=%d err=%v", retainedUnits, err)
	}
	if _, err := svc.Pause(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Produce(ctx, second.ID, userID, executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000076", Quantity: 1}); !errors.Is(err, executiondomain.ErrInvalidState) {
		t.Fatalf("paused execution produced units: %v", err)
	}
}

func TestTerminalExecutionReleasesReservationsWithoutCreditingDeliveredStock(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(2000), quantity.Quantity(1000))
	older := f.create(2)
	newer := f.create(1)
	if older.Materials[0].ReservedQuantity != quantity.Quantity(2000) || newer.Materials[0].ReservedQuantity != 0 {
		t.Fatalf("initial reservations: older=%+v newer=%+v", older.Materials, newer.Materials)
	}
	if _, err := f.service.Complete(f.ctx, older.ID); !errors.Is(err, executiondomain.ErrInvalidState) {
		t.Fatalf("registered execution completed: %v", err)
	}
	if _, err := f.service.Start(f.ctx, older.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Complete(f.ctx, older.ID); !errors.Is(err, executiondomain.ErrProductionIncomplete) {
		t.Fatalf("unproduced execution completed: %v", err)
	}
	if _, err := f.service.Produce(f.ctx, older.ID, f.userID, executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000081", Quantity: 2}); err != nil {
		t.Fatal(err)
	}
	completed, err := f.service.Complete(f.ctx, older.ID)
	if err != nil || completed.Status != executiondomain.StatusCompleted || completed.Materials[0].ReservedQuantity != 0 || completed.ProducedUnits != 2 {
		t.Fatalf("completed execution = %+v, err=%v", completed, err)
	}
	newer, err = f.service.Get(f.ctx, newer.ID)
	if err != nil || newer.Materials[0].ReservedQuantity != quantity.Quantity(1000) {
		t.Fatalf("released stock was not reserved for next execution: %+v, err=%v", newer.Materials, err)
	}
	if _, err := f.service.Cancel(f.ctx, newer.ID); err != nil {
		t.Fatal(err)
	}
	third := f.create(2)
	if _, err := f.service.Start(f.ctx, third.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Produce(f.ctx, third.ID, f.userID, executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000082", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DeliverMaterial(f.ctx, third.ID, f.itemID, f.userID, "00000000-0000-4000-8000-000000000083", quantity.Quantity(1000), f.locationID, "used"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.service.Cancel(f.ctx, third.ID)
	if err != nil || cancelled.Status != executiondomain.StatusCancelled || cancelled.ProducedUnits != 1 || cancelled.Materials[0].ReservedQuantity != 0 {
		t.Fatalf("cancelled execution = %+v, err=%v", cancelled, err)
	}
	units, err := f.service.ListUnits(f.ctx, third.ID)
	if err != nil || len(units) != 1 {
		t.Fatalf("cancelled execution lost production: %+v, err=%v", units, err)
	}
	var stock quantity.Quantity
	if err := f.pool.QueryRow(f.ctx, `SELECT stock_current FROM items WHERE id=$1::uuid`, f.itemID).Scan(&stock); err != nil || stock != quantity.Quantity(1000) {
		t.Fatalf("terminal transition credited physical stock: %s, err=%v", stock, err)
	}
}

func TestCancellationAndMaterialAdditionDoNotLeaveStaleReservations(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(2000), quantity.Quantity(1000))
	execution := f.create(1)
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	addResult := make(chan error, 1)
	closeResult := make(chan error, 1)
	go func() {
		<-start
		_, err := f.service.AddMaterialRequirement(ctx, execution.ID, f.userID, executiondomain.AddMaterialInput{
			RequestID: "00000000-0000-4000-8000-000000000091", ItemID: f.itemID, Quantity: quantity.Quantity(500),
		})
		addResult <- err
	}()
	go func() {
		<-start
		_, err := f.service.Cancel(ctx, execution.ID)
		closeResult <- err
	}()
	close(start)
	if err := <-closeResult; err != nil {
		t.Fatalf("cancel = %v", err)
	}
	if err := <-addResult; err != nil && !errors.Is(err, executiondomain.ErrMaterialAdditionClosed) {
		t.Fatalf("concurrent addition = %v", err)
	}
	current, err := f.service.Get(f.ctx, execution.ID)
	if err != nil || current.Status != executiondomain.StatusCancelled || current.Materials[0].ReservedQuantity != 0 {
		t.Fatalf("cancel/addition left a reservation: %+v, err=%v", current, err)
	}
}

func TestFinishedUnitDeliveriesAllocateOnlyToExecutionCustomers(t *testing.T) {
	pool, ctx := setupExecutions(t)
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Dispatcher','dispatcher-t7@example.com','x','worker') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	customers := make([]int64, 3)
	for i, name := range []string{"Client A", "Client B", "Other client"} {
		if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES($1) RETURNING id`, name).Scan(&customers[i]); err != nil {
			t.Fatal(err)
		}
	}
	templates := projectpg.New(pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Marmita delivery", Characteristics: []string{}, Consumables: []projectdomain.Material{}, Tools: []string{}, EstimatedDays: 1}}
	if err := templates.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	svc := executionapp.NewService(executionpg.New(pool), templates)
	execution, err := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 3, CustomerIDs: customers[:2], PrincipalUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(ctx, execution.ID); err != nil {
		t.Fatal(err)
	}
	units, err := svc.Produce(ctx, execution.ID, userID, executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000101", Quantity: 3})
	if err != nil {
		t.Fatal(err)
	}
	availableStock, total, _, err := svc.ListFinishedStock(ctx, executiondomain.FinishedStockFilter{Status: "available", Q: units[0].Code})
	if err != nil || total != 1 || len(availableStock) != 1 || availableStock[0].Code != units[0].Code || availableStock[0].ProjectName != "Marmita delivery" {
		t.Fatalf("available finished stock = %+v, total=%d, err=%v", availableStock, total, err)
	}
	firstInput := executiondomain.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000000102", CustomerID: customers[0], UnitIDs: []int64{units[0].ID}}
	first, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, firstInput)
	if err != nil || first.Number != "DEL-000001" || first.CustomerID != customers[0] || len(first.UnitCodes) != 1 || first.UnitCodes[0] != units[0].Code {
		t.Fatalf("first delivery = %+v, err=%v", first, err)
	}
	replayed, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, firstInput)
	if err != nil || replayed.ID != first.ID || replayed.Number != first.Number {
		t.Fatalf("delivery replay = %+v, err=%v", replayed, err)
	}
	events, err := svc.ListEvents(ctx, execution.ID)
	if err != nil || len(events) != 2 || events[1].Type != "delivery" || events[1].ActorID != userID || events[1].ActorName != "Dispatcher" {
		t.Fatalf("delivery events = %+v, err=%v", events, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE customers SET name='Renamed client' WHERE id=$1`, customers[0]); err != nil {
		t.Fatal(err)
	}
	if first.CustomerName != "Client A" {
		t.Fatalf("receipt lost customer name snapshot: %+v", first)
	}
	deliveredStock, total, _, err := svc.ListFinishedStock(ctx, executiondomain.FinishedStockFilter{Status: "delivered", Q: "Marmita delivery"})
	if err != nil || total != 1 || len(deliveredStock) != 1 || deliveredStock[0].Code != units[0].Code || deliveredStock[0].CustomerName != "Client A" {
		t.Fatalf("delivered finished stock = %+v, total=%d, err=%v", deliveredStock, total, err)
	}
	if _, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, executiondomain.DeliveryInput{RequestID: firstInput.RequestID, CustomerID: customers[1], UnitIDs: firstInput.UnitIDs}); !errors.Is(err, executiondomain.ErrDeliveryReplay) {
		t.Fatalf("changed delivery replay = %v", err)
	}
	if _, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, executiondomain.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000000103", CustomerID: customers[1], UnitIDs: []int64{units[0].ID}}); !errors.Is(err, executiondomain.ErrUnitDelivered) {
		t.Fatalf("duplicate delivery = %v", err)
	}
	if _, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, executiondomain.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000000104", CustomerID: customers[2], UnitIDs: []int64{units[2].ID}}); !errors.Is(err, executiondomain.ErrCustomerNotAssigned) {
		t.Fatalf("unassigned delivery client = %v", err)
	}
	second, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, executiondomain.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000000105", CustomerID: customers[1], UnitIDs: []int64{units[1].ID}})
	if err != nil || second.CustomerID != customers[1] || second.Number != "DEL-000002" {
		t.Fatalf("second delivery = %+v, err=%v", second, err)
	}
	currentUnits, err := svc.ListUnits(ctx, execution.ID)
	if err != nil || len(currentUnits) != 3 || currentUnits[0].CustomerID == nil || *currentUnits[0].CustomerID != customers[0] || currentUnits[2].CustomerID != nil {
		t.Fatalf("unit allocation = %+v, err=%v", currentUnits, err)
	}
	if _, err := svc.Cancel(ctx, execution.ID); err != nil {
		t.Fatal(err)
	}
	late, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, executiondomain.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000000106", CustomerID: customers[0], UnitIDs: []int64{units[2].ID}})
	if err != nil || len(late.UnitCodes) != 1 || late.UnitCodes[0] != units[2].Code {
		t.Fatalf("finished stock from cancelled execution was lost: %+v, err=%v", late, err)
	}
	listed, err := svc.ListDeliveries(ctx, execution.ID)
	if err != nil || len(listed) != 3 || listed[0].ID != first.ID || listed[0].CustomerName != "Client A" {
		t.Fatalf("delivery history = %+v, err=%v", listed, err)
	}
	secondExecution, err := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 1, CustomerIDs: customers[:2], PrincipalUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(ctx, secondExecution.ID); err != nil {
		t.Fatal(err)
	}
	foreign, err := svc.Produce(ctx, secondExecution.ID, userID, executiondomain.ProduceInput{RequestID: "00000000-0000-4000-8000-000000000107", Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeliverFinishedUnits(ctx, execution.ID, userID, executiondomain.DeliveryInput{RequestID: "00000000-0000-4000-8000-000000000108", CustomerID: customers[0], UnitIDs: []int64{foreign[0].ID}}); !errors.Is(err, executiondomain.ErrUnitUnavailable) {
		t.Fatalf("cross-execution delivery = %v", err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, customerID := range customers[:2] {
		go func(i int, clientID int64) {
			<-start
			requestID := fmt.Sprintf("00000000-0000-4000-8000-%012d", 109+i)
			_, deliveryErr := svc.DeliverFinishedUnits(ctx, secondExecution.ID, userID, executiondomain.DeliveryInput{RequestID: requestID, CustomerID: clientID, UnitIDs: []int64{foreign[0].ID}})
			results <- deliveryErr
		}(index, customerID)
	}
	close(start)
	success, duplicate := 0, 0
	for range 2 {
		switch result := <-results; {
		case result == nil:
			success++
		case errors.Is(result, executiondomain.ErrUnitDelivered):
			duplicate++
		default:
			t.Fatalf("concurrent delivery error = %v", result)
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatalf("concurrent delivery outcomes = %d success, %d duplicate", success, duplicate)
	}
	sqlDB, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 23); err == nil {
		t.Fatal("delivery migration discarded receipt history")
	}
	var receiptCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM manufacturing_deliveries`).Scan(&receiptCount); err != nil || receiptCount != 4 {
		t.Fatalf("rejected rollback changed receipts: count=%d err=%v", receiptCount, err)
	}
}

func TestExecutionManualIncidentsRetainActorAndReplay(t *testing.T) {
	pool, ctx := setupExecutions(t)
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('Recorder','recorder-t8@example.com','x','worker') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers(name) VALUES('Client T8') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	templates := projectpg.New(pool)
	project := &projectdomain.Project{Input: projectdomain.Input{Name: "Marmita events", Characteristics: []string{}, Consumables: []projectdomain.Material{}, Tools: []string{}, EstimatedDays: 1}}
	if err := templates.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	svc := executionapp.NewService(executionpg.New(pool), templates)
	execution, err := svc.Create(ctx, executiondomain.CreateInput{ProjectID: project.ID, Units: 1, CustomerIDs: []int64{customerID}, PrincipalUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	input := executiondomain.IncidentInput{RequestID: "00000000-0000-4000-8000-000000000201", Type: "negative", Message: "Material defectuoso"}
	first, err := svc.RecordIncident(ctx, execution.ID, userID, input)
	if err != nil || first.Type != "negative" || first.ActorID != userID || first.ActorName != "Recorder" || first.Message != input.Message {
		t.Fatalf("recorded incident = %+v, err=%v", first, err)
	}
	replay, err := svc.RecordIncident(ctx, execution.ID, userID, input)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("incident replay = %+v, err=%v", replay, err)
	}
	if _, err := svc.RecordIncident(ctx, execution.ID, userID, executiondomain.IncidentInput{RequestID: input.RequestID, Type: "positive", Message: "Resuelto"}); !errors.Is(err, executiondomain.ErrIncidentReplay) {
		t.Fatalf("changed incident replay = %v", err)
	}
	events, err := svc.ListEvents(ctx, execution.ID)
	if err != nil || len(events) != 1 || events[0].ID != first.ID {
		t.Fatalf("execution events = %+v, err=%v", events, err)
	}
}
