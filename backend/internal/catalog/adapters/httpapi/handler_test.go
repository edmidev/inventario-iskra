package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/catalog/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/catalog/adapters/memory"
	"github.com/edgar/inventario/backend/internal/catalog/app"
	catalog "github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

type env struct {
	srv *httptest.Server
	st  *memory.Store
}

func newEnv(t *testing.T) env {
	t.Helper()
	st := memory.NewStore()
	svc := app.NewService(st.Categories(), st.Locations(), st.Items())
	verify := func(token string) (httpx.Principal, error) {
		role := domain.Role(token) // tokens are role names in these tests
		if !role.Valid() {
			return httpx.Principal{}, errors.New("bad token")
		}
		var perms []string
		for _, p := range role.Permissions() {
			perms = append(perms, string(p))
		}
		return httpx.Principal{UserID: "u-" + token, Role: token, Permissions: perms}, nil
	}
	h := httpapi.New(svc, httpx.Authenticate(verify), httpapi.Permissions{
		Read: string(domain.PermRead), WriteItems: string(domain.PermWriteItems), ManageCatalogs: string(domain.PermManageCatalogs),
	})
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

func (r resp) str(k string) string { s, _ := r.Body[k].(string); return s }
func (r resp) num(k string) int    { f, _ := r.Body[k].(float64); return int(f) }
func (r resp) data() []any         { d, _ := r.Body["data"].([]any); return d }

func (e env) mustCreate(t *testing.T, path string, body any) resp {
	t.Helper()
	r := e.do(t, http.MethodPost, path, "admin", body)
	if r.Status != http.StatusCreated {
		t.Fatalf("POST %s: %d %v", path, r.Status, r.Body)
	}
	return r
}

func TestAuthAndPermissionMatrix(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	catID := cat.num("id")
	item := e.mustCreate(t, "/items", map[string]any{"description": "Taladro", "category_id": catID, "kind": "asset"})
	itemPath := "/items/" + item.str("id")

	type c struct {
		method, path string
		body         any
		token        string
		want         int
	}
	itemBody := map[string]any{"description": "Otro", "category_id": catID, "kind": "asset"}
	cases := []c{
		{"GET", "/categories", nil, "", 401},
		{"GET", "/items", nil, "", 401},
		{"GET", "/items", nil, "garbage", 401},
		{"GET", "/categories", nil, "viewer", 200},
		{"GET", "/locations", nil, "viewer", 200},
		{"GET", "/items", nil, "viewer", 200},
		{"GET", itemPath, nil, "viewer", 200},
		{"GET", "/items/by-code/ISK-HER-001", nil, "viewer", 200},
		{"POST", "/items", itemBody, "viewer", 403},
		{"PATCH", itemPath, map[string]any{"notes": "x"}, "viewer", 403},
		{"DELETE", itemPath, nil, "viewer", 403},
		{"POST", "/categories", map[string]any{"name": "N", "code_prefix": "NN"}, "viewer", 403},
		{"POST", "/categories", map[string]any{"name": "N", "code_prefix": "NN"}, "operator", 403},
		{"PATCH", "/categories/1", map[string]any{"name": "N"}, "operator", 403},
		{"DELETE", "/categories/1", nil, "operator", 403},
		{"POST", "/locations", map[string]any{"name": "L", "kind": "storage"}, "operator", 403},
		{"PATCH", "/locations/1", map[string]any{"name": "L"}, "operator", 403},
		{"DELETE", "/locations/1", nil, "operator", 403},
		{"POST", "/items", itemBody, "operator", 201},
		{"PATCH", itemPath, map[string]any{"notes": "x"}, "operator", 200},
		{"POST", "/categories", map[string]any{"name": "Nueva", "code_prefix": "NUE"}, "admin", 201},
		{"POST", "/locations", map[string]any{"name": "L", "kind": "storage"}, "admin", 201},
		{"DELETE", itemPath, nil, "operator", 204},
	}
	for _, tc := range cases {
		if got := e.do(t, tc.method, tc.path, tc.token, tc.body); got.Status != tc.want {
			t.Errorf("%s %s as %q: got %d want %d (%v)", tc.method, tc.path, tc.token, got.Status, tc.want, got.Body)
		}
	}
}

func TestOperatorCannotSetItemCode(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	body := map[string]any{"code": "ISK-OLD-1", "description": "x", "category_id": cat.num("id"), "kind": "asset"}
	if r := e.do(t, "POST", "/items", "operator", body); r.Status != 403 {
		t.Fatalf("operator with code: %d", r.Status)
	}
	r := e.do(t, "POST", "/items", "admin", body)
	if r.Status != 201 || r.str("code") != "ISK-OLD-1" {
		t.Fatalf("admin with code: %d %v", r.Status, r.Body)
	}
	if dup := e.do(t, "POST", "/items", "admin", body); dup.Status != 409 || dup.errCode() != "code_taken" {
		t.Fatalf("duplicate: %d %v", dup.Status, dup.Body)
	}
}

func TestCategoryEndpoints(t *testing.T) {
	e := newEnv(t)
	c := e.mustCreate(t, "/categories", map[string]any{"name": " Herramientas ", "code_prefix": "her"})
	if c.str("name") != "Herramientas" || c.str("code_prefix") != "HER" {
		t.Fatalf("created: %v", c.Body)
	}
	if r := e.do(t, "POST", "/categories", "admin", map[string]any{"name": "herramientas", "code_prefix": "XX"}); r.Status != 409 || r.errCode() != "category_name_taken" {
		t.Errorf("dup name: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "POST", "/categories", "admin", map[string]any{"name": "Otra", "code_prefix": "HER"}); r.Status != 409 || r.errCode() != "category_prefix_taken" {
		t.Errorf("dup prefix: %d %v", r.Status, r.Body)
	}
	r := e.do(t, "POST", "/categories", "admin", map[string]any{"name": "", "code_prefix": "a"})
	if r.Status != 422 || r.errCode() != "validation_failed" || r.field("name") == "" || r.field("code_prefix") == "" {
		t.Errorf("validation: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "POST", "/categories", "admin", "{not json"); r.Status != 400 || r.errCode() != "invalid_request" {
		t.Errorf("malformed: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "POST", "/categories", "admin", `{"name":"A","code_prefix":"AB","bogus":1}`); r.Status != 400 {
		t.Errorf("unknown field: %d", r.Status)
	}
	id := c.num("id")
	p := "/categories/" + itoa(id)
	if r := e.do(t, "PATCH", p, "admin", map[string]any{"name": "Equipos"}); r.Status != 200 || r.str("name") != "Equipos" {
		t.Errorf("patch: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "PATCH", "/categories/999", "admin", map[string]any{"name": "x"}); r.Status != 404 {
		t.Errorf("patch missing: %d", r.Status)
	}
	if r := e.do(t, "PATCH", "/categories/abc", "admin", map[string]any{"name": "x"}); r.Status != 404 {
		t.Errorf("patch bad id: %d", r.Status)
	}
	e.mustCreate(t, "/items", map[string]any{"description": "Taladro", "category_id": id, "kind": "asset"})
	if r := e.do(t, "DELETE", p, "admin", nil); r.Status != 409 || r.errCode() != "category_in_use" {
		t.Errorf("delete in use: %d %v", r.Status, r.Body)
	}
	other := e.mustCreate(t, "/categories", map[string]any{"name": "Vacía", "code_prefix": "VAC"})
	if r := e.do(t, "DELETE", "/categories/"+itoa(other.num("id")), "admin", nil); r.Status != 204 {
		t.Errorf("delete empty: %d", r.Status)
	}
	if r := e.do(t, "GET", "/categories", "viewer", nil); len(r.data()) != 1 {
		t.Errorf("list: %v", r.Body)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestLocationEndpoints(t *testing.T) {
	e := newEnv(t)
	root := e.mustCreate(t, "/locations", map[string]any{"name": "Bodega", "kind": "storage"})
	kid := e.mustCreate(t, "/locations", map[string]any{"name": "Maleta de Alejandro", "kind": "container", "parent_id": root.num("id")})
	if kid.Body["parent_id"] == nil {
		t.Fatalf("parent missing: %v", kid.Body)
	}
	if r := e.do(t, "POST", "/locations", "admin", map[string]any{"name": "x", "kind": "garage"}); r.Status != 422 || r.field("kind") == "" {
		t.Errorf("bad kind: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "POST", "/locations", "admin", map[string]any{"name": "x", "kind": "storage", "parent_id": 999}); r.Status != 422 || r.field("parent_id") == "" {
		t.Errorf("bad parent: %d %v", r.Status, r.Body)
	}
	rootPath, kidPath := "/locations/"+itoa(root.num("id")), "/locations/"+itoa(kid.num("id"))
	if r := e.do(t, "PATCH", rootPath, "admin", map[string]any{"parent_id": kid.num("id")}); r.Status != 409 || r.errCode() != "location_cycle" {
		t.Errorf("cycle: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "DELETE", rootPath, "admin", nil); r.Status != 409 || r.errCode() != "location_in_use" {
		t.Errorf("delete with child: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "PATCH", kidPath, "admin", `{"parent_id":null,"name":"Maleta"}`); r.Status != 200 || r.Body["parent_id"] != nil || r.str("name") != "Maleta" {
		t.Errorf("detach: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "DELETE", kidPath, "admin", nil); r.Status != 204 {
		t.Errorf("delete: %d", r.Status)
	}
	if r := e.do(t, "DELETE", kidPath, "admin", nil); r.Status != 404 {
		t.Errorf("delete again: %d", r.Status)
	}
}

func TestItemCreateReadUpdateDelete(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	loc := e.mustCreate(t, "/locations", map[string]any{"name": "Bodega", "kind": "storage"})
	created := e.do(t, "POST", "/items", "operator", map[string]any{
		"description": "Taladro", "category_id": cat.num("id"), "kind": "asset", "location_id": loc.num("id"),
		"attributes": map[string]any{"marca": "Bosch", "color": "azul"}, "maintenance_interval_days": 90,
	})
	if created.Status != 201 {
		t.Fatalf("create: %d %v", created.Status, created.Body)
	}
	b := created.Body
	if b["code"] != "ISK-HER-001" || b["availability"] != "available" || b["status"] != "functional" ||
		b["stock_current"] != float64(1) || b["unit"] != nil || b["low_stock"] != false {
		t.Fatalf("body: %v", b)
	}
	if attrs, _ := b["attributes"].(map[string]any); attrs["marca"] != "Bosch" {
		t.Fatalf("attributes: %v", b["attributes"])
	}
	id := created.str("id")

	if r := e.do(t, "GET", "/items/"+id, "viewer", nil); r.Status != 200 || r.str("code") != "ISK-HER-001" {
		t.Errorf("get: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "GET", "/items/by-code/isk-her-001", "viewer", nil); r.Status != 200 || r.str("id") != id {
		t.Errorf("by code (case-insensitive): %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "GET", "/items/by-code/ISK-NOPE-1", "viewer", nil); r.Status != 404 || r.errCode() != "not_found" {
		t.Errorf("by code missing: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "GET", "/items/not-a-uuid", "viewer", nil); r.Status != 404 {
		t.Errorf("bad id: %d", r.Status)
	}

	r := e.do(t, "PATCH", "/items/"+id, "operator", `{"status":"in_repair","location_id":null,"notes":"falla","attributes":{"marca":"Makita"}}`)
	if r.Status != 200 || r.str("status") != "in_repair" || r.str("availability") != "unavailable" || r.Body["location_id"] != nil || r.str("notes") != "falla" {
		t.Errorf("patch: %d %v", r.Status, r.Body)
	}
	if attrs, _ := r.Body["attributes"].(map[string]any); attrs["marca"] != "Makita" || attrs["color"] != nil {
		t.Errorf("attributes replaced: %v", r.Body["attributes"])
	}
	for field, body := range map[string]string{
		"kind":          `{"kind":"consumable"}`,
		"code":          `{"code":"ISK-X-1"}`,
		"stock_current": `{"stock_current":5}`,
		"status":        `{"status":"broken"}`,
	} {
		if r := e.do(t, "PATCH", "/items/"+id, "operator", body); r.Status != 422 || r.field(field) == "" {
			t.Errorf("patch %s: %d %v", field, r.Status, r.Body)
		}
	}
	if r := e.do(t, "PATCH", "/items/00000000-0000-4000-8000-0000000000ff", "operator", `{}`); r.Status != 404 {
		t.Errorf("patch missing: %d", r.Status)
	}

	if r := e.do(t, "DELETE", "/items/"+id, "operator", nil); r.Status != 204 {
		t.Errorf("delete: %d", r.Status)
	}
	if r := e.do(t, "GET", "/items/"+id, "viewer", nil); r.Status != 404 {
		t.Errorf("get deleted: %d", r.Status)
	}
	if r := e.do(t, "GET", "/items", "viewer", nil); r.num("total") != 0 {
		t.Errorf("list deleted: %v", r.Body)
	}
}

func TestItemCreateValidation(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	r := e.do(t, "POST", "/items", "operator", map[string]any{"description": "", "category_id": 999, "kind": "consumable"})
	if r.Status != 422 || r.field("description") == "" || r.field("unit") == "" {
		t.Errorf("fields: %d %v", r.Status, r.Body)
	}
	r = e.do(t, "POST", "/items", "operator", map[string]any{"description": "x", "category_id": 999, "kind": "asset"})
	if r.Status != 422 || r.field("category_id") == "" {
		t.Errorf("category: %d %v", r.Status, r.Body)
	}
	r = e.do(t, "POST", "/items", "operator", map[string]any{"description": "x", "category_id": cat.num("id"), "kind": "asset", "location_id": 42})
	if r.Status != 422 || r.field("location_id") == "" {
		t.Errorf("location: %d %v", r.Status, r.Body)
	}
	r = e.do(t, "POST", "/items", "operator", map[string]any{"description": "Cinta", "category_id": cat.num("id"), "kind": "consumable", "unit": "rollo", "stock_current": 12, "stock_min": 4})
	if r.Status != 201 || r.str("unit") != "rollo" || r.num("stock_current") != 12 || r.Body["low_stock"] != false {
		t.Errorf("consumable: %d %v", r.Status, r.Body)
	}
}

func TestItemListFiltersSortPagination(t *testing.T) {
	e := newEnv(t)
	her := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"}).num("id")
	cab := e.mustCreate(t, "/categories", map[string]any{"name": "Cables", "code_prefix": "CAB"}).num("id")
	bodega := e.mustCreate(t, "/locations", map[string]any{"name": "Bodega", "kind": "storage"}).num("id")
	mk := func(body map[string]any) { e.mustCreate(t, "/items", body) }
	mk(map[string]any{"description": "Taladro", "category_id": her, "kind": "asset", "location_id": bodega})
	mk(map[string]any{"description": "Martillo", "category_id": her, "kind": "asset", "status": "inoperative"})
	mk(map[string]any{"description": "Cable HDMI", "category_id": cab, "kind": "consumable", "unit": "unidad", "stock_current": 1, "stock_min": 2})
	mk(map[string]any{"description": "Cinta aislante", "category_id": cab, "kind": "consumable", "unit": "rollo", "stock_current": 30, "stock_min": 2})
	mk(map[string]any{"description": "Rollo 100%_x", "category_id": cab, "kind": "consumable", "unit": "rollo", "stock_current": 3})

	total := func(q string) int {
		r := e.do(t, "GET", "/items?"+q, "viewer", nil)
		if r.Status != 200 {
			t.Fatalf("%s: %d %v", q, r.Status, r.Body)
		}
		return r.num("total")
	}
	cases := map[string]int{
		"": 5, "q=cable": 1, "q=ISK-HER": 2, "q=100%25_x": 1, "q=%25": 1, "category_id=" + itoa(cab): 3,
		"status=inoperative": 1, "kind=consumable": 3, "kind=asset": 2, "availability=available": 4,
		"availability=unavailable": 1, "location_id=" + itoa(bodega): 1, "low_stock=true": 1,
		"kind=asset&availability=available": 1, "q=zzz": 0,
	}
	for q, want := range cases {
		if got := total(q); got != want {
			t.Errorf("?%s: total %d want %d", q, got, want)
		}
	}

	r := e.do(t, "GET", "/items?sort=description&order=desc&limit=2&offset=1", "viewer", nil)
	data := r.data()
	if r.num("total") != 5 || r.num("limit") != 2 || r.num("offset") != 1 || len(data) != 2 {
		t.Fatalf("page: %v", r.Body)
	}
	first := data[0].(map[string]any)["description"]
	second := data[1].(map[string]any)["description"]
	if first != "Taladro" && first != "Rollo 100%_x" || second == nil {
		t.Errorf("sorted page: %v %v", first, second)
	}
	r = e.do(t, "GET", "/items?sort=-code&limit=1", "viewer", nil)
	if r.data()[0].(map[string]any)["code"] != "ISK-HER-002" {
		t.Errorf("sort -code: %v", r.data())
	}
	if r := e.do(t, "GET", "/items", "viewer", nil); r.num("limit") != 25 {
		t.Errorf("default limit: %v", r.Body)
	}

	for _, q := range []string{"limit=101", "limit=abc", "offset=-1", "sort=color", "status=x", "availability=maybe", "category_id=x", "low_stock=1", "order=sideways"} {
		if r := e.do(t, "GET", "/items?"+q, "viewer", nil); r.Status != 422 || r.errCode() != "validation_failed" {
			t.Errorf("?%s: %d %v", q, r.Status, r.Body)
		}
	}
	if body := e.do(t, "GET", "/items?q=zzz", "viewer", nil); body.Body["data"] == nil {
		t.Errorf("empty list must be [] not null: %v", body.Body)
	}
}

func TestItemsOnLoanExposeCustodyAndAvailability(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	mk := func(desc string) string {
		return e.mustCreate(t, "/items", map[string]any{"description": desc, "category_id": cat.num("id"), "kind": "asset"}).str("id")
	}
	free, lent, late := mk("Free"), mk("Lent"), mk("Late")
	due, lapsed := time.Now().Add(48*time.Hour), time.Now().Add(-48*time.Hour)
	e.st.SetLoan(lent, &catalog.LoanInfo{AssigneeLocationID: 7, AssigneeName: "Alejandro", AssignedAt: time.Now(), DueAt: &due})
	e.st.SetLoan(late, &catalog.LoanInfo{AssigneeLocationID: 7, AssigneeName: "Alejandro", AssignedAt: time.Now().Add(-72 * time.Hour), DueAt: &lapsed})

	r := e.do(t, "GET", "/items/"+lent, "viewer", nil)
	c, _ := r.Body["custody"].(map[string]any)
	if r.Body["on_loan"] != true || r.str("availability") != "unavailable" || r.str("status") != "functional" ||
		c["assignee_name"] != "Alejandro" || c["assignee_location_id"] != float64(7) || c["overdue"] != false || c["due_at"] == nil {
		t.Errorf("on loan: %v", r.Body)
	}
	r = e.do(t, "GET", "/items/"+late, "viewer", nil)
	if c, _ := r.Body["custody"].(map[string]any); c["overdue"] != true {
		t.Errorf("overdue: %v", r.Body)
	}
	r = e.do(t, "GET", "/items/"+free, "viewer", nil)
	if _, has := r.Body["custody"]; r.Body["on_loan"] != false || has || r.str("availability") != "available" {
		t.Errorf("free item: %v", r.Body)
	}

	ids := func(path string) map[string]bool {
		out := map[string]bool{}
		for _, d := range e.do(t, "GET", path, "viewer", nil).data() {
			out[d.(map[string]any)["id"].(string)] = true
		}
		return out
	}
	if got := ids("/items?availability=available"); len(got) != 1 || !got[free] {
		t.Errorf("available filter: %v", got)
	}
	if got := ids("/items?availability=unavailable"); len(got) != 2 || !got[lent] || !got[late] {
		t.Errorf("unavailable filter: %v", got)
	}
}

func TestItemOnLoanCannotBeDeletedOrMoved(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	home := e.mustCreate(t, "/locations", map[string]any{"name": "Bodega", "kind": "storage"})
	other := e.mustCreate(t, "/locations", map[string]any{"name": "Taller", "kind": "storage"})
	id := e.mustCreate(t, "/items", map[string]any{"description": "Drill", "category_id": cat.num("id"), "kind": "asset", "location_id": home.num("id")}).str("id")
	e.st.SetLoan(id, &catalog.LoanInfo{AssigneeLocationID: 7, AssigneeName: "Alejandro", AssignedAt: time.Now()})

	if r := e.do(t, "DELETE", "/items/"+id, "operator", nil); r.Status != 409 || r.errCode() != "item_on_loan" {
		t.Errorf("delete on loan: %d %v", r.Status, r.Body)
	}
	for name, body := range map[string]string{
		"move":  `{"location_id":` + itoa(other.num("id")) + `}`,
		"clear": `{"location_id":null}`,
	} {
		if r := e.do(t, "PATCH", "/items/"+id, "operator", body); r.Status != 409 || r.errCode() != "item_on_loan" {
			t.Errorf("%s on loan: %d %v", name, r.Status, r.Body)
		}
	}
	if r := e.do(t, "PATCH", "/items/"+id, "operator", `{"notes":"still editable","status":"in_repair"}`); r.Status != 200 ||
		r.Body["on_loan"] != true || r.str("availability") != "unavailable" {
		t.Errorf("other edits on loan: %d %v", r.Status, r.Body)
	}
	e.st.SetLoan(id, nil)
	if r := e.do(t, "DELETE", "/items/"+id, "operator", nil); r.Status != 204 {
		t.Errorf("delete after return: %d", r.Status)
	}
}

func TestItemsExposeMaintenanceDueDateAndOverdueFlag(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	mk := func(extra map[string]any) resp {
		body := map[string]any{"description": "Taladro", "category_id": cat.num("id"), "kind": "asset"}
		for k, v := range extra {
			body[k] = v
		}
		return e.mustCreate(t, "/items", body)
	}
	stale := time.Now().Add(-200 * 24 * time.Hour).UTC().Truncate(time.Second)
	overdue := mk(map[string]any{"maintenance_interval_days": 90, "last_maintenance_at": stale.Format(time.RFC3339)})
	fresh := mk(map[string]any{"maintenance_interval_days": 90})
	plain := mk(nil)

	if overdue.Body["maintenance_overdue"] != true || overdue.str("maintenance_due_at") != stale.Add(90*24*time.Hour).Format(time.RFC3339) {
		t.Errorf("overdue item: %v", overdue.Body)
	}
	if fresh.Body["maintenance_overdue"] != false || fresh.Body["maintenance_due_at"] == nil {
		t.Errorf("fresh item: %v", fresh.Body)
	}
	if v, has := plain.Body["maintenance_due_at"]; !has || v != nil || plain.Body["maintenance_overdue"] != false {
		t.Errorf("item without interval: %v", plain.Body)
	}
	// The same fields come back on read and list.
	if r := e.do(t, "GET", "/items/"+overdue.str("id"), "viewer", nil); r.Body["maintenance_overdue"] != true {
		t.Errorf("get: %v", r.Body)
	}
	for _, d := range e.do(t, "GET", "/items", "viewer", nil).data() {
		if _, has := d.(map[string]any)["maintenance_overdue"]; !has {
			t.Errorf("list item lacks maintenance_overdue: %v", d)
		}
	}
}

func TestPatchStatusWhileFailureOpenIs409(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Herramientas", "code_prefix": "HER"})
	id := e.mustCreate(t, "/items", map[string]any{"description": "Taladro", "category_id": cat.num("id"), "kind": "asset"}).str("id")
	e.st.SetOpenFailure(id, true)

	r := e.do(t, "PATCH", "/items/"+id, "operator", map[string]any{"status": "in_repair"})
	if r.Status != 409 || r.errCode() != "item_under_repair" {
		t.Fatalf("status change: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "PATCH", "/items/"+id, "operator", map[string]any{"status": "functional", "notes": "ok"}); r.Status != 200 {
		t.Errorf("same status: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "GET", "/items/"+id, "viewer", nil); r.str("status") != "functional" {
		t.Errorf("status changed despite 409: %v", r.Body)
	}
}

// stocksOf returns the stocks array of a response as location name -> quantity.
func stocksOf(t *testing.T, body map[string]any) map[string]int {
	t.Helper()
	list, ok := body["stocks"].([]any)
	if !ok {
		t.Fatalf("stocks is not an array: %v", body["stocks"])
	}
	out := map[string]int{}
	for _, s := range list {
		m := s.(map[string]any)
		out[m["location_name"].(string)] = int(m["quantity"].(float64))
	}
	return out
}

func TestConsumableStockedInSeveralLocations(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Consumibles", "code_prefix": "CON"}).num("id")
	taller := e.mustCreate(t, "/locations", map[string]any{"name": "Taller", "kind": "storage"}).num("id")
	maleta := e.mustCreate(t, "/locations", map[string]any{"name": "Maleta y caja de Alejandro", "kind": "container"}).num("id")

	created := e.do(t, "POST", "/items", "operator", map[string]any{
		"description": "Tornillos", "category_id": cat, "kind": "consumable", "unit": "u", "stock_min": 3,
		"stocks": []any{map[string]any{"location_id": taller, "quantity": 5}, map[string]any{"location_id": maleta, "quantity": 5}},
	})
	if created.Status != 201 || created.num("stock_current") != 10 || created.Body["location_id"] != nil {
		t.Fatalf("create: %d %v", created.Status, created.Body)
	}
	got := stocksOf(t, created.Body)
	if len(got) != 2 || got["Taller"] != 5 || got["Maleta y caja de Alejandro"] != 5 {
		t.Errorf("stocks: %v", got)
	}
	list := created.Body["stocks"].([]any)
	if list[0].(map[string]any)["location_name"] != "Maleta y caja de Alejandro" { // ordered by name
		t.Errorf("not ordered by location name: %v", list)
	}
	id := created.str("id")
	if r := e.do(t, "GET", "/items/"+id, "viewer", nil); r.Status != 200 || len(stocksOf(t, r.Body)) != 2 {
		t.Errorf("get: %d %v", r.Status, r.Body)
	}

	// Filtering by either location finds it; an unrelated one does not.
	for _, loc := range []int{taller, maleta} {
		if r := e.do(t, "GET", "/items?location_id="+itoa(loc), "viewer", nil); r.num("total") != 1 || r.data()[0].(map[string]any)["id"] != id {
			t.Errorf("location %d: %v", loc, r.Body)
		}
	}
	if r := e.do(t, "GET", "/items?location_id=999", "viewer", nil); r.num("total") != 0 {
		t.Errorf("unrelated location: %v", r.Body)
	}
	// low_stock stays on the total.
	if r := e.do(t, "GET", "/items?low_stock=true", "viewer", nil); r.num("total") != 0 {
		t.Errorf("low_stock on a total of 10: %v", r.Body)
	}

	// Moving a consumable through PATCH is refused; other edits keep the rows.
	r := e.do(t, "PATCH", "/items/"+id, "operator", map[string]any{"location_id": taller})
	if r.Status != 422 || r.errCode() != "not_applicable" {
		t.Errorf("patch location: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "PATCH", "/items/"+id, "operator", `{"location_id":null,"notes":"ok"}`); r.Status != 200 || r.str("notes") != "ok" || len(stocksOf(t, r.Body)) != 2 {
		t.Errorf("patch notes: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "DELETE", "/locations/"+itoa(taller), "admin", nil); r.Status != 409 || r.errCode() != "location_in_use" {
		t.Errorf("delete location holding stock: %d %v", r.Status, r.Body)
	}
}

func TestConsumableLegacyCreateKeepsWorking(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Consumibles", "code_prefix": "CON"}).num("id")
	bodega := e.mustCreate(t, "/locations", map[string]any{"name": "Bodega", "kind": "storage"}).num("id")
	r := e.do(t, "POST", "/items", "operator", map[string]any{"description": "Cinta", "category_id": cat, "kind": "consumable",
		"unit": "rollo", "stock_current": 8, "location_id": bodega})
	if r.Status != 201 || r.num("stock_current") != 8 || r.Body["location_id"] != nil || stocksOf(t, r.Body)["Bodega"] != 8 {
		t.Fatalf("legacy with location: %d %v", r.Status, r.Body)
	}
	r = e.do(t, "POST", "/items", "operator", map[string]any{"description": "Pegamento", "category_id": cat, "kind": "consumable",
		"unit": "u", "stock_current": 2})
	if r.Status != 201 || stocksOf(t, r.Body)["Sin ubicación"] != 2 {
		t.Fatalf("legacy without location: %d %v", r.Status, r.Body)
	}
	r = e.do(t, "POST", "/items", "operator", map[string]any{"description": "Vacío", "category_id": cat, "kind": "consumable", "unit": "u"})
	if r.Status != 201 || len(stocksOf(t, r.Body)) != 0 || r.num("stock_current") != 0 {
		t.Fatalf("no stock: %d %v", r.Status, r.Body)
	}
	// Assets expose an empty list and keep their single location.
	a := e.do(t, "POST", "/items", "operator", map[string]any{"description": "Taladro", "category_id": cat, "kind": "asset", "location_id": bodega})
	if a.Status != 201 || len(stocksOf(t, a.Body)) != 0 || a.num("location_id") != bodega {
		t.Errorf("asset: %d %v", a.Status, a.Body)
	}
	if r := e.do(t, "GET", "/items?location_id="+itoa(bodega), "viewer", nil); r.num("total") != 2 { // Cinta + Taladro
		t.Errorf("filter by Bodega: %v", r.Body)
	}
}

func TestCreateConsumableRejectsBothStockForms(t *testing.T) {
	e := newEnv(t)
	cat := e.mustCreate(t, "/categories", map[string]any{"name": "Consumibles", "code_prefix": "CON"}).num("id")
	bodega := e.mustCreate(t, "/locations", map[string]any{"name": "Bodega", "kind": "storage"}).num("id")
	row := []any{map[string]any{"location_id": bodega, "quantity": 1}}
	base := func(extra map[string]any) map[string]any {
		b := map[string]any{"description": "Cinta", "category_id": cat, "kind": "consumable", "unit": "u"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	for name, body := range map[string]map[string]any{
		"stocks + stock_current":     base(map[string]any{"stocks": row, "stock_current": 1}),
		"stocks + location_id":       base(map[string]any{"stocks": row, "location_id": bodega}),
		"unknown location in stocks": base(map[string]any{"stocks": []any{map[string]any{"location_id": 999, "quantity": 1}}}),
		"zero quantity":              base(map[string]any{"stocks": []any{map[string]any{"location_id": bodega, "quantity": 0}}}),
		"repeated location":          base(map[string]any{"stocks": []any{map[string]any{"location_id": bodega, "quantity": 1}, map[string]any{"location_id": bodega, "quantity": 2}}}),
		"stocks on an asset":         {"description": "Taladro", "category_id": cat, "kind": "asset", "stocks": row},
	} {
		r := e.do(t, "POST", "/items", "operator", body)
		if r.Status != 422 || r.errCode() != "validation_failed" {
			t.Errorf("%s: %d %v", name, r.Status, r.Body)
		}
	}
	if r := e.do(t, "POST", "/items", "viewer", base(map[string]any{"stocks": row})); r.Status != 403 {
		t.Errorf("viewer: %d", r.Status)
	}
	if r := e.do(t, "GET", "/items", "viewer", nil); r.num("total") != 0 {
		t.Errorf("rejected requests created items: %v", r.Body)
	}
}
