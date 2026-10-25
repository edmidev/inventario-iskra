package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/suppliers/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/suppliers/adapters/memory"
	"github.com/edgar/inventario/backend/internal/suppliers/app"
	"github.com/go-chi/chi/v5"
)

type env struct{ srv *httptest.Server }

func newEnv(t *testing.T) env {
	t.Helper()
	svc := app.NewService(memory.NewStore())
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
	return env{srv: srv}
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

func TestPermissions(t *testing.T) {
	e := newEnv(t)
	body := map[string]any{"name": "Sur"}

	if r := e.do(t, "GET", "/suppliers", "", nil); r.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous list: %d", r.Status)
	}
	if r := e.do(t, "POST", "/suppliers", "viewer", body); r.Status != http.StatusForbidden {
		t.Fatalf("viewer create: %d", r.Status)
	}
	if r := e.do(t, "POST", "/suppliers", "operator", body); r.Status != http.StatusCreated {
		t.Fatalf("operator create: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "GET", "/suppliers", "viewer", nil); r.Status != http.StatusOK {
		t.Fatalf("viewer list: %d", r.Status)
	}
	if r := e.do(t, "DELETE", "/suppliers/1", "viewer", nil); r.Status != http.StatusForbidden {
		t.Fatalf("viewer delete: %d", r.Status)
	}
}

func TestCrudFlow(t *testing.T) {
	e := newEnv(t)

	r := e.do(t, "POST", "/suppliers", "admin", map[string]any{"name": " Sur ", "email": "v@sur.test"})
	if r.Status != http.StatusCreated || r.Body["name"] != "Sur" || r.Body["active"] != true {
		t.Fatalf("create: %d %v", r.Status, r.Body)
	}
	id := int(r.Body["id"].(float64))

	if r := e.do(t, "POST", "/suppliers", "admin", map[string]any{"name": "sur"}); r.Status != http.StatusConflict || code(r) != "supplier_name_taken" {
		t.Fatalf("duplicate: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "POST", "/suppliers", "admin", map[string]any{"name": ""}); r.Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "POST", "/suppliers", "admin", `{"name":"x","bogus":1}`); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", r.Status)
	}

	path := "/suppliers/" + itoa(id)
	r = e.do(t, "PATCH", path, "operator", map[string]any{"active": false, "phone": "555"})
	if r.Status != http.StatusOK || r.Body["active"] != false || r.Body["phone"] != "555" || r.Body["email"] != "v@sur.test" {
		t.Fatalf("patch: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "GET", path, "viewer", nil); r.Status != http.StatusOK || r.Body["name"] != "Sur" {
		t.Fatalf("get: %d %v", r.Status, r.Body)
	}
	if r := e.do(t, "DELETE", path, "operator", nil); r.Status != http.StatusNoContent {
		t.Fatalf("delete: %d", r.Status)
	}
	if r := e.do(t, "GET", path, "viewer", nil); r.Status != http.StatusNotFound {
		t.Fatalf("get after delete: %d", r.Status)
	}
	if r := e.do(t, "GET", "/suppliers/abc", "viewer", nil); r.Status != http.StatusNotFound {
		t.Fatalf("bad id: %d", r.Status)
	}
}

func TestListFiltersAndPagination(t *testing.T) {
	e := newEnv(t)
	for _, n := range []string{"Alfa", "Beta", "Gamma"} {
		e.do(t, "POST", "/suppliers", "admin", map[string]any{"name": n, "active": n != "Beta"})
	}

	r := e.do(t, "GET", "/suppliers?limit=2&offset=1", "viewer", nil)
	data := r.Body["data"].([]any)
	if r.Body["total"] != float64(3) || r.Body["limit"] != float64(2) || r.Body["offset"] != float64(1) ||
		len(data) != 2 || data[0].(map[string]any)["name"] != "Beta" {
		t.Fatalf("page: %v", r.Body)
	}
	if r := e.do(t, "GET", "/suppliers?active=false", "viewer", nil); r.Body["total"] != float64(1) {
		t.Fatalf("active=false: %v", r.Body)
	}
	if r := e.do(t, "GET", "/suppliers?q=gam", "viewer", nil); r.Body["total"] != float64(1) {
		t.Fatalf("q: %v", r.Body)
	}
	for _, bad := range []string{"?active=maybe", "?limit=abc", "?limit=1000", "?offset=-1"} {
		if r := e.do(t, "GET", "/suppliers"+bad, "viewer", nil); r.Status != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d %v", bad, r.Status, r.Body)
		}
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
