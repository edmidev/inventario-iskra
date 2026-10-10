package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identity "github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/maintenance/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/maintenance/adapters/memory"
	"github.com/edgar/inventario/backend/internal/maintenance/app"
	"github.com/edgar/inventario/backend/internal/maintenance/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

const uuidItem = "11111111-1111-4111-8111-111111111111"

func ip(i int) *int { return &i }

type env struct {
	srv *httptest.Server
	st  *memory.Store
}

func newEnv(t *testing.T) env {
	t.Helper()
	st := memory.NewStore()
	st.SeedUser("u-operator", "Olga Operator")
	st.SeedUser("u-admin", "Adam Admin")
	created := time.Now().Add(-100 * 24 * time.Hour)
	st.SeedItem(memory.Item{ID: "a1", Code: "ISK-HER-001", Description: "Drill", CategoryID: 1, CategoryName: "Tools", Kind: domain.ItemAsset, CreatedAt: created})
	st.SeedItem(memory.Item{ID: "a2", Code: "ISK-HER-002", Description: "Saw", CategoryID: 1, CategoryName: "Tools", Kind: domain.ItemAsset, IntervalDays: ip(30), CreatedAt: created})
	st.SeedItem(memory.Item{ID: uuidItem, Code: "ISK-VEH-001", Description: "Truck", CategoryID: 2, CategoryName: "Vehicles", Kind: domain.ItemAsset, IntervalDays: ip(3650), CreatedAt: time.Now()})
	st.SeedItem(memory.Item{ID: "c1", Code: "ISK-CON-001", Description: "Gloves", Kind: domain.ItemConsumable, CreatedAt: created})
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

func (r resp) str(name string) string { s, _ := r.Body[name].(string); return s }

type m = map[string]any

func TestAuthAndPermissionMatrix(t *testing.T) {
	e := newEnv(t)
	report := m{"kind": "corrective", "description": "Motor burnt"}
	tests := []struct {
		name, method, path, token string
		body                      any
		want                      int
	}{
		{"create anonymous", "POST", "/items/a1/maintenance", "", report, 401},
		{"update anonymous", "PATCH", "/maintenance/1", "", m{"description": "x"}, 401},
		{"close anonymous", "POST", "/maintenance/1/close", "", nil, 401},
		{"history anonymous", "GET", "/items/a1/maintenance", "", nil, 401},
		{"list anonymous", "GET", "/maintenance", "", nil, 401},
		{"due anonymous", "GET", "/maintenance/due", "", nil, 401},
		{"viewer cannot report", "POST", "/items/a1/maintenance", "viewer", report, 403},
		{"viewer cannot update", "PATCH", "/maintenance/1", "viewer", m{"description": "x"}, 403},
		{"viewer cannot close", "POST", "/maintenance/1/close", "viewer", nil, 403},
		{"operator reports", "POST", "/items/a1/maintenance", "operator", report, 201},
		{"viewer reads history", "GET", "/items/a1/maintenance", "viewer", nil, 200},
		{"viewer reads list", "GET", "/maintenance", "viewer", nil, 200},
		{"viewer reads due", "GET", "/maintenance/due", "viewer", nil, 200},
		{"operator updates", "PATCH", "/maintenance/1", "operator", m{"description": "Bearing"}, 200},
		{"operator closes", "POST", "/maintenance/1/close", "operator", nil, 200},
		{"admin registers preventive", "POST", "/items/a2/maintenance", "admin", m{"kind": "preventive", "description": "Greasing"}, 201},
	}
	for _, tc := range tests {
		if got := e.do(t, tc.method, tc.path, tc.token, tc.body); got.Status != tc.want {
			t.Errorf("%s: status %d, want %d (%v)", tc.name, got.Status, tc.want, got.Body)
		}
	}
	if n := len(e.st.Records()); n != 2 {
		t.Errorf("forbidden/anonymous requests wrote records: %d", n)
	}
}

func TestItemStatusThroughReportRepairAndClose(t *testing.T) {
	e := newEnv(t)
	rep := e.do(t, "POST", "/items/a1/maintenance", "operator", m{"kind": "corrective", "description": "Motor burnt", "destination": "Taller de Reparación Interno"})
	if rep.Status != 201 || rep.str("status") != "open" || rep.str("kind") != "corrective" || rep.str("item_status") != "inoperative" ||
		rep.str("destination") != "Taller de Reparación Interno" || rep.str("reported_by_name") != "Olga Operator" || rep.Body["closed_at"] != nil || rep.Body["cost"] != nil {
		t.Fatalf("report: %d %v", rep.Status, rep.Body)
	}
	id := int(rep.Body["id"].(float64))
	if e.st.Item("a1").Status != domain.ItemInoperative {
		t.Fatalf("item = %s", e.st.Item("a1").Status)
	}

	if dup := e.do(t, "POST", "/items/a1/maintenance", "operator", m{"kind": "corrective", "description": "again"}); dup.Status != 409 || dup.errCode() != "open_failure_exists" {
		t.Errorf("second failure: %d %v", dup.Status, dup.Body)
	}

	path := "/maintenance/" + itoa(id)
	up := e.do(t, "PATCH", path, "operator", m{"item_status": "in_repair", "destination": "Espera de Repuestos"})
	if up.Status != 200 || up.str("item_status") != "in_repair" || up.str("destination") != "Espera de Repuestos" || up.str("description") != "Motor burnt" {
		t.Fatalf("patch: %d %v", up.Status, up.Body)
	}
	if e.st.Item("a1").Status != domain.ItemInRepair {
		t.Fatalf("item = %s", e.st.Item("a1").Status)
	}
	if clr := e.do(t, "PATCH", path, "operator", `{"destination":null}`); clr.Status != 200 || clr.Body["destination"] != nil || clr.str("item_status") != "in_repair" {
		t.Errorf("null destination clears it: %d %v", clr.Status, clr.Body)
	}

	cl := e.do(t, "POST", path+"/close", "admin", m{"cost": 99.5, "resolution_note": "Replaced motor"})
	if cl.Status != 200 || cl.str("status") != "closed" || cl.str("item_status") != "functional" || cl.Body["cost"] != 99.5 ||
		cl.str("closed_by_name") != "Adam Admin" || cl.Body["closed_at"] == nil || cl.str("resolution_note") != "Replaced motor" {
		t.Fatalf("close: %d %v", cl.Status, cl.Body)
	}
	if it := e.st.Item("a1"); it.Status != domain.ItemFunctional || it.LastMaintenanceAt == nil {
		t.Errorf("item after close: %+v", it)
	}
	if again := e.do(t, "POST", path+"/close", "operator", nil); again.Status != 409 || again.errCode() != "record_closed" {
		t.Errorf("close twice: %d %v", again.Status, again.Body)
	}
	if edit := e.do(t, "PATCH", path, "operator", m{"description": "x"}); edit.Status != 409 || edit.errCode() != "record_closed" {
		t.Errorf("edit closed: %d %v", edit.Status, edit.Body)
	}
	if next := e.do(t, "POST", "/items/a1/maintenance", "operator", m{"kind": "corrective", "description": "new failure", "item_status": "in_repair"}); next.Status != 201 || next.str("item_status") != "in_repair" {
		t.Errorf("new failure after close: %d %v", next.Status, next.Body)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestPreventiveRegistration(t *testing.T) {
	e := newEnv(t)
	performed := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	got := e.do(t, "POST", "/items/a2/maintenance", "operator", m{"kind": "preventive", "description": "Greasing", "performed_at": performed.Format(time.RFC3339), "cost": 12.5})
	if got.Status != 201 || got.str("status") != "closed" || got.str("kind") != "preventive" || got.str("item_status") != "functional" ||
		got.Body["cost"] != 12.5 || got.str("reported_at") != performed.Format(time.RFC3339) {
		t.Fatalf("%d %v", got.Status, got.Body)
	}
	if want := domain.AddDays(performed, 30).Format(time.RFC3339); got.str("next_due_at") != want {
		t.Errorf("next_due_at = %s, want derived %s", got.str("next_due_at"), want)
	}
	if last := e.st.Item("a2").LastMaintenanceAt; last == nil || !last.Equal(performed) || e.st.Item("a2").Status != domain.ItemFunctional {
		t.Errorf("item: %+v", e.st.Item("a2"))
	}
	explicit := time.Now().Add(10 * 24 * time.Hour).UTC().Truncate(time.Second)
	got = e.do(t, "POST", "/items/a2/maintenance", "operator", m{"kind": "preventive", "description": "Again", "next_due_at": explicit.Format(time.RFC3339)})
	if got.Status != 201 || got.str("next_due_at") != explicit.Format(time.RFC3339) {
		t.Errorf("explicit next due: %d %v", got.Status, got.Body)
	}
	// A preventive service does not touch an item that is broken.
	e.do(t, "POST", "/items/a1/maintenance", "operator", m{"kind": "corrective", "description": "x", "item_status": "in_repair"})
	if p := e.do(t, "POST", "/items/a1/maintenance", "operator", m{"kind": "preventive", "description": "Check"}); p.Status != 201 || p.str("item_status") != "in_repair" {
		t.Errorf("preventive on broken item: %d %v", p.Status, p.Body)
	}
}

func TestCreateErrors(t *testing.T) {
	e := newEnv(t)
	t.Run("consumables are not applicable", func(t *testing.T) {
		for _, kind := range []string{"corrective", "preventive"} {
			got := e.do(t, "POST", "/items/c1/maintenance", "operator", m{"kind": kind, "description": "x"})
			if got.Status != 422 || got.errCode() != "not_applicable" {
				t.Errorf("%s: %d %v", kind, got.Status, got.Body)
			}
		}
	})
	t.Run("validation lists fields", func(t *testing.T) {
		future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
		cases := []struct {
			body  any
			field string
		}{
			{m{"description": "x"}, "kind"},
			{m{"kind": "other", "description": "x"}, "kind"},
			{m{"kind": "corrective"}, "description"},
			{m{"kind": "corrective", "description": "x", "reported_at": future}, "reported_at"},
			{m{"kind": "corrective", "description": "x", "item_status": "functional"}, "item_status"},
			{m{"kind": "corrective", "description": "x", "cost": 5}, "cost"},
			{m{"kind": "preventive", "description": "x", "item_status": "in_repair"}, "item_status"},
			{m{"kind": "preventive", "description": "x", "destination": "Taller"}, "destination"},
			{m{"kind": "preventive", "description": "x", "performed_at": future}, "performed_at"},
			{m{"kind": "preventive", "description": "x", "cost": -1}, "cost"},
			{m{"kind": "preventive", "description": "x", "performed_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), "next_due_at": time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)}, "next_due_at"},
		}
		for _, c := range cases {
			got := e.do(t, "POST", "/items/a1/maintenance", "operator", c.body)
			if got.Status != 422 || got.errCode() != "validation_failed" || got.field(c.field) == "" {
				t.Errorf("%v: %d %v", c.body, got.Status, got.Body)
			}
		}
	})
	t.Run("malformed and unknown fields are 400", func(t *testing.T) {
		for _, body := range []any{"{", m{"kind": "corrective", "description": "x", "user_id": "u"}, `{"kind":"corrective","description":"x","reported_at":"yesterday"}`} {
			if got := e.do(t, "POST", "/items/a1/maintenance", "operator", body); got.Status != 400 {
				t.Errorf("%v: %d", body, got.Status)
			}
		}
	})
	t.Run("unknown item and record are 404", func(t *testing.T) {
		if got := e.do(t, "POST", "/items/nope/maintenance", "operator", m{"kind": "corrective", "description": "x"}); got.Status != 404 {
			t.Errorf("create: %d", got.Status)
		}
		if got := e.do(t, "GET", "/items/nope/maintenance", "viewer", nil); got.Status != 404 {
			t.Errorf("history: %d", got.Status)
		}
		for _, c := range []struct{ method, path string }{{"PATCH", "/maintenance/999"}, {"PATCH", "/maintenance/abc"}, {"POST", "/maintenance/999/close"}, {"POST", "/maintenance/abc/close"}} {
			var body any = m{"description": "x"}
			if c.method == "POST" {
				body = nil
			}
			if got := e.do(t, c.method, c.path, "operator", body); got.Status != 404 {
				t.Errorf("%s %s: %d", c.method, c.path, got.Status)
			}
		}
	})
	t.Run("patch and close validation", func(t *testing.T) {
		rep := e.do(t, "POST", "/items/a2/maintenance", "operator", m{"kind": "corrective", "description": "x"})
		path := "/maintenance/" + itoa(int(rep.Body["id"].(float64)))
		for _, body := range []any{m{}, m{"description": " "}, m{"item_status": "functional"}} {
			if got := e.do(t, "PATCH", path, "operator", body); got.Status != 422 {
				t.Errorf("patch %v: %d %v", body, got.Status, got.Body)
			}
		}
		for field, body := range map[string]any{
			"item_status_after": m{"item_status_after": "in_repair"},
			"cost":              m{"cost": -3},
			"closed_at":         m{"closed_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
		} {
			if got := e.do(t, "POST", path+"/close", "operator", body); got.Status != 422 || got.field(field) == "" {
				t.Errorf("close %s: %d %v", field, got.Status, got.Body)
			}
		}
		// Rejected closes leave the record open and the item broken.
		if e.st.Item("a2").Status != domain.ItemInoperative {
			t.Errorf("item = %s", e.st.Item("a2").Status)
		}
		if got := e.do(t, "POST", path+"/close", "operator", m{"item_status_after": "inoperative"}); got.Status != 200 || got.str("item_status") != "inoperative" {
			t.Errorf("close as inoperative: %d %v", got.Status, got.Body)
		}
	})
}

func TestHistoryAndListEnvelopeFiltersAndPaging(t *testing.T) {
	e := newEnv(t)
	e.do(t, "POST", "/items/a1/maintenance", "operator", m{"kind": "preventive", "description": "Greasing", "performed_at": time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)})
	e.do(t, "POST", "/items/"+uuidItem+"/maintenance", "operator", m{"kind": "preventive", "description": "Annual service"})
	e.do(t, "POST", "/items/a1/maintenance", "operator", m{"kind": "corrective", "description": "Chuck broken", "destination": "Espera de Repuestos"})

	hist := e.do(t, "GET", "/items/a1/maintenance", "viewer", nil)
	d := hist.data()
	if hist.Status != 200 || hist.Body["total"] != 2.0 || hist.Body["limit"] != 25.0 || hist.Body["offset"] != 0.0 || len(d) != 2 ||
		d[0].(map[string]any)["description"] != "Chuck broken" {
		t.Fatalf("history: %v", hist.Body)
	}
	if page := e.do(t, "GET", "/items/a1/maintenance?limit=1&offset=1", "viewer", nil); len(page.data()) != 1 || page.data()[0].(map[string]any)["description"] != "Greasing" {
		t.Errorf("history page: %v", page.Body)
	}
	count := func(q string) int {
		got := e.do(t, "GET", "/maintenance"+q, "viewer", nil)
		if got.Status != 200 {
			t.Fatalf("%s: %d %v", q, got.Status, got.Body)
		}
		return len(got.data())
	}
	if first := e.do(t, "GET", "/maintenance", "viewer", nil).data()[0].(map[string]any); first["status"] != "open" || first["item_code"] != "ISK-HER-001" || first["item_description"] != "Drill" {
		t.Errorf("open first with item info: %v", first)
	}
	for q, want := range map[string]int{
		"": 3, "?status=open": 1, "?status=closed": 2, "?kind=preventive": 2, "?kind=corrective": 1,
		"?destination=espera%20de%20repuestos": 1, "?destination=espera": 0, "?item_id=" + uuidItem: 1,
		"?q=truck": 1, "?q=chuck": 1, "?q=ISK-HER-001": 2, "?limit=2": 2, "?limit=2&offset=2": 1,
	} {
		if got := count(q); got != want {
			t.Errorf("%q: %d rows, want %d", q, got, want)
		}
	}
	for _, q := range []string{"?status=x", "?kind=x", "?item_id=a1", "?limit=0x", "?limit=101", "?offset=-1"} {
		if got := e.do(t, "GET", "/maintenance"+q, "viewer", nil); got.Status != 422 || got.errCode() != "validation_failed" {
			t.Errorf("%q: %d %v", q, got.Status, got.Body)
		}
	}
}

func TestDueEndpoint(t *testing.T) {
	e := newEnv(t)
	got := e.do(t, "GET", "/maintenance/due", "viewer", nil)
	d := got.data()
	if got.Status != 200 || len(d) != 1 {
		t.Fatalf("default window: %d %v", got.Status, got.Body)
	}
	row := d[0].(map[string]any)
	if row["code"] != "ISK-HER-002" || row["maintenance_overdue"] != true || row["maintenance_interval_days"] != 30.0 ||
		row["category_name"] != "Tools" || row["maintenance_due_at"] == nil || row["last_maintenance_at"] != nil {
		t.Errorf("row: %v", row)
	}
	if far := e.do(t, "GET", "/maintenance/due?within_days=365", "viewer", nil); len(far.data()) != 1 { // truck is due in ~3650 days
		t.Errorf("365: %v", far.Body)
	}
	if byCat := e.do(t, "GET", "/maintenance/due?category_id=2&within_days=365", "viewer", nil); len(byCat.data()) != 0 {
		t.Errorf("category: %v", byCat.Body)
	}
	if q := e.do(t, "GET", "/maintenance/due?q=saw", "viewer", nil); len(q.data()) != 1 {
		t.Errorf("q: %v", q.Body)
	}
	for _, q := range []string{"?within_days=366", "?within_days=-1", "?within_days=x", "?category_id=x"} {
		if got := e.do(t, "GET", "/maintenance/due"+q, "viewer", nil); got.Status != 422 {
			t.Errorf("%q: %d", q, got.Status)
		}
	}
	// Servicing the saw clears it from the overdue list.
	e.do(t, "POST", "/items/a2/maintenance", "operator", m{"kind": "preventive", "description": "Service"})
	if after := e.do(t, "GET", "/maintenance/due", "viewer", nil); len(after.data()) != 0 {
		t.Errorf("after service: %v", after.Body)
	}
}
