package httpapi_test

import (
	"bytes"
	"errors"
	"github.com/edgar/inventario/backend/internal/customers/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/customers/adapters/memory"
	"github.com/edgar/inventario/backend/internal/customers/app"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"testing"
)

func TestCustomerHTTP(t *testing.T) {
	r := chi.NewRouter()
	auth := httpx.Authenticate(func(token string) (httpx.Principal, error) {
		if token == "" {
			return httpx.Principal{}, errors.New("bad token")
		}
		perms := []string{"customers:read"}
		if token == "writer" {
			perms = append(perms, "customers:write")
		}
		return httpx.Principal{UserID: "u", Permissions: perms}, nil
	})
	httpapi.New(app.NewService(memory.NewStore()), auth, httpapi.Permissions{Read: "customers:read", Write: "customers:write"}).Register(r)
	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := do("POST", "/customers/", "reader", `{"name":"x"}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := do("POST", "/customers/", "writer", `{"name":"  Acero SA ","identification":"J123","contact":"Ana","notes":"Taller"}`); w.Code != 201 || !bytes.Contains(w.Body.Bytes(), []byte(`"active":true`)) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := do("PATCH", "/customers/1", "writer", `{"contact":"Luis"}`); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"identification":"J123"`)) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := do("GET", "/customers/?q=acero", "reader", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := do("DELETE", "/customers/1", "writer", ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := do("GET", "/customers/1", "reader", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
