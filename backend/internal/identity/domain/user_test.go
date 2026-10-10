package domain

import "testing"

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		role Role
		perm Permission
		want bool
	}{
		{RoleViewer, PermRead, true},
		{RoleViewer, PermWrite, false},
		{RoleViewer, PermManageUsers, false},
		{RoleOperator, PermRead, true},
		{RoleOperator, PermWrite, true},
		{RoleOperator, PermManageCatalogs, false},
		{RoleOperator, PermManageUsers, false},
		{RoleAdmin, PermRead, true},
		{RoleAdmin, PermWrite, true},
		{RoleAdmin, PermManageCatalogs, true},
		{RoleAdmin, PermManageUsers, true},
		{Role("ghost"), PermRead, false},
	}
	for _, tt := range tests {
		if got := tt.role.Has(tt.perm); got != tt.want {
			t.Errorf("%s.Has(%s) = %v, want %v", tt.role, tt.perm, got, tt.want)
		}
	}
}

func TestRoleValid(t *testing.T) {
	if !RoleAdmin.Valid() || !RoleOperator.Valid() || !RoleViewer.Valid() {
		t.Fatal("known roles must be valid")
	}
	if Role("root").Valid() || Role("").Valid() {
		t.Fatal("unknown roles must be invalid")
	}
}

func TestCanChangeTo(t *testing.T) {
	admin := User{Role: RoleAdmin, Active: true}
	if err := admin.CanChangeTo(RoleViewer, true, 1); err != ErrLastAdmin {
		t.Errorf("demoting last admin: got %v", err)
	}
	if err := admin.CanChangeTo(RoleAdmin, false, 1); err != ErrLastAdmin {
		t.Errorf("deactivating last admin: got %v", err)
	}
	if err := admin.CanChangeTo(RoleViewer, true, 2); err != nil {
		t.Errorf("demoting one of two admins: got %v", err)
	}
	if err := admin.CanChangeTo(RoleAdmin, true, 1); err != nil {
		t.Errorf("no-op change: got %v", err)
	}
	viewer := User{Role: RoleViewer, Active: true}
	if err := viewer.CanChangeTo(RoleOperator, false, 1); err != nil {
		t.Errorf("non-admin change: got %v", err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got, err := NormalizeEmail("  Ana@Example.COM "); err != nil || got != "ana@example.com" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "nope", "Ana <ana@example.com>", "a@b@c"} {
		if _, err := NormalizeEmail(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidatePasswordAndName(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Error("short password accepted")
	}
	if ValidatePassword("long-enough-password") != nil {
		t.Error("valid password rejected")
	}
	if _, err := NormalizeName("   "); err == nil {
		t.Error("blank name accepted")
	}
}

func TestValidID(t *testing.T) {
	if !ValidID("123e4567-e89b-12d3-a456-426614174000") {
		t.Error("valid uuid rejected")
	}
	if ValidID("42") || ValidID("") {
		t.Error("invalid id accepted")
	}
}
