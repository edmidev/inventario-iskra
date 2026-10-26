package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/purchasing/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/purchasing/adapters/memory"
	"github.com/edgar/inventario/backend/internal/purchasing/app"
	pdomain "github.com/edgar/inventario/backend/internal/purchasing/domain"
	"github.com/go-chi/chi/v5"
)

type catalog struct {
	created int
	failNth int
}

func (c *catalog) ValidateNewItem(_ context.Context, s app.NewItemSpec) error {
	if s.CategoryID <= 0 {
		return pdomain.FieldError("category_id", "category does not exist")
	}
	return nil
}

func (c *catalog) CreateItem(context.Context, app.NewItemSpec, int64) (app.CreatedItem, error) {
	c.created++
	if c.created == c.failNth {
		return app.CreatedItem{}, errors.New("boom")
	}
	return app.CreatedItem{ID: fmt.Sprintf("id-%d", c.created), Code: fmt.Sprintf("ISK-T-%03d", c.created)}, nil
}

func (c *catalog) ItemInfo(_ context.Context, id string) (app.ItemInfo, error) {
	switch id {
	case "cons":
		return app.ItemInfo{Kind: "consumable", Description: "Papel"}, nil
	case "asset":
		return app.ItemInfo{Kind: "asset", Description: "Proyector"}, nil
	}
	return app.ItemInfo{}, app.ErrItemNotFound
}

func (c *catalog) LocationExists(_ context.Context, id int64) (bool, error) { return id == 1, nil }

type stock struct{ ins int }

func (s *stock) ReceiveIn(context.Context, string, string, int64, int, string) error {
	s.ins++
	return nil
}

type suppliers struct{}

func (suppliers) Get(_ context.Context, id int64) (string, bool, error) {
	switch id {
	case 1:
		return "Sur", true, nil
	case 2:
		return "Norte", false, nil
	}
	return "", false, app.ErrSupplierNotFound
}

type env struct {
	srv   *httptest.Server
	stock *stock
}

func newEnv(t *testing.T) env {
	t.Helper()
	store := memory.NewStore()
	store.SetSupplierName(1, "Sur")
	st := &stock{}
	svc := app.NewService(store, &catalog{}, st, suppliers{}, nil)
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
		Read: string(domain.PermRead), Write: string(domain.PermWriteItems),
	})
	r := chi.NewRouter()
	r.Route("/api/v1", h.Register)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return env{srv: srv, stock: st}
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

func code(r resp) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

func field(r resp, name string) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		if f, ok := e["fields"].(map[string]any); ok {
			s, _ := f[name].(string)
			return s
		}
	}
	return ""
}

func orderBody(lines ...map[string]any) map[string]any {
	ls := make([]any, len(lines))
	for i, l := range lines {
		ls[i] = l
	}
	return map[string]any{"supplier_id": 1, "currency": "USD", "lines": ls}
}

var (
	paper  = map[string]any{"item_id": "cons", "quantity": 10, "unit_cost_minor": 450}
	laptop = map[string]any{"new_item": map[string]any{"description": "Laptop", "category_id": 1, "kind": "asset"}, "quantity": 3, "unit_cost_minor": 90000}
)

func (e env) create(t *testing.T, lines ...map[string]any) int {
	t.Helper()
	r := e.do(t, "POST", "/purchase-orders", "operator", orderBody(lines...))
	if r.Status != http.StatusCreated {
		t.Fatalf("create: %d %v", r.Status, r.Body)
	}
	return int(r.Body["id"].(float64))
}

func path(id int, suffix string) string { return fmt.Sprintf("/purchase-orders/%d%s", id, suffix) }

func TestPermissions(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, paper)
	writes := []struct{ method, path string }{
		{"POST", "/purchase-orders"}, {"PATCH", path(id, "")}, {"POST", path(id, "/order")},
		{"POST", path(id, "/cancel")}, {"POST", path(id, "/receive")},
	}
	for _, w := range writes {
		if r := e.do(t, w.method, w.path, "", `{}`); r.Status != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s: %d", w.method, w.path, r.Status)
		}
		if r := e.do(t, w.method, w.path, "viewer", `{}`); r.Status != http.StatusForbidden {
			t.Fatalf("viewer %s %s: %d", w.method, w.path, r.Status)
		}
	}
	for _, p := range []string{"/purchase-orders", path(id, "")} {
		if r := e.do(t, "GET", p, "", nil); r.Status != http.StatusUnauthorized {
			t.Fatalf("anonymous GET %s: %d", p, r.Status)
		}
		if r := e.do(t, "GET", p, "viewer", nil); r.Status != http.StatusOK {
			t.Fatalf("viewer GET %s: %d", p, r.Status)
		}
	}
	if r := e.do(t, "POST", "/purchase-orders", "admin", orderBody(paper)); r.Status != http.StatusCreated {
		t.Fatalf("admin create: %d %v", r.Status, r.Body)
	}
}

func TestCreateShapeAndTotals(t *testing.T) {
	e := newEnv(t)
	body := orderBody(paper, laptop)
	body["exchange_rate"] = 36.5
	body["expected_at"] = "2026-12-31"
	body["notes"] = " hola "
	r := e.do(t, "POST", "/purchase-orders", "operator", body)
	if r.Status != http.StatusCreated {
		t.Fatalf("create: %d %v", r.Status, r.Body)
	}
	b := r.Body
	if b["status"] != "draft" || b["supplier_name"] != "Sur" || b["currency"] != "USD" || b["exchange_rate"] != 36.5 ||
		b["expected_at"] != "2026-12-31" || b["notes"] != "hola" || b["total_minor"] != float64(10*450+3*90000) {
		t.Fatalf("body: %v", b)
	}
	lines := b["lines"].([]any)
	l0, l1 := lines[0].(map[string]any), lines[1].(map[string]any)
	if l0["item_id"] != "cons" || l0["description"] != "Papel" || l0["received_qty"] != float64(0) || l0["new_item"] != nil ||
		l1["item_id"] != nil || l1["new_item"].(map[string]any)["kind"] != "asset" {
		t.Fatalf("lines: %v", lines)
	}
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name   string
		body   any
		status int
		code   string
		field  string
		msg    string
	}{
		{"malformed", `{"supplier_id":`, 400, "invalid_request", "", ""},
		{"unknown field", `{"supplier_id":1,"currency":"USD","bogus":1}`, 400, "invalid_request", "", ""},
		{"bad currency", map[string]any{"supplier_id": 1, "currency": "EUR"}, 422, "validation_failed", "currency", ""},
		{"unknown supplier", map[string]any{"supplier_id": 9, "currency": "USD"}, 422, "validation_failed", "supplier_id", ""},
		{"inactive supplier", map[string]any{"supplier_id": 2, "currency": "USD"}, 422, "supplier_inactive", "supplier_id", ""},
		{"bad date", map[string]any{"supplier_id": 1, "currency": "USD", "expected_at": "31/12/2026"}, 422, "validation_failed", "expected_at", ""},
		{"asset item_id", orderBody(map[string]any{"item_id": "asset", "quantity": 1}), 422, "validation_failed", "lines[0].item_id", "existing_item_must_be_consumable"},
		{"asset over cap", orderBody(map[string]any{"new_item": laptop["new_item"], "quantity": 101}), 422, "validation_failed", "lines[0].quantity", ""},
		{"new_item category", orderBody(map[string]any{"new_item": map[string]any{"description": "x", "kind": "asset"}, "quantity": 1}), 422, "validation_failed", "lines[0].new_item.category_id", ""},
		{"both item and new_item", orderBody(map[string]any{"item_id": "cons", "new_item": laptop["new_item"], "quantity": 1}), 422, "validation_failed", "lines[0].item_id", ""},
		{"negative cost", orderBody(map[string]any{"item_id": "cons", "quantity": 1, "unit_cost_minor": -5}), 422, "validation_failed", "lines[0].unit_cost_minor", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := e.do(t, "POST", "/purchase-orders", "operator", tc.body)
			if r.Status != tc.status || code(r) != tc.code {
				t.Fatalf("got %d %s: %v", r.Status, code(r), r.Body)
			}
			if tc.field != "" && field(r, tc.field) == "" {
				t.Fatalf("field %s missing: %v", tc.field, r.Body)
			}
			if tc.msg != "" && field(r, tc.field) != tc.msg {
				t.Fatalf("message = %q, want %q", field(r, tc.field), tc.msg)
			}
		})
	}
}

func TestPatchEditsDraftOnly(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, paper)
	r := e.do(t, "PATCH", path(id, ""), "operator", map[string]any{
		"currency": "VES", "exchange_rate": 40.25, "expected_at": "2027-01-15", "notes": "n",
		"lines": []any{paper, laptop},
	})
	if r.Status != http.StatusOK || r.Body["currency"] != "VES" || r.Body["exchange_rate"] != 40.25 ||
		r.Body["expected_at"] != "2027-01-15" || len(r.Body["lines"].([]any)) != 2 {
		t.Fatalf("patch: %d %v", r.Status, r.Body)
	}
	// Absent keeps, null clears; lines absent keeps the set.
	r = e.do(t, "PATCH", path(id, ""), "operator", `{"exchange_rate":null,"expected_at":null}`)
	if r.Status != http.StatusOK || r.Body["exchange_rate"] != nil || r.Body["expected_at"] != nil ||
		r.Body["currency"] != "VES" || len(r.Body["lines"].([]any)) != 2 {
		t.Fatalf("null/absent: %d %v", r.Status, r.Body)
	}
	// A fetched order can be sent back (read-only line fields are tolerated).
	got := e.do(t, "GET", path(id, ""), "viewer", nil)
	if r := e.do(t, "PATCH", path(id, ""), "operator", map[string]any{"lines": got.Body["lines"]}); r.Status != http.StatusOK {
		t.Fatalf("round trip: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "PATCH", path(id, ""), "operator", map[string]any{"lines": []any{}}); r.Status != http.StatusOK || len(r.Body["lines"].([]any)) != 0 {
		t.Fatalf("clear lines: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "PATCH", path(id, ""), "operator", map[string]any{"supplier_id": 2}); code(r) != "supplier_inactive" {
		t.Fatalf("inactive supplier: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "PATCH", path(id, ""), "operator", map[string]any{"expected_at": "nope"}); r.Status != 422 {
		t.Fatalf("bad date: %d", r.Status)
	}
	e.do(t, "PATCH", path(id, ""), "operator", map[string]any{"lines": []any{paper}})
	if r := e.do(t, "POST", path(id, "/order"), "operator", nil); r.Status != http.StatusOK {
		t.Fatalf("order: %d %v", r.Status, r.Body)
	}
	r = e.do(t, "PATCH", path(id, ""), "operator", map[string]any{"notes": "late"})
	if r.Status != http.StatusConflict || code(r) != "invalid_state" {
		t.Fatalf("patch ordered: %d %v", r.Status, r.Body)
	}
}

func TestStateErrors(t *testing.T) {
	e := newEnv(t)
	empty := e.do(t, "POST", "/purchase-orders", "operator", orderBody()).Body["id"].(float64)
	if r := e.do(t, "POST", path(int(empty), "/order"), "operator", nil); r.Status != 422 || field(r, "lines") == "" {
		t.Fatalf("order without lines: %d %v", r.Status, r.Body)
	}

	id := e.create(t, paper)
	rcv := map[string]any{"location_id": 1, "lines": []any{map[string]any{"line_id": 1, "quantity": 1}}}
	if r := e.do(t, "POST", path(id, "/receive"), "operator", rcv); r.Status != http.StatusConflict || code(r) != "invalid_state" {
		t.Fatalf("receive draft: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "POST", path(id, "/cancel"), "operator", nil); r.Status != http.StatusOK || r.Body["status"] != "cancelled" {
		t.Fatalf("cancel: %d %v", r.Status, r.Body)
	}
	for _, s := range []string{"/order", "/cancel", "/receive"} {
		r := e.do(t, "POST", path(id, s), "operator", rcv)
		msg, _ := r.Body["error"].(map[string]any)["message"].(string)
		if r.Status != http.StatusConflict || code(r) != "invalid_state" || !bytes.Contains([]byte(msg), []byte("cancelled")) {
			t.Fatalf("%s on cancelled: %d %v", s, r.Status, r.Body)
		}
	}
	for _, p := range []string{path(999, ""), path(999, "/order"), "/purchase-orders/abc"} {
		method := "GET"
		if p != path(999, "") && p != "/purchase-orders/abc" {
			method = "POST"
		}
		if r := e.do(t, method, p, "operator", nil); r.Status != http.StatusNotFound {
			t.Fatalf("%s %s: %d", method, p, r.Status)
		}
	}
}

func TestReceiveFlow(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, paper, laptop)
	if r := e.do(t, "POST", path(id, "/order"), "operator", nil); r.Status != http.StatusOK || r.Body["status"] != "ordered" || r.Body["ordered_at"] == nil {
		t.Fatalf("order: %d %v", r.Status, r.Body)
	}
	got := e.do(t, "GET", path(id, ""), "viewer", nil)
	lines := got.Body["lines"].([]any)
	paperLine := lines[0].(map[string]any)["id"].(float64)
	laptopLine := lines[1].(map[string]any)["id"].(float64)

	rcv := func(body map[string]any) resp { return e.do(t, "POST", path(id, "/receive"), "operator", body) }
	line := func(id float64, q int) map[string]any { return map[string]any{"line_id": id, "quantity": q} }

	// Validation: over receipt has its own code, location must exist.
	r := rcv(map[string]any{"location_id": 1, "lines": []any{line(paperLine, 11)}})
	if r.Status != 422 || code(r) != "over_receipt" || field(r, "lines[0].quantity") == "" {
		t.Fatalf("over receipt: %d %v", r.Status, r.Body)
	}
	if r := rcv(map[string]any{"location_id": 77, "lines": []any{line(paperLine, 1)}}); r.Status != 422 || field(r, "location_id") == "" {
		t.Fatalf("bad location: %d %v", r.Status, r.Body)
	}
	if r := rcv(map[string]any{"location_id": 1, "lines": []any{line(12345, 1)}}); r.Status != 422 || field(r, "lines[0].line_id") == "" {
		t.Fatalf("unknown line: %d %v", r.Status, r.Body)
	}
	if e.stock.ins != 0 {
		t.Fatalf("rejected receipts moved stock: %d", e.stock.ins)
	}

	r = rcv(map[string]any{"location_id": 1, "note": "camión", "client_key": "k1", "lines": []any{line(paperLine, 4), line(laptopLine, 2)}})
	if r.Status != http.StatusCreated {
		t.Fatalf("receive: %d %v", r.Status, r.Body)
	}
	receipt := r.Body["receipt"].(map[string]any)
	order := r.Body["order"].(map[string]any)
	if order["status"] != "partially_received" || receipt["note"] != "camión" || receipt["received_by"] != "u-operator" {
		t.Fatalf("receive result: %v", r.Body)
	}
	items := receipt["lines"].([]any)[1].(map[string]any)["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["code"] == items[1].(map[string]any)["code"] {
		t.Fatalf("asset items: %v", items)
	}

	again := rcv(map[string]any{"location_id": 1, "client_key": "k1", "lines": []any{line(paperLine, 4), line(laptopLine, 2)}})
	if again.Status != http.StatusOK || again.Body["receipt"].(map[string]any)["id"] != receipt["id"] || e.stock.ins != 1 {
		t.Fatalf("replay: %d %v ins=%d", again.Status, again.Body, e.stock.ins)
	}

	if r := rcv(map[string]any{"location_id": 1, "lines": []any{line(paperLine, 6), line(laptopLine, 1)}}); r.Status != http.StatusCreated || r.Body["order"].(map[string]any)["status"] != "received" {
		t.Fatalf("complete: %d %v", r.Status, r.Body)
	}
	final := e.do(t, "GET", path(id, ""), "viewer", nil)
	if final.Body["status"] != "received" || len(final.Body["receipts"].([]any)) != 2 {
		t.Fatalf("detail: %v", final.Body)
	}
	if r := e.do(t, "POST", path(id, "/cancel"), "operator", nil); r.Status != http.StatusConflict {
		t.Fatalf("cancel received: %d", r.Status)
	}
}

func TestListFiltersAndPagination(t *testing.T) {
	e := newEnv(t)
	a := e.create(t, paper)
	e.create(t, paper)
	e.do(t, "POST", path(a, "/order"), "operator", nil)

	r := e.do(t, "GET", "/purchase-orders?limit=1&offset=1", "viewer", nil)
	if r.Body["total"] != float64(2) || r.Body["limit"] != float64(1) || r.Body["offset"] != float64(1) || len(r.Body["data"].([]any)) != 1 {
		t.Fatalf("page: %v", r.Body)
	}
	if r := e.do(t, "GET", "/purchase-orders?status=ordered", "viewer", nil); r.Body["total"] != float64(1) {
		t.Fatalf("status: %v", r.Body)
	}
	if r := e.do(t, "GET", "/purchase-orders?supplier_id=1&q=su", "viewer", nil); r.Body["total"] != float64(2) {
		t.Fatalf("supplier/q: %v", r.Body)
	}
	if r := e.do(t, "GET", "/purchase-orders?q=%232", "viewer", nil); r.Body["total"] != float64(1) {
		t.Fatalf("q by id: %v", r.Body)
	}
	for _, bad := range []string{"?status=nope", "?limit=abc", "?limit=1000", "?offset=-1", "?supplier_id=x"} {
		if r := e.do(t, "GET", "/purchase-orders"+bad, "viewer", nil); r.Status != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d %v", bad, r.Status, r.Body)
		}
	}
}
