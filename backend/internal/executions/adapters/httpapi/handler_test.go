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
func (routeRepo) LinkPurchaseOrderLine(context.Context, int64, int64, quantity.Quantity) (domain.Execution, error) {
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
func (routeRepo) Close(context.Context, int64, domain.Status) (domain.Execution, error) {
	return domain.Execution{}, nil
}
func (routeRepo) Produce(context.Context, int64, string, domain.ProduceInput) ([]domain.FinishedUnit, error) {
	return []domain.FinishedUnit{}, nil
}
func (routeRepo) ListUnits(context.Context, int64) ([]domain.FinishedUnit, error) {
	return []domain.FinishedUnit{}, nil
}
func (routeRepo) DeliverFinishedUnits(context.Context, int64, string, domain.DeliveryInput, string) (domain.DeliveryReceipt, error) {
	return domain.DeliveryReceipt{}, nil
}
func (routeRepo) ListDeliveries(context.Context, int64) ([]domain.DeliveryReceipt, error) {
	return []domain.DeliveryReceipt{}, nil
}
func (routeRepo) ListFinishedStock(context.Context, domain.FinishedStockFilter) ([]domain.FinishedStockRow, int, error) {
	return []domain.FinishedStockRow{}, 0, nil
}
func (routeRepo) RecordIncident(context.Context, int64, string, domain.IncidentInput) (domain.ExecutionEvent, error) {
	return domain.ExecutionEvent{}, nil
}
func (routeRepo) ListEvents(context.Context, int64) ([]domain.ExecutionEvent, error) {
	return []domain.ExecutionEvent{}, nil
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

func TestFinishedStockListRequiresReadPermissionAndValidFilter(t *testing.T) {
	for _, tt := range []struct {
		name, path  string
		permissions []string
		want        int
	}{
		{name: "without read", path: "/finished-products", want: http.StatusForbidden},
		{name: "available stock", path: "/finished-products?status=available&limit=20", permissions: []string{"executions:read"}, want: http.StatusOK},
		{name: "invalid status", path: "/finished-products?status=unknown", permissions: []string{"executions:read"}, want: http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			routeHandler(tt.permissions).ServeHTTP(res, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if res.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, tt.want, res.Body.String())
			}
		})
	}
}

func TestExecutionEventRoutesRequireRespectivePermissions(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, body string
		permissions              []string
		want                     int
	}{
		{name: "timeline denied", method: http.MethodGet, path: "/executions/1/events", want: http.StatusForbidden},
		{name: "timeline readable", method: http.MethodGet, path: "/executions/1/events", permissions: []string{"executions:read"}, want: http.StatusOK},
		{name: "incident denied", method: http.MethodPost, path: "/executions/1/events", permissions: []string{"executions:read"}, want: http.StatusForbidden},
		{name: "incident validates body", method: http.MethodPost, path: "/executions/1/events", body: `{}`, permissions: []string{"executions:write"}, want: http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			routeHandler(tt.permissions).ServeHTTP(res, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			if res.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, tt.want, res.Body.String())
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

func TestLinkingExecutionPurchaseLineRequiresWritePermission(t *testing.T) {
	body := strings.NewReader(`{"purchase_order_line_id":42,"quantity":1.5}`)
	req := httptest.NewRequest(http.MethodPost, "/executions/1/purchase-lines", body)
	res := httptest.NewRecorder()
	routeHandler([]string{"executions:read"}).ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusForbidden, res.Body.String())
	}
}

func TestProductionRoutesEnforceReadAndWritePermissions(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		permissions []string
		wantStatus  int
	}{
		{name: "unit list requires read", method: http.MethodGet, path: "/executions/1/units", wantStatus: http.StatusForbidden},
		{name: "unit list with read", method: http.MethodGet, path: "/executions/1/units", permissions: []string{"executions:read"}, wantStatus: http.StatusOK},
		{name: "production requires write", method: http.MethodPost, path: "/executions/1/produce", body: `{"request_id":"00000000-0000-4000-8000-000000000071","quantity":1}`, permissions: []string{"executions:read"}, wantStatus: http.StatusForbidden},
		{name: "production with write", method: http.MethodPost, path: "/executions/1/produce", body: `{"request_id":"00000000-0000-4000-8000-000000000071","quantity":1}`, permissions: []string{"executions:write"}, wantStatus: http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			res := httptest.NewRecorder()
			routeHandler(tt.permissions).ServeHTTP(res, req)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, tt.wantStatus, res.Body.String())
			}
		})
	}
}

func TestTerminalExecutionRoutesRequireWritePermission(t *testing.T) {
	for _, action := range []string{"complete", "cancel"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/executions/1/"+action, nil)
			res := httptest.NewRecorder()
			routeHandler([]string{"executions:read"}).ServeHTTP(res, req)
			if res.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusForbidden, res.Body.String())
			}
		})
	}
}

func TestFinishedProductDeliveryRoutesRequirePermissions(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		permissions []string
		wantStatus  int
	}{
		{name: "list denied", method: http.MethodGet, wantStatus: http.StatusForbidden},
		{name: "list allowed", method: http.MethodGet, permissions: []string{"executions:read"}, wantStatus: http.StatusOK},
		{name: "deliver denied", method: http.MethodPost, permissions: []string{"executions:read"}, wantStatus: http.StatusForbidden},
		{name: "deliver allowed", method: http.MethodPost, permissions: []string{"executions:write"}, wantStatus: http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"request_id":"00000000-0000-4000-8000-000000000101","customer_id":1,"unit_ids":[1]}`)
			req := httptest.NewRequest(tt.method, "/executions/1/deliveries", body)
			res := httptest.NewRecorder()
			routeHandler(tt.permissions).ServeHTTP(res, req)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, tt.wantStatus, res.Body.String())
			}
		})
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
