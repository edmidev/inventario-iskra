package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func verifier(token string) (Principal, error) {
	switch token {
	case "viewer":
		return Principal{UserID: "u1", Role: "viewer", Permissions: []string{"inventory:read"}}, nil
	case "admin":
		return Principal{UserID: "u2", Role: "admin", Permissions: []string{"inventory:read", "users:manage"}}, nil
	}
	return Principal{}, errors.New("bad")
}

func do(h http.Handler, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticateAndPermissions(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		WriteJSON(w, 200, map[string]string{"user": p.UserID})
	})
	h := Authenticate(verifier)(RequirePermission("users:manage")(ok))

	if rec := do(h, ""); rec.Code != 401 {
		t.Errorf("no token: %d", rec.Code)
	}
	if rec := do(h, "Bearer nope"); rec.Code != 401 {
		t.Errorf("bad token: %d", rec.Code)
	}
	if rec := do(h, "Basic abc"); rec.Code != 401 {
		t.Errorf("wrong scheme: %d", rec.Code)
	}
	rec := do(h, "Bearer viewer")
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), `"code":"forbidden"`) {
		t.Errorf("viewer: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "bearer admin"); rec.Code != 200 {
		t.Errorf("admin: %d", rec.Code)
	}
}

func TestRequireRole(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	h := Authenticate(verifier)(RequireRole("admin")(ok))
	if rec := do(h, "Bearer viewer"); rec.Code != 403 {
		t.Errorf("viewer: %d", rec.Code)
	}
	if rec := do(h, "Bearer admin"); rec.Code != 204 {
		t.Errorf("admin: %d", rec.Code)
	}
}

func TestWriteErrorShape(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, 400, CodeInvalidRequest, "bad")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":{"code":"invalid_request","message":"bad"}}` {
		t.Fatalf("body = %s", got)
	}
}
