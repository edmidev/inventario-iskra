package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/custody/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/custody/adapters/memory"
	"github.com/edgar/inventario/backend/internal/custody/app"
	"github.com/edgar/inventario/backend/internal/custody/domain"
	identity "github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

type env struct {
	srv *httptest.Server
	st  *memory.Store
}

func p64(v int64) *int64 { return &v }

func newEnv(t *testing.T) env {
	t.Helper()
	st := memory.NewStore()
	st.SeedPlace(1, "Bodega", domain.PlaceStorage)
	st.SeedPlace(2, "Alejandro", domain.PlacePerson)
	st.SeedPlace(3, "Maleta de Alejandro", domain.PlaceContainer)
	st.SeedUser("u-operator", "Olga Operator")
	st.SeedUser("u-admin", "Adam Admin")
	st.SeedItem(memory.Item{ID: "a1", Code: "ISK-HER-001", Description: "Drill", Kind: domain.KindAsset, LocationID: p64(1)})
	st.SeedItem(memory.Item{ID: "a2", Code: "ISK-HER-002", Description: "Saw", Kind: domain.KindAsset, LocationID: p64(1)})
	st.SeedItem(memory.Item{ID: "c1", Code: "ISK-CON-001", Description: "Gloves", Kind: domain.KindConsumable})
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
	h := httpapi.New(app.NewService(st, nil), httpx.Authenticate(verify), httpapi.Permissions{
		Read: string(identity.PermRead), WriteItems: string(identity.PermWriteItems)})
	r := chi.NewRouter()
	r.Route("/api/v1", h.Register)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return env{srv: srv, st: st}
}

type resp struct {
	Status int
	Body   map[string]any
}

func (e env) do(t *testing.T, method, path, token string, body any) resp {
	t.Helper()
	var rd *bytes.Reader
	if s, ok := body.(string); ok {
		rd = bytes.NewReader([]byte(s))
	} else {
		b, _ := json.Marshal(body)
		if body == nil {
			b = nil
		}
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+"/api/v1"+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := resp{Status: res.StatusCode, Body: map[string]any{}}
	_ = json.NewDecoder(res.Body).Decode(&out.Body)
	return out
}

func (r resp) errCode() string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		c, _ := e["code"].(string)
		return c
	}
	return ""
}

func (r resp) field(name string) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		if f, ok := e["fields"].(map[string]any); ok {
			s, _ := f[name].(string)
			return s
		}
	}
	return ""
}

func (r resp) data() []any { d, _ := r.Body["data"].([]any); return d }

type m = map[string]any

func TestAuthAndPermissionMatrix(t *testing.T) {
	e := newEnv(t)
	assign := m{"assignee_location_id": 2}
	tests := []struct {
		name, method, path, token string
		body                      any
		want                      int
	}{
		{"assign anonymous", "POST", "/items/a1/assign", "", assign, 401},
		{"return anonymous", "POST", "/items/a1/return", "", nil, 401},
		{"history anonymous", "GET", "/items/a1/custody", "", nil, 401},
		{"active anonymous", "GET", "/custody/active", "", nil, 401},
		{"viewer cannot assign", "POST", "/items/a1/assign", "viewer", assign, 403},
		{"viewer cannot return", "POST", "/items/a1/return", "viewer", nil, 403},
		{"operator assigns", "POST", "/items/a1/assign", "operator", assign, 201},
		{"viewer reads history", "GET", "/items/a1/custody", "viewer", nil, 200},
		{"viewer reads active", "GET", "/custody/active", "viewer", nil, 200},
		{"operator returns", "POST", "/items/a1/return", "operator", nil, 200},
		{"admin assigns", "POST", "/items/a2/assign", "admin", assign, 201},
	}
	for _, tc := range tests {
		if got := e.do(t, tc.method, tc.path, tc.token, tc.body); got.Status != tc.want {
			t.Errorf("%s: status %d, want %d (%v)", tc.name, got.Status, tc.want, got.Body)
		}
	}
	if n := len(e.st.Rows()); n != 2 {
		t.Errorf("forbidden/anonymous requests wrote rows: %d", n)
	}
}

func TestAssignReturnsTheAssignmentAndMovesTheItem(t *testing.T) {
	e := newEnv(t)
	due := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	got := e.do(t, "POST", "/items/a1/assign", "operator", m{"assignee_location_id": 3, "due_at": due, "note": "field job"})
	if got.Status != 201 {
		t.Fatalf("status %d: %v", got.Status, got.Body)
	}
	b := got.Body
	if b["item_code"] != "ISK-HER-001" || b["assignee_name"] != "Maleta de Alejandro" || b["assignee_kind"] != "container" ||
		b["origin_name"] != "Bodega" || b["assigned_by"] != "u-operator" || b["assigned_by_name"] != "Olga Operator" ||
		b["assign_note"] != "field job" || b["returned_at"] != nil || b["overdue"] != false || b["due_at"] == nil {
		t.Errorf("body: %v", b)
	}
	if loc := e.st.ItemLocation("a1"); loc == nil || *loc != 3 {
		t.Errorf("item location = %v", loc)
	}

	ret := e.do(t, "POST", "/items/a1/return", "admin", m{"note": "back"})
	if ret.Status != 200 || ret.Body["returned_at"] == nil || ret.Body["returned_by_name"] != "Adam Admin" ||
		ret.Body["returned_to_name"] != "Bodega" || ret.Body["return_note"] != "back" {
		t.Errorf("return: %d %v", ret.Status, ret.Body)
	}
	if loc := e.st.ItemLocation("a1"); loc == nil || *loc != 1 {
		t.Errorf("restored location = %v", loc)
	}
}

func TestAssignValidationAndConflicts(t *testing.T) {
	e := newEnv(t)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	for name, tc := range map[string]struct {
		path  string
		body  any
		want  int
		code  string
		field string
	}{
		"missing assignee":   {"/items/a1/assign", m{}, 422, "validation_failed", "assignee_location_id"},
		"unknown assignee":   {"/items/a1/assign", m{"assignee_location_id": 99}, 422, "validation_failed", "assignee_location_id"},
		"storage assignee":   {"/items/a1/assign", m{"assignee_location_id": 1}, 422, "validation_failed", "assignee_location_id"},
		"due in the past":    {"/items/a1/assign", m{"assignee_location_id": 2, "due_at": past}, 422, "validation_failed", "due_at"},
		"consumable":         {"/items/c1/assign", m{"assignee_location_id": 2}, 422, "not_applicable", ""},
		"unknown item":       {"/items/nope/assign", m{"assignee_location_id": 2}, 404, "not_found", ""},
		"malformed json":     {"/items/a1/assign", "{nope", 400, "invalid_request", ""},
		"unknown field":      {"/items/a1/assign", `{"assignee_location_id":2,"x":1}`, 400, "invalid_request", ""},
		"return not on loan": {"/items/a1/return", m{}, 409, "not_assigned", ""},
		"return unknown loc": {"/items/a1/return", m{"return_location_id": 99}, 422, "validation_failed", "return_location_id"},
		"return consumable":  {"/items/c1/return", nil, 422, "not_applicable", ""},
	} {
		got := e.do(t, "POST", tc.path, "operator", tc.body)
		if got.Status != tc.want || got.errCode() != tc.code || (tc.field != "" && got.field(tc.field) == "") {
			t.Errorf("%s: %d %v", name, got.Status, got.Body)
		}
	}
	if n := len(e.st.Rows()); n != 0 {
		t.Errorf("rejected requests wrote %d rows", n)
	}

	e.do(t, "POST", "/items/a1/assign", "operator", m{"assignee_location_id": 2})
	if got := e.do(t, "POST", "/items/a1/assign", "operator", m{"assignee_location_id": 3}); got.Status != 409 || got.errCode() != "already_assigned" {
		t.Errorf("second assign: %d %v", got.Status, got.Body)
	}
	e.st.SetStatus("a2", "inoperative")
	if got := e.do(t, "POST", "/items/a2/assign", "operator", m{"assignee_location_id": 2}); got.Status != 409 || got.errCode() != "item_not_available" {
		t.Errorf("inoperative: %d %v", got.Status, got.Body)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	e := newEnv(t)
	e.do(t, "POST", "/items/a1/assign", "operator", m{"assignee_location_id": 2})
	e.do(t, "POST", "/items/a1/return", "operator", nil)
	e.do(t, "POST", "/items/a1/assign", "operator", m{"assignee_location_id": 3})

	got := e.do(t, "GET", "/items/a1/custody", "viewer", nil)
	if got.Status != 200 || got.Body["total"] != 2.0 || got.Body["limit"] != 25.0 || got.Body["offset"] != 0.0 || len(got.data()) != 2 {
		t.Fatalf("%d %v", got.Status, got.Body)
	}
	first, second := got.data()[0].(map[string]any), got.data()[1].(map[string]any)
	if first["assignee_name"] != "Maleta de Alejandro" || first["returned_at"] != nil || second["assignee_name"] != "Alejandro" || second["returned_at"] == nil {
		t.Errorf("order/content: %v", got.data())
	}
	if paged := e.do(t, "GET", "/items/a1/custody?limit=1&offset=1", "viewer", nil); len(paged.data()) != 1 || paged.Body["total"] != 2.0 {
		t.Errorf("paging: %v", paged.Body)
	}
	if r := e.do(t, "GET", "/items/nope/custody", "viewer", nil); r.Status != 404 {
		t.Errorf("unknown item: %d", r.Status)
	}
	if r := e.do(t, "GET", "/items/a1/custody?limit=abc", "viewer", nil); r.Status != 422 || r.field("limit") == "" {
		t.Errorf("bad limit: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "GET", "/items/a1/custody?limit=1000", "viewer", nil); r.Status != 422 {
		t.Errorf("limit too big: %d", r.Status)
	}
}

func TestActiveEndpointFiltersAndOrder(t *testing.T) {
	e := newEnv(t)
	e.do(t, "POST", "/items/a2/assign", "operator", m{"assignee_location_id": 3})
	e.do(t, "POST", "/items/a1/assign", "operator", m{"assignee_location_id": 2})

	got := e.do(t, "GET", "/custody/active", "viewer", nil)
	if got.Status != 200 || got.Body["total"] != 2.0 || got.data()[0].(map[string]any)["item_code"] != "ISK-HER-002" {
		t.Fatalf("oldest first: %d %v", got.Status, got.Body)
	}
	if r := e.do(t, "GET", "/custody/active?assignee_location_id=2", "viewer", nil); len(r.data()) != 1 || r.data()[0].(map[string]any)["item_code"] != "ISK-HER-001" {
		t.Errorf("assignee filter: %v", r.Body)
	}
	if r := e.do(t, "GET", "/custody/active?q=saw", "viewer", nil); len(r.data()) != 1 {
		t.Errorf("q filter: %v", r.Body)
	}
	if r := e.do(t, "GET", "/custody/active?overdue=true", "viewer", nil); r.Status != 200 || len(r.data()) != 0 {
		t.Errorf("overdue filter: %v", r.Body)
	}
	for _, q := range []string{"overdue=maybe", "assignee_location_id=x", "offset=-1"} {
		if r := e.do(t, "GET", "/custody/active?"+q, "viewer", nil); r.Status != 422 {
			t.Errorf("%s: %d", q, r.Status)
		}
	}
	e.do(t, "POST", "/items/a2/return", "operator", nil)
	if r := e.do(t, "GET", "/custody/active", "viewer", nil); r.Body["total"] != 1.0 {
		t.Errorf("returned loans leave the list: %v", r.Body)
	}
}
