package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	identity "github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/stock/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/stock/adapters/memory"
	"github.com/edgar/inventario/backend/internal/stock/app"
	"github.com/edgar/inventario/backend/internal/stock/domain"
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
	st.SeedLocation(1, "Warehouse")
	st.SeedLocation(2, "Workshop")
	st.SeedUser("u-operator", "Olga Operator")
	st.SeedItem(memory.Item{ID: "c1", Code: "ISK-CON-001", Description: "Gloves", Kind: domain.KindConsumable, Unit: "pair", StockCurrent: 5, StockMin: 5, LocationID: p64(1)})
	st.SeedItem(memory.Item{ID: "a1", Code: "ISK-HER-001", Description: "Drill", Kind: domain.KindAsset, StockCurrent: 1, LocationID: p64(1)})
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
	h := httpapi.New(app.NewService(st), httpx.Authenticate(verify), httpapi.Permissions{
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

type m = map[string]any

func TestAuthAndPermissionMatrix(t *testing.T) {
	e := newEnv(t)
	post := m{"type": "in", "qty": 1}
	tests := []struct {
		name, method, path, token string
		body                      any
		want                      int
	}{
		{"post anonymous", "POST", "/items/c1/movements", "", post, 401},
		{"get anonymous", "GET", "/items/c1/movements", "", nil, 401},
		{"alerts anonymous", "GET", "/stock/alerts", "", nil, 401},
		{"viewer cannot post", "POST", "/items/c1/movements", "viewer", post, 403},
		{"operator posts", "POST", "/items/c1/movements", "operator", post, 201},
		{"admin posts", "POST", "/items/c1/movements", "admin", post, 201},
		{"viewer reads history", "GET", "/items/c1/movements", "viewer", nil, 200},
		{"viewer reads alerts", "GET", "/stock/alerts", "viewer", nil, 200},
	}
	for _, tc := range tests {
		if got := e.do(t, tc.method, tc.path, tc.token, tc.body); got.Status != tc.want {
			t.Errorf("%s: status %d, want %d (%v)", tc.name, got.Status, tc.want, got.Body)
		}
	}
	if n := len(e.st.Movements()); n != 2 {
		t.Errorf("forbidden/anonymous requests recorded movements: %d", n)
	}
}

func TestRecordReturnsMovementAndUpdatedStock(t *testing.T) {
	e := newEnv(t)
	got := e.do(t, "POST", "/items/c1/movements", "operator", m{"type": "out", "qty": 2, "note": "job 12"})
	if got.Status != 201 {
		t.Fatalf("status %d: %v", got.Status, got.Body)
	}
	mv := got.Body["movement"].(map[string]any)
	if mv["type"] != "out" || mv["qty"] != -2.0 || mv["stock_before"] != 5.0 || mv["stock_after"] != 3.0 ||
		mv["user_id"] != "u-operator" || mv["note"] != "job 12" || mv["item_id"] != "c1" {
		t.Errorf("movement: %v", mv)
	}
	if got.Body["stock_current"] != 3.0 {
		t.Errorf("stock_current = %v", got.Body["stock_current"])
	}
}

func TestRecordErrors(t *testing.T) {
	e := newEnv(t)
	t.Run("insufficient stock is 409", func(t *testing.T) {
		got := e.do(t, "POST", "/items/c1/movements", "operator", m{"type": "out", "qty": 6})
		if got.Status != 409 || got.errCode() != "insufficient_stock" {
			t.Errorf("%d %v", got.Status, got.Body)
		}
	})
	t.Run("assets reject stock changes", func(t *testing.T) {
		for _, typ := range []string{"in", "out"} {
			got := e.do(t, "POST", "/items/a1/movements", "operator", m{"type": typ, "qty": 1})
			if got.Status != 422 || got.errCode() != "not_applicable" {
				t.Errorf("%s: %d %v", typ, got.Status, got.Body)
			}
		}
	})
	t.Run("validation lists fields", func(t *testing.T) {
		cases := []struct {
			body  any
			field string
		}{
			{m{"type": "in", "qty": 0}, "qty"},
			{m{"type": "adjust", "qty": 3}, "note"},
			{m{"type": "transfer"}, "to_location_id"},
			{m{"type": "transfer", "to_location_id": 99}, "to_location_id"},
			{m{"type": "opening", "qty": 1}, "type"},
			{m{"type": ""}, "type"},
		}
		for _, c := range cases {
			got := e.do(t, "POST", "/items/c1/movements", "operator", c.body)
			if got.Status != 422 || got.errCode() != "validation_failed" || got.field(c.field) == "" {
				t.Errorf("%v: %d %v", c.body, got.Status, got.Body)
			}
		}
	})
	t.Run("malformed and unknown fields are 400", func(t *testing.T) {
		for _, body := range []any{"{", m{"type": "in", "qty": 1, "user_id": "x"}} {
			if got := e.do(t, "POST", "/items/c1/movements", "operator", body); got.Status != 400 {
				t.Errorf("%v: %d", body, got.Status)
			}
		}
	})
	t.Run("unknown item is 404", func(t *testing.T) {
		if got := e.do(t, "POST", "/items/nope/movements", "operator", m{"type": "in", "qty": 1}); got.Status != 404 {
			t.Errorf("status %d", got.Status)
		}
		if got := e.do(t, "GET", "/items/nope/movements", "viewer", nil); got.Status != 404 {
			t.Errorf("history status %d", got.Status)
		}
	})
	if n := len(e.st.Movements()); n != 0 {
		t.Errorf("failed requests recorded %d movements", n)
	}
}

func TestTransferWorksForAssets(t *testing.T) {
	e := newEnv(t)
	got := e.do(t, "POST", "/items/a1/movements", "operator", m{"type": "transfer", "to_location_id": 2, "note": "to workshop"})
	if got.Status != 201 {
		t.Fatalf("%d %v", got.Status, got.Body)
	}
	mv := got.Body["movement"].(map[string]any)
	if mv["from_location_id"] != 1.0 || mv["to_location_id"] != 2.0 || mv["qty"] != 0.0 || got.Body["location_id"] != 2.0 || got.Body["stock_current"] != 1.0 {
		t.Errorf("transfer: %v", got.Body)
	}
	if again := e.do(t, "POST", "/items/a1/movements", "operator", m{"type": "transfer", "to_location_id": 2}); again.Status != 422 {
		t.Errorf("transfer to the same location: %d", again.Status)
	}
}

func TestHistoryEnvelopeFilterAndPaging(t *testing.T) {
	e := newEnv(t)
	for _, body := range []m{{"type": "in", "qty": 1}, {"type": "out", "qty": 2}, {"type": "in", "qty": 3}} {
		if got := e.do(t, "POST", "/items/c1/movements", "operator", body); got.Status != 201 {
			t.Fatalf("%d %v", got.Status, got.Body)
		}
	}
	got := e.do(t, "GET", "/items/c1/movements?limit=2", "viewer", nil)
	data := got.Body["data"].([]any)
	if got.Status != 200 || got.Body["total"] != 3.0 || got.Body["limit"] != 2.0 || got.Body["offset"] != 0.0 || len(data) != 2 {
		t.Fatalf("envelope: %d %v", got.Status, got.Body)
	}
	first := data[0].(map[string]any)
	if first["qty"] != 3.0 || first["user_name"] != "Olga Operator" {
		t.Errorf("newest first with user name: %v", first)
	}
	if got := e.do(t, "GET", "/items/c1/movements?type=out", "viewer", nil); got.Body["total"] != 1.0 {
		t.Errorf("type filter: %v", got.Body)
	}
	if got := e.do(t, "GET", "/items/c1/movements", "viewer", nil); got.Body["limit"] != 25.0 {
		t.Errorf("default limit: %v", got.Body["limit"])
	}
	for _, q := range []string{"limit=101", "limit=abc", "offset=-1", "type=bogus"} {
		if got := e.do(t, "GET", "/items/c1/movements?"+q, "viewer", nil); got.Status != 422 {
			t.Errorf("?%s: %d", q, got.Status)
		}
	}
}

func TestAlertsEndpoint(t *testing.T) {
	e := newEnv(t)
	e.st.SeedItem(memory.Item{ID: "c2", Code: "ISK-CON-002", Description: "Tape", Kind: domain.KindConsumable, Unit: "roll", StockCurrent: 0, StockMin: 4})
	e.st.SeedItem(memory.Item{ID: "c3", Code: "ISK-CON-003", Description: "Fine", Kind: domain.KindConsumable, Unit: "u", StockCurrent: 9, StockMin: 4})
	got := e.do(t, "GET", "/stock/alerts", "viewer", nil)
	data := got.Body["data"].([]any)
	if got.Status != 200 || got.Body["total"] != 2.0 || len(data) != 2 {
		t.Fatalf("%d %v", got.Status, got.Body)
	}
	a := data[0].(map[string]any)
	if a["code"] != "ISK-CON-002" || a["shortage"] != -4.0 || a["unit"] != "roll" || a["stock_min"] != 4.0 {
		t.Errorf("most severe first: %v", a)
	}
	b := data[1].(map[string]any)
	if b["location_name"] != "Warehouse" || b["description"] != "Gloves" || b["stock_current"] != 5.0 {
		t.Errorf("alert fields: %v", b)
	}
}

func TestTransferOfAssetOnLoanIs409(t *testing.T) {
	e := newEnv(t)
	e.st.SeedItem(memory.Item{ID: "a-loan", Code: "ISK-HER-002", Description: "Saw", Kind: domain.KindAsset, StockCurrent: 1, LocationID: p64(1), OnLoan: true})
	got := e.do(t, "POST", "/items/a-loan/movements", "operator", m{"type": "transfer", "to_location_id": 2})
	if got.Status != 409 || got.errCode() != "item_on_loan" {
		t.Errorf("%d %v", got.Status, got.Body)
	}
	if n := len(e.st.Movements()); n != 0 {
		t.Errorf("recorded %d movements", n)
	}
}

// seedMulti adds a consumable stocked in two locations (warehouse 6,
// workshop 4) with a minimum of 5.
func seedMulti(e env) {
	e.st.SeedItem(memory.Item{ID: "m1", Code: "ISK-CON-050", Description: "Screws", Kind: domain.KindConsumable, Unit: "u",
		StockMin: 5, Stocks: map[int64]int{1: 6, 2: 4}})
}

func TestOutPerLocationAndInsufficientStockNamesTheLocation(t *testing.T) {
	e := newEnv(t)
	seedMulti(e)
	got := e.do(t, "POST", "/items/m1/movements", "operator", m{"type": "out", "qty": 3, "location_id": 2})
	if got.Status != 201 || got.Body["stock_current"] != 7.0 {
		t.Fatalf("%d %v", got.Status, got.Body)
	}
	mv := got.Body["movement"].(map[string]any)
	if mv["location_id"] != 2.0 || mv["moved_qty"] != 0.0 || mv["qty"] != -3.0 {
		t.Errorf("movement: %v", mv)
	}
	stocks := got.Body["stocks"].([]any)
	if len(stocks) != 2 || stocks[0].(map[string]any)["location_name"] != "Warehouse" || stocks[1].(map[string]any)["quantity"] != 1.0 {
		t.Errorf("stocks: %v", stocks)
	}
	over := e.do(t, "POST", "/items/m1/movements", "operator", m{"type": "out", "qty": 2, "location_id": 2}) // only 1 left there
	if over.Status != 409 || over.errCode() != "insufficient_stock" {
		t.Fatalf("%d %v", over.Status, over.Body)
	}
	msg := over.Body["error"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "Workshop") || !strings.Contains(msg, "1") {
		t.Errorf("message does not name the location and quantity: %q", msg)
	}
}

func TestAmbiguousLocationIs422WithFields(t *testing.T) {
	e := newEnv(t)
	seedMulti(e)
	for _, body := range []m{
		{"type": "in", "qty": 1}, {"type": "out", "qty": 1}, {"type": "adjust", "qty": 1, "note": "n"},
	} {
		got := e.do(t, "POST", "/items/m1/movements", "operator", body)
		if got.Status != 422 || got.errCode() != "location_required" || got.field("location_id") == "" {
			t.Errorf("%v: %d %v", body, got.Status, got.Body)
		}
	}
	got := e.do(t, "POST", "/items/m1/movements", "operator", m{"type": "transfer", "qty": 1, "to_location_id": 2})
	if got.Status != 422 || got.errCode() != "location_required" || got.field("from_location_id") == "" {
		t.Errorf("transfer: %d %v", got.Status, got.Body)
	}
	if n := len(e.st.Movements()); n != 0 {
		t.Errorf("recorded %d movements", n)
	}
	if got := e.do(t, "POST", "/items/m1/movements", "viewer", m{"type": "in", "qty": 1, "location_id": 1}); got.Status != 403 {
		t.Errorf("viewer: %d", got.Status)
	}
	if got := e.do(t, "POST", "/items/m1/movements", "", m{"type": "in", "qty": 1, "location_id": 1}); got.Status != 401 {
		t.Errorf("anonymous: %d", got.Status)
	}
}

func TestLegacySingleLocationRequestsStillWork(t *testing.T) {
	e := newEnv(t) // c1: 5 gloves, one location (Warehouse)
	got := e.do(t, "POST", "/items/c1/movements", "operator", m{"type": "out", "qty": 2})
	if got.Status != 201 || got.Body["stock_current"] != 3.0 {
		t.Fatalf("%d %v", got.Status, got.Body)
	}
	if mv := got.Body["movement"].(map[string]any); mv["location_id"] != 1.0 {
		t.Errorf("inferred location: %v", mv)
	}
	got = e.do(t, "POST", "/items/c1/movements", "operator", m{"type": "adjust", "qty": 8, "note": "count"})
	if got.Status != 201 || got.Body["stock_current"] != 8.0 {
		t.Errorf("adjust: %d %v", got.Status, got.Body)
	}
}

func TestPartialTransferBetweenLocations(t *testing.T) {
	e := newEnv(t)
	seedMulti(e)
	got := e.do(t, "POST", "/items/m1/movements", "operator", m{"type": "transfer", "qty": 2, "from_location_id": 1, "to_location_id": 2})
	if got.Status != 201 || got.Body["stock_current"] != 10.0 {
		t.Fatalf("%d %v", got.Status, got.Body)
	}
	mv := got.Body["movement"].(map[string]any)
	if mv["qty"] != 0.0 || mv["moved_qty"] != 2.0 || mv["from_location_id"] != 1.0 || mv["to_location_id"] != 2.0 || mv["location_id"] != nil {
		t.Errorf("movement: %v", mv)
	}
	stocks := got.Body["stocks"].([]any)
	if stocks[0].(map[string]any)["quantity"] != 4.0 || stocks[1].(map[string]any)["quantity"] != 6.0 {
		t.Errorf("stocks: %v", stocks)
	}
	if over := e.do(t, "POST", "/items/m1/movements", "operator", m{"type": "transfer", "qty": 5, "from_location_id": 1, "to_location_id": 2}); over.Status != 409 || over.errCode() != "insufficient_stock" {
		t.Errorf("over-transfer: %d %v", over.Status, over.Body)
	}
	for _, body := range []m{
		{"type": "transfer", "from_location_id": 1, "to_location_id": 2},            // no qty
		{"type": "transfer", "qty": 1, "from_location_id": 1, "to_location_id": 1},  // same location
		{"type": "transfer", "qty": 1, "from_location_id": 1, "to_location_id": 99}, // unknown
		{"type": "out", "qty": 1, "location_id": 99},                                // unknown
		{"type": "in", "qty": 1, "location_id": 1, "from_location_id": 2},           // from only for transfer
	} {
		if got := e.do(t, "POST", "/items/m1/movements", "operator", body); got.Status != 422 || got.errCode() != "validation_failed" {
			t.Errorf("%v: %d %v", body, got.Status, got.Body)
		}
	}
}

func TestAlertsReturnTheBreakdown(t *testing.T) {
	e := newEnv(t)
	seedMulti(e) // total 10 > min 5: no alert yet
	e.do(t, "POST", "/items/m1/movements", "operator", m{"type": "out", "qty": 4, "location_id": 2})
	e.do(t, "POST", "/items/m1/movements", "operator", m{"type": "out", "qty": 1, "location_id": 1}) // total 5 = min
	got := e.do(t, "GET", "/stock/alerts", "viewer", nil)
	var alert map[string]any
	for _, a := range got.Body["data"].([]any) {
		if a.(map[string]any)["code"] == "ISK-CON-050" {
			alert = a.(map[string]any)
		}
	}
	if alert == nil || alert["stock_current"] != 5.0 {
		t.Fatalf("alerts: %v", got.Body)
	}
	stocks := alert["stocks"].([]any)
	if len(stocks) != 1 || stocks[0].(map[string]any)["location_name"] != "Warehouse" || stocks[0].(map[string]any)["quantity"] != 5.0 {
		t.Errorf("breakdown: %v", stocks)
	}
}
