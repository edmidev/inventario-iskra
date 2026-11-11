package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestExecutionReservationMigrationBackfillsFIFOAndRejectsHistoryRollback(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("execution_reservations_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	}()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	database, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 16); err != nil {
		t.Fatal(err)
	}
	var userID, itemID string
	var projectID, customerID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO users(name,email,password_hash,role) VALUES('T5 owner','t5-migration@example.com','x','operator') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO customers(name) VALUES('T5 migration customer') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	var categoryID, locationID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO categories(name,code_prefix) VALUES('T5 migration category','T5MIG') RETURNING id`).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO locations(name,kind) VALUES('T5 migration storage','storage') RETURNING id`).Scan(&locationID); err != nil {
		t.Fatal(err)
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO items(code,description,category_id,kind,unit,stock_current,stock_min)
VALUES('ISK-T5MIG-001','T5 migration material',$1,'consumable','kg',4,0) RETURNING id::text`, categoryID).Scan(&itemID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO item_stocks(item_id,location_id,quantity) VALUES($1::uuid,$2,4)`, itemID, locationID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO project_templates(name,characteristics,consumables,tools,estimated_days)
VALUES('T5 migration template','[]','[]','[]',1) RETURNING id`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	snapshot := fmt.Sprintf(`{"project_id":%d,"project_name":"T5 migration template","description":"","characteristics":[],"consumables":[{"item_id":"%s","quantity_per_unit":1.5}],"tools":[],"template_estimated_days":1}`, projectID, itemID)
	for _, createdAt := range []string{"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"} {
		if _, err := database.ExecContext(ctx, `INSERT INTO manufacturing_executions(project_template_id,project_snapshot,units,estimated_days,status,principal_user_id,created_at)
VALUES($1,$2::jsonb,2,1,'registered',$3::uuid,$4::timestamptz)`, projectID, snapshot, userID, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO manufacturing_execution_customers(execution_id,customer_id) SELECT id,$1 FROM manufacturing_executions`, customerID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 17); err != nil {
		t.Fatal(err)
	}
	var oldestReserved, newerReserved, oldestRequired, newerRequired string
	if err := database.QueryRowContext(ctx, `SELECT quantity_required::text,quantity_reserved::text FROM manufacturing_execution_materials ORDER BY execution_id LIMIT 1`).Scan(&oldestRequired, &oldestReserved); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT quantity_required::text,quantity_reserved::text FROM manufacturing_execution_materials ORDER BY execution_id DESC LIMIT 1`).Scan(&newerRequired, &newerReserved); err != nil {
		t.Fatal(err)
	}
	if oldestRequired != "3.000" || oldestReserved != "3.000" || newerRequired != "3.000" || newerReserved != "1.000" {
		t.Fatalf("backfill requirements/reservations oldest=%s/%s newer=%s/%s", oldestRequired, oldestReserved, newerRequired, newerReserved)
	}
	before := reservationSnapshot(t, ctx, database)
	if _, err := provider.DownTo(ctx, 16); err == nil || !strings.Contains(err.Error(), "cannot roll back material reservations") {
		t.Fatalf("rollback with execution history error = %v", err)
	}
	after := reservationSnapshot(t, ctx, database)
	if before != after {
		t.Fatalf("rejected rollback changed reservation rows: before=%s after=%s", before, after)
	}
}

func reservationSnapshot(t *testing.T, ctx context.Context, database *sql.DB) string {
	t.Helper()
	var snapshot string
	if err := database.QueryRowContext(ctx, `SELECT string_agg(execution_id::text||':'||quantity_required::text||':'||quantity_reserved::text,',' ORDER BY execution_id) FROM manufacturing_execution_materials`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
