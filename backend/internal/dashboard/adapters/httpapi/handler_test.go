package httpapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/dashboard/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/dashboard/adapters/memory"
	"github.com/edgar/inventario/backend/internal/dashboard/app"
	"github.com/edgar/inventario/backend/internal/dashboard/domain"
	identity "github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

var clock = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func newServer(t *testing.T) (*httptest.Server, *memory.Repo) {
	t.Helper()
	repo := memory.New()
	svc := app.NewService(repo, func() time.Time { return clock })
	verify := func(token string) (httpx.Principal, error) {
		role := identity.Role(token) // tokens are role names in these tests
		if !role.Valid() {
			return httpx.Principal{}, errors.New("bad token")
		}
		var perms []string
		for _, p := range role.Permissions() {
			perms = append(perms, string(p))
		}
		return httpx.Principal{UserID: "u-" + token, Role: token, Permissions: perms}, nil
	}
	h := httpapi.New(svc, httpx.Authenticate(verify), httpapi.Permissions{Read: string(identity.PermRead)})
	r := chi.NewRouter()
	r.Route("/api/v1", h.Register)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, repo
}

func get(t *testing.T, srv *httptest.Server, token string) (int, map[string]any) {
	t.Helper()
	return getPath(t, srv, "/api/v1/dashboard/summary", token)
}

func getPath(t *testing.T, srv *httptest.Server, path, token string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body
}

func TestSummaryRequiresAuthentication(t *testing.T) {
	srv, _ := newServer(t)
	for _, token := range []string{"", "garbage"} {
		if status, _ := get(t, srv, token); status != http.StatusUnauthorized {
			t.Errorf("token %q: got %d, want 401", token, status)
		}
	}
}

func TestSummaryIsReadableByEveryRole(t *testing.T) {
	srv, _ := newServer(t)
	for _, role := range []string{"viewer", "operator", "admin"} {
		if status, body := get(t, srv, role); status != http.StatusOK {
			t.Errorf("%s: got %d %v, want 200", role, status, body)
		}
	}
}

func TestSummaryShape(t *testing.T) {
	srv, repo := newServer(t)
	loc := "Bodega"
	repo.Set(domain.Summary{
		Items:    domain.ItemCounts{Total: 3, Assets: 2, Consumables: 1},
		Assets:   domain.AssetCounts{Available: 1, Unavailable: 1, OnLoan: 1, Inoperative: 0, InRepair: 0},
		LowStock: domain.LowStock{Count: 1, Top: []domain.LowStockItem{{ID: "i1", Code: "ISK-CON-001", Description: "Tape", Unit: "roll", StockCurrent: 1, StockMin: 4, LocationName: &loc}}},
		Loans: domain.Loans{Active: 1, Overdue: 1, OverdueTop: []domain.OverdueLoan{
			{ItemID: "i2", Code: "ISK-HER-001", Description: "Drill", AssigneeName: "Juan", DueAt: clock.Add(-time.Hour)}}},
	})
	status, body := get(t, srv, "viewer")
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	if body["generated_at"] != clock.Format(time.RFC3339Nano) && body["generated_at"] != "2026-06-01T12:00:00Z" {
		t.Errorf("generated_at = %v, want the service clock", body["generated_at"])
	}
	section := func(name string) map[string]any {
		m, ok := body[name].(map[string]any)
		if !ok {
			t.Fatalf("missing object %q in %v", name, body)
		}
		return m
	}
	if got := section("items")["total"]; got != float64(3) {
		t.Errorf("items.total = %v", got)
	}
	for _, k := range []string{"available", "unavailable", "on_loan", "inoperative", "in_repair"} {
		if _, ok := section("assets")[k]; !ok {
			t.Errorf("assets.%s missing", k)
		}
	}
	low := section("low_stock")
	top := low["top"].([]any)
	if low["count"] != float64(1) || len(top) != 1 {
		t.Fatalf("low_stock = %v", low)
	}
	first := top[0].(map[string]any)
	for _, k := range []string{"id", "code", "description", "unit", "stock_current", "stock_min", "location_name", "stocks"} {
		if _, ok := first[k]; !ok {
			t.Errorf("low_stock.top[0].%s missing", k)
		}
	}
	loans := section("loans")
	if o := loans["overdue_top"].([]any)[0].(map[string]any); o["assignee_name"] != "Juan" || o["due_at"] == nil || o["item_id"] != "i2" {
		t.Errorf("loans.overdue_top[0] = %v", o)
	}
	maint := section("maintenance")
	if top, ok := maint["overdue_top"].([]any); !ok || len(top) != 0 {
		t.Errorf("maintenance.overdue_top must be an empty array, got %v", maint["overdue_top"])
	}
	if _, ok := maint["open_failures"]; !ok {
		t.Error("maintenance.open_failures missing")
	}
}

func TestSummaryPassesAppClockToRepository(t *testing.T) {
	srv, repo := newServer(t)
	get(t, srv, "viewer")
	if !repo.LastNow().Equal(clock) {
		t.Errorf("repository asked for %v, want %v", repo.LastNow(), clock)
	}
}

func TestSummaryRepositoryFailureIs500(t *testing.T) {
	srv, repo := newServer(t)
	repo.FailWith(errors.New("db down"))
	if status, _ := get(t, srv, "viewer"); status != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", status)
	}
}

const reorderPath = "/api/v1/dashboard/reorder-suggestions"

func TestReorderSuggestionsAuth(t *testing.T) {
	srv, _ := newServer(t)
	for _, token := range []string{"", "garbage"} {
		if status, _ := getPath(t, srv, reorderPath, token); status != http.StatusUnauthorized {
			t.Errorf("token %q: got %d, want 401", token, status)
		}
	}
	for _, role := range []string{"viewer", "operator", "admin"} {
		if status, body := getPath(t, srv, reorderPath, role); status != http.StatusOK {
			t.Errorf("%s: got %d %v, want 200", role, status, body)
		}
	}
}

func TestReorderSuggestionsEmptyIsAnEmptyArray(t *testing.T) {
	srv, _ := newServer(t)
	_, body := getPath(t, srv, reorderPath, "viewer")
	if g, ok := body["groups"].([]any); !ok || len(g) != 0 {
		t.Fatalf("groups = %#v, want []", body["groups"])
	}
}

func TestReorderSuggestionsShape(t *testing.T) {
	srv, repo := newServer(t)
	sid, name, cost, cur := int64(7), "Alfa", int64(250), "USD"
	repo.SetCandidates([]domain.ReorderCandidate{
		{ItemID: "i1", Code: "ISK-CON-001", Description: "Tape", Unit: "roll", StockCurrent: 1, StockMin: 5, PendingOrdered: 2,
			SupplierID: &sid, SupplierName: &name, LastUnitCostMinor: &cost, LastCurrency: &cur},
		{ItemID: "i2", Code: "ISK-CON-002", Description: "Glue", StockCurrent: 0, StockMin: 3},
		{ItemID: "i3", Code: "ISK-CON-003", StockCurrent: 1, StockMin: 5, PendingOrdered: 9}, // covered
	})
	status, body := getPath(t, srv, reorderPath, "viewer")
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	groups := body["groups"].([]any)
	if len(groups) != 2 {
		t.Fatalf("groups = %v", groups)
	}
	g0 := groups[0].(map[string]any)
	l0 := g0["lines"].([]any)[0].(map[string]any)
	if g0["supplier_id"] != float64(7) || g0["supplier_name"] != "Alfa" || l0["suggested_qty"] != float64(7) ||
		l0["pending_ordered"] != float64(2) || l0["last_unit_cost_minor"] != float64(250) || l0["last_currency"] != "USD" {
		t.Errorf("first group = %v", g0)
	}
	g1 := groups[1].(map[string]any)
	l1 := g1["lines"].([]any)[0].(map[string]any)
	if g1["supplier_id"] != nil || g1["supplier_name"] != nil || l1["suggested_qty"] != float64(6) ||
		l1["last_unit_cost_minor"] != nil || l1["last_currency"] != nil {
		t.Errorf("unassigned group = %v", g1)
	}
}

func TestReorderSuggestionsRepositoryFailureIs500(t *testing.T) {
	srv, repo := newServer(t)
	repo.FailWith(errors.New("db down"))
	if status, _ := getPath(t, srv, reorderPath, "viewer"); status != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", status)
	}
}

func TestSummaryLowStockCarriesPendingOrdered(t *testing.T) {
	srv, repo := newServer(t)
	repo.Set(domain.Summary{LowStock: domain.LowStock{Count: 1, Top: []domain.LowStockItem{{ID: "i1", PendingOrdered: 4}}}})
	_, body := get(t, srv, "viewer")
	first := body["low_stock"].(map[string]any)["top"].([]any)[0].(map[string]any)
	if first["pending_ordered"] != float64(4) {
		t.Errorf("pending_ordered = %v", first["pending_ordered"])
	}
}
