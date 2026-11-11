package main

import (
	"testing"

	identitydomain "github.com/edgar/inventario/backend/internal/identity/domain"
)

func TestExecutionPermissionsAreConfigurable(t *testing.T) {
	for _, permission := range []identitydomain.Permission{
		identitydomain.PermReadExecutions,
		identitydomain.PermWriteExecutions,
	} {
		if err := identitydomain.ValidateRolePermissions(identitydomain.RoleWorker, []identitydomain.Permission{permission}); err != nil {
			t.Fatalf("grant %s: %v", permission, err)
		}
	}
}
