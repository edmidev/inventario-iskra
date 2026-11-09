package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edgar/inventario/backend/internal/executions/app"
	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
	"github.com/go-chi/chi/v5"
)

type routeRepo struct{}

func (routeRepo) Create(context.Context, domain.Execution) (domain.Execution, error) {
	return domain.Execution{}, nil
}
func (routeRepo) AddMaterialRequirement(context.Context, int64, string, string, quantity.Quantity, string) (domain.Execution, error) {
	return domain.Execution{}, nil
}
func (routeRepo) AddToolRequirement(context.Context, int64, string, string, string, string) (domain.Execution, error) {
	return domain.Execution{}, nil
}
func (routeRepo) Get(context.Context, int64) (domain.Execution, error) {
	return domain.Execution{}, nil
}
func (routeRepo) List(context.Context, domain.Filter) ([]domain.Execution, int, error) {
	return nil, 0, nil
}
func (routeRepo) Transition(context.Context, int64, domain.Status, domain.Status) (domain.Execution, error) {
	return domain.Execution{}, nil
}
func (routeRepo) ListAssignableUsers(context.Context) ([]domain.AssignableUser, error) {
	return []domain.AssignableUser{{ID: "active-user", Name: "Active User"}}, nil
}

type routeTemplates struct{}

func (routeTemplates) Get(context.Context, int64) (projectdomain.Project, error) {
	return projectdomain.Project{}, projectdomain.ErrNotFound
}

func routeHandler(permissions []string) http.Handler {
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := httpx.Principal{UserID: "requester", Permissions: permissions}
			next.ServeHTTP(w, r.WithContext(httpx.WithPrincipal(r.Context(), p)))
		})
	}
	h := New(app.NewService(routeRepo{}, routeTemplates{}), auth, Permissions{Read: "executions:read", Write: "executions:write"})
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestAssignableUsersRequiresExecutionReadPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		wantStatus  int
	}{
		{name: "read grant", permissions: []string{"executions:read"}, wantStatus: http.StatusOK},
		{name: "missing grant", wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/executions/assignable-users", nil)
			res := httptest.NewRecorder()
			routeHandler(tt.permissions).ServeHTTP(res, req)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, tt.wantStatus, res.Body.String())
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(res.Body.String(), "active-user") {
				t.Fatalf("assignable user missing from response: %s", res.Body.String())
			}
		})
	}
}

func TestExecutionCreationRequiresWritePermission(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/executions/", strings.NewReader(`{"project_id":1,"units":1,"customer_ids":[1],"principal_user_id":"u"}`))
	res := httptest.NewRecorder()
	routeHandler([]string{"executions:read"}).ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusForbidden, res.Body.String())
	}
}

func TestAddingExecutionMaterialRequiresWritePermission(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/executions/1/materials", strings.NewReader(`{"request_id":"00000000-0000-4000-8000-000000000001","item_id":"00000000-0000-4000-8000-000000000002","quantity":1}`))
	res := httptest.NewRecorder()
	routeHandler([]string{"executions:read"}).ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusForbidden, res.Body.String())
	}
}

func TestAddingExecutionMaterialAcceptsExecutionTotal(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/executions/1/materials", strings.NewReader(`{"request_id":"00000000-0000-4000-8000-000000000001","item_id":"00000000-0000-4000-8000-000000000002","quantity":2.5}`))
	res := httptest.NewRecorder()
	routeHandler([]string{"executions:write"}).ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}
}

func TestAddingExecutionToolRequiresWritePermission(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/executions/1/tools", strings.NewReader(`{"request_id":"00000000-0000-4000-8000-000000000001","item_id":"00000000-0000-4000-8000-000000000002"}`))
	res := httptest.NewRecorder()
	routeHandler([]string{"executions:read"}).ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusForbidden, res.Body.String())
	}
}

func TestAddingExecutionToolReturnsExecution(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/executions/1/tools", strings.NewReader(`{"request_id":"00000000-0000-4000-8000-000000000001","item_id":"00000000-0000-4000-8000-000000000002"}`))
	res := httptest.NewRecorder()
	routeHandler([]string{"executions:write"}).ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}
}
