package httpapi_test

import (
	"bytes"
	"context"
	"errors"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/projects/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/projects/adapters/memory"
	"github.com/edgar/inventario/backend/internal/projects/app"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"testing"
)

type catalog struct{}

func (catalog) Kind(_ context.Context, id string) (string, error) {
	switch id {
	case "steel":
		return "consumable", nil
	case "welder":
		return "asset", nil
	}
	return "", nil
}
func TestTemplateHTTPRules(t *testing.T) {
	r := chi.NewRouter()
	auth := httpx.Authenticate(func(token string) (httpx.Principal, error) {
		if token == "" {
			return httpx.Principal{}, errors.New("unauthorized")
		}
		ps := []string{"projects:read"}
		if token == "writer" {
			ps = append(ps, "projects:write")
		}
		return httpx.Principal{UserID: "user", Permissions: ps}, nil
	})
	httpapi.New(app.NewService(memory.NewStore(), catalog{}), auth, httpapi.Permissions{Read: "projects:read", Write: "projects:write"}).Register(r)
	body := `{"name":"Marmita","description":"Acero inoxidable","characteristics":["Alta presión"],"estimated_days":5,"consumables":[{"item_id":"steel","quantity":1.125}],"tools":["welder"]}`
	do := func(method, path, token, payload string) *httptest.ResponseRecorder {
		q := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
		if token != "" {
			q.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		return w
	}
	for _, tc := range []struct {
		name, token, body string
		code              int
	}{{"anonymous", "", body, 401}, {"read only", "reader", body, 403}, {"precision", "writer", `{"name":"x","estimated_days":1,"consumables":[{"item_id":"steel","quantity":0.0001}]}`, 400}, {"wrong kind", "writer", `{"name":"x","estimated_days":1,"tools":["steel"]}`, 422}, {"valid", "writer", body, 201}} {
		t.Run(tc.name, func(t *testing.T) {
			w := do(http.MethodPost, "/projects/", tc.token, tc.body)
			if w.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	if w := do("GET", "/projects/1", "reader", ""); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"quantity":1.125`)) {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
	if w := do("PUT", "/projects/1", "writer", `{"name":"New","estimated_days":2,"tools":[],"consumables":[],"characteristics":[]}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := do("DELETE", "/projects/1", "writer", ""); w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	if w := do("GET", "/projects/1", "reader", ""); w.Code != 404 {
		t.Fatalf("deleted %d", w.Code)
	}
}
