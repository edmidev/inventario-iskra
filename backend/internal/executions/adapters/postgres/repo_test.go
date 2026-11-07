package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	executionpg "github.com/edgar/inventario/backend/internal/executions/adapters/postgres"
	executionapp "github.com/edgar/inventario/backend/internal/executions/app"
	executiondomain "github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	projectpg "github.com/edgar/inventario/backend/internal/projects/adapters/postgres"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
	"github.com/jackc/pgx/v5/pgxpool"
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
	if _, err := pool.Exec(ctx, `TRUNCATE manufacturing_execution_workers, manufacturing_execution_customers, manufacturing_executions, project_templates, customers, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return pool, ctx
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
	templates := projectpg.New(pool)
	project := projectdomain.Input{
		Name: "Marmita", Description: "Acero inoxidable", Characteristics: []string{"Aislamiento"},
		Consumables: []projectdomain.Material{{ItemID: "item-42", Quantity: quantity.Quantity(1250)}},
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
