package httpapi_test

import (
	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"testing"
)

func TestDynamicPermissionsImmediate(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "admin@example.com", domain.RoleAdmin)
	e.seed(t, "op@example.com", domain.RoleOperator)
	admin, _ := e.login(t, "admin@example.com")
	op, _ := e.login(t, "op@example.com")
	if r := e.do(t, "GET", "/roles", admin, nil); r.StatusCode != 200 {
		t.Fatalf("list roles: %d", r.StatusCode)
	}
	for _, tc := range []struct {
		perms  []string
		status int
	}{{[]string{"users:manage"}, 200}, {[]string{}, 403}} {
		r := e.do(t, "PUT", "/roles/operator/permissions", admin, map[string]any{"permissions": tc.perms})
		if r.StatusCode != 200 {
			t.Fatalf("set: %d", r.StatusCode)
		}
		r = e.do(t, "GET", "/users", op, nil)
		if r.StatusCode != tc.status {
			t.Fatalf("same token permission: %d want %d", r.StatusCode, tc.status)
		}
	}
	r := e.do(t, "GET", "/auth/me", op, nil)
	var me struct {
		Permissions []string `json:"permissions"`
	}
	decodeBody(t, r, &me)
	if me.Permissions == nil || len(me.Permissions) != 0 {
		t.Fatalf("permissions: %#v", me.Permissions)
	}
}
func TestPermissionConfigurationSafety(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "admin@example.com", domain.RoleAdmin)
	admin, _ := e.login(t, "admin@example.com")
	for _, tc := range []struct {
		role  string
		perms []string
	}{{"admin", []string{}}, {"operator", []string{"made:up"}}, {"unknown", []string{}}, {"worker", []string{"inventory:read", "inventory:read"}}} {
		r := e.do(t, "PUT", "/roles/"+tc.role+"/permissions", admin, map[string]any{"permissions": tc.perms})
		if r.StatusCode != 400 {
			t.Fatalf("%s invalid: %d", tc.role, r.StatusCode)
		}
	}
}
func TestNewRolesHaveNoPermissions(t *testing.T) {
	e := newEnv(t)
	for _, role := range []domain.Role{"workshop_chief", "supervisor", "inventory_chief", "worker"} {
		email := string(role) + "@example.com"
		e.seed(t, email, role)
		token, _ := e.login(t, email)
		if r := e.do(t, "GET", "/users", token, nil); r.StatusCode != 403 {
			t.Fatalf("new role access: %d", r.StatusCode)
		}
		if r := e.do(t, "GET", "/auth/me", token, nil); r.StatusCode != 200 {
			t.Fatalf("identity: %d", r.StatusCode)
		}
	}
}

func TestStaleRoleAndInactiveUserCannotKeepAccess(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "admin@example.com", domain.RoleAdmin)
	e.seed(t, "other@example.com", domain.RoleAdmin)
	token, _ := e.login(t, "admin@example.com")
	users, err := e.svc.ListUsers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, u := range users {
		if u.Email == "admin@example.com" {
			id = u.ID
		}
	}
	viewer := domain.RoleViewer
	if _, err := e.svc.UpdateUser(t.Context(), id, app.UpdateUserInput{Role: &viewer}); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, "GET", "/users", token, nil); r.StatusCode != 403 {
		t.Fatalf("stale admin token: %d", r.StatusCode)
	}
	active := false
	if _, err := e.svc.UpdateUser(t.Context(), id, app.UpdateUserInput{Active: &active}); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, "GET", "/auth/me", token, nil); r.StatusCode != 401 {
		t.Fatalf("inactive user token: %d", r.StatusCode)
	}
}
