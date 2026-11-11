package postgres_test

import (
	"context"
	identitypg "github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	importpg "github.com/edgar/inventario/backend/internal/importer/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"os"
	"testing"
)

func TestResolveActorUsesPersistedPermissions(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := t.Context()
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := identitypg.New(pool)
	original, err := repo.RolePermissions(ctx, domain.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := repo.SetRolePermissions(context.Background(), domain.RoleOperator, original); err != nil {
			t.Error(err)
		}
	}()
	u := domain.User{Name: "Import permission test", Email: "import-permission-test@example.com", PasswordHash: "unused", Role: domain.RoleOperator, Active: true}
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID)
		if err != nil {
			t.Error(err)
		}
	}()
	if _, err := importpg.ResolveActor(ctx, pool, u.Email); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetRolePermissions(ctx, domain.RoleOperator, []domain.Permission{}); err != nil {
		t.Fatal(err)
	}
	if _, err := importpg.ResolveActor(ctx, pool, u.Email); err == nil {
		t.Fatal("import accepted revoked role permissions")
	}
}
