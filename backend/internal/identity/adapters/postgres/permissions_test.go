package postgres_test

import (
	"context"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"reflect"
	"testing"
)

func TestRolePermissionsPersistAndProtectAdmin(t *testing.T) {
	r, pool := setup(t)
	ctx := t.Context()
	initial, err := r.RolePermissions(ctx, domain.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.SetRolePermissions(context.Background(), domain.RoleOperator, initial); err != nil {
			t.Error(err)
		}
	})
	grants := []domain.Permission{domain.PermRead, domain.PermManageUsers}
	if err := r.SetRolePermissions(ctx, domain.RoleOperator, grants); err != nil {
		t.Fatal(err)
	}
	got, err := r.RolePermissions(ctx, domain.RoleOperator)
	if err != nil || !reflect.DeepEqual(got, grants) {
		t.Fatalf("grants: %v %v", got, err)
	}
	if err := r.SetRolePermissions(ctx, domain.RoleAdmin, []domain.Permission{}); err == nil {
		t.Fatal("admin lockout accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_roles SET permissions='{}' WHERE role='admin'`); err == nil {
		t.Fatal("database must reject admin lockout")
	}
	for _, role := range []domain.Role{domain.RoleWorkshopChief, domain.RoleSupervisor, domain.RoleInventoryChief, domain.RoleWorker} {
		if ps, err := r.RolePermissions(ctx, role); err != nil || len(ps) != 0 {
			t.Fatalf("new grants %s: %v %v", role, ps, err)
		}
		mkUser(t, r, string(role)+"@example.com", role)
	}
}
