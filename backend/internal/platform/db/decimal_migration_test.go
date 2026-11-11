package db_test

import (
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

// A private database keeps schema rollback probes independent of application fixtures.
func TestDecimalSchemaPreservesLegacyUnitsAndRejectsLossyRollback(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := t.Context()
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("decimal_migration_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); e != nil {
			t.Error(e)
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
	if _, err = provider.UpTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	var category int64
	if err = database.QueryRowContext(ctx, `INSERT INTO categories(name,code_prefix) VALUES('Migration fixture','MIG') RETURNING id`).Scan(&category); err != nil {
		t.Fatal(err)
	}
	var id string
	if err = database.QueryRowContext(ctx, `INSERT INTO items(code,description,category_id,kind,unit,stock_current,stock_min) VALUES('ISK-MIG-001','Legacy consumable',$1,'consumable','m',0,2) RETURNING id`, category).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 13); err != nil {
		t.Fatal(err)
	}
	var legacy string
	if err = database.QueryRowContext(ctx, `SELECT stock_min::text FROM items WHERE id=$1`, id).Scan(&legacy); err != nil || legacy != "2.000" {
		t.Fatalf("legacy units=%s error=%v", legacy, err)
	}
	var numericType string
	if err = database.QueryRowContext(ctx, `SELECT data_type FROM information_schema.columns WHERE table_name='stock_movements' AND column_name='qty'`).Scan(&numericType); err != nil || numericType != "numeric" {
		t.Fatalf("ledger type=%s error=%v", numericType, err)
	}
	if _, err = database.ExecContext(ctx, `UPDATE items SET stock_min=0.125 WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 12); err == nil || !strings.Contains(err.Error(), "fractional") {
		t.Fatalf("lossy rollback not rejected: %v", err)
	}
	var preserved string
	if err = database.QueryRowContext(ctx, `SELECT stock_min::text FROM items WHERE id=$1`, id).Scan(&preserved); err != nil || preserved != "0.125" {
		t.Fatalf("rejected rollback changed quantity=%s error=%v", preserved, err)
	}
	if _, err = database.ExecContext(ctx, `UPDATE items SET stock_min=2 WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	var whole int
	if err = database.QueryRowContext(ctx, `SELECT stock_min FROM items WHERE id=$1`, id).Scan(&whole); err != nil || whole != 2 {
		t.Fatalf("lossless rollback=%d error=%v", whole, err)
	}
}
