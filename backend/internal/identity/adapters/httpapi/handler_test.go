package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/adapters"
	"github.com/edgar/inventario/backend/internal/identity/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/identity/adapters/memory"
	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/go-chi/chi/v5"
)

const pw = "correct-horse-battery"

type env struct {
	srv *httptest.Server
	svc *app.Service
}

func newEnv(t *testing.T) env {
	t.Helper()
	issuer, _ := adapters.NewJWTIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	store := memory.NewStore()
	svc := app.NewService(app.Deps{
		Users: store, Tokens: store.Tokens(), Hasher: memory.PlainHasher{}, Issuer: issuer,
		Limiter: app.NewMemoryLoginLimiter(3, time.Minute, nil), RefreshTTL: time.Hour,
	})
	r := chi.NewRouter()
	r.Route("/api/v1", httpapi.New(svc, false).Register)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return env{srv: srv, svc: svc}
}

func (e env) seed(t *testing.T, email string, role domain.Role) {
	t.Helper()
	if _, err := e.svc.CreateUser(t.Context(), app.CreateUserInput{Name: "N", Email: email, Password: pw, Role: role}); err != nil {
		t.Fatal(err)
	}
}

func (e env) do(t *testing.T, method, path, token string, body any, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, e.srv.URL+"/api/v1"+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatal(err)
	}
}

func (e env) login(t *testing.T, email string) (token string, refresh *http.Cookie) {
	t.Helper()
	resp := e.do(t, "POST", "/auth/login", "", map[string]string{"email": email, "password": pw})
	if resp.StatusCode != 200 {
		t.Fatalf("login %s: %d", email, resp.StatusCode)
	}
	var s struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, resp, &s)
	for _, c := range resp.Cookies() {
		if c.Name == "refresh_token" {
			refresh = c
		}
	}
	return s.AccessToken, refresh
}

func TestProtectedEndpointsRequireToken(t *testing.T) {
	e := newEnv(t)
	for _, p := range [][2]string{{"GET", "/auth/me"}, {"GET", "/users"}, {"POST", "/users"}, {"PATCH", "/users/x"}, {"POST", "/auth/password"}} {
		resp := e.do(t, p[0], p[1], "", nil)
		if resp.StatusCode != 401 {
			t.Errorf("%s %s = %d, want 401", p[0], p[1], resp.StatusCode)
		}
		var body struct {
			Error struct{ Code, Message string }
		}
		decodeBody(t, resp, &body)
		if body.Error.Code != "unauthorized" {
			t.Errorf("%s %s error body = %+v", p[0], p[1], body)
		}
	}
}

func TestLoginSetsHardenedRefreshCookie(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "ana@example.com", domain.RoleViewer)
	_, c := e.login(t, "ana@example.com")
	if c == nil || c.Value == "" {
		t.Fatal("no refresh cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/api/v1/auth" {
		t.Fatalf("cookie not hardened: %+v", c)
	}
}

func TestLoginBadCredentialsAndBodyShape(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "ana@example.com", domain.RoleViewer)
	r1 := e.do(t, "POST", "/auth/login", "", map[string]string{"email": "ana@example.com", "password": "nope-nope-nope"})
	r2 := e.do(t, "POST", "/auth/login", "", map[string]string{"email": "ghost@example.com", "password": "nope-nope-nope"})
	var b1, b2 map[string]map[string]string
	decodeBody(t, r1, &b1)
	decodeBody(t, r2, &b2)
	if r1.StatusCode != 401 || r2.StatusCode != 401 || b1["error"]["message"] != b2["error"]["message"] {
		t.Fatalf("responses differ: %d %v / %d %v", r1.StatusCode, b1, r2.StatusCode, b2)
	}
	if r := e.do(t, "POST", "/auth/login", "", nil); r.StatusCode != 400 {
		t.Errorf("empty body = %d", r.StatusCode)
	}
}

func TestLoginThrottled(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "ana@example.com", domain.RoleViewer)
	var last *http.Response
	for i := 0; i < 4; i++ {
		last = e.do(t, "POST", "/auth/login", "", map[string]string{"email": "ana@example.com", "password": "nope-nope-nope"})
	}
	if last.StatusCode != 429 || last.Header.Get("Retry-After") == "" {
		t.Fatalf("status = %d", last.StatusCode)
	}
}

func TestRefreshCookieFlow(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "ana@example.com", domain.RoleViewer)
	_, c1 := e.login(t, "ana@example.com")

	resp := e.do(t, "POST", "/auth/refresh", "", nil, c1)
	if resp.StatusCode != 200 {
		t.Fatalf("refresh = %d", resp.StatusCode)
	}
	var c2 *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "refresh_token" {
			c2 = c
		}
	}
	if c2 == nil || c2.Value == c1.Value {
		t.Fatal("cookie not rotated")
	}
	if r := e.do(t, "POST", "/auth/refresh", "", nil, c1); r.StatusCode != 401 {
		t.Fatalf("reused token = %d", r.StatusCode)
	}
	if r := e.do(t, "POST", "/auth/refresh", "", nil); r.StatusCode != 401 {
		t.Fatalf("no cookie = %d", r.StatusCode)
	}
}

func TestLogoutClearsCookieAndRevokes(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "ana@example.com", domain.RoleViewer)
	_, c := e.login(t, "ana@example.com")
	resp := e.do(t, "POST", "/auth/logout", "", nil, c)
	if resp.StatusCode != 204 {
		t.Fatalf("logout = %d", resp.StatusCode)
	}
	cleared := false
	for _, ck := range resp.Cookies() {
		if ck.Name == "refresh_token" && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("cookie not cleared")
	}
	if r := e.do(t, "POST", "/auth/refresh", "", nil, c); r.StatusCode != 401 {
		t.Fatalf("refresh after logout = %d", r.StatusCode)
	}
}

func TestMeReturnsProfileWithoutSecrets(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "ana@example.com", domain.RoleOperator)
	tok, _ := e.login(t, "ana@example.com")
	resp := e.do(t, "GET", "/auth/me", tok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("me = %d", resp.StatusCode)
	}
	var raw bytes.Buffer
	raw.ReadFrom(resp.Body)
	body := raw.String()
	if !strings.Contains(body, `"role":"operator"`) || strings.Contains(strings.ToLower(body), "password") {
		t.Fatalf("body = %s", body)
	}
}

func TestViewerAndOperatorForbiddenOnUserAdmin(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "v@example.com", domain.RoleViewer)
	e.seed(t, "o@example.com", domain.RoleOperator)
	for _, email := range []string{"v@example.com", "o@example.com"} {
		tok, _ := e.login(t, email)
		for _, p := range [][2]string{{"GET", "/users"}, {"POST", "/users"}, {"PATCH", "/users/x"}, {"POST", "/users/x/password"}} {
			if resp := e.do(t, p[0], p[1], tok, map[string]string{}); resp.StatusCode != 403 {
				t.Errorf("%s %s %s = %d, want 403", email, p[0], p[1], resp.StatusCode)
			}
		}
	}
}

func TestAdminUserManagement(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "root@example.com", domain.RoleAdmin)
	tok, _ := e.login(t, "root@example.com")

	resp := e.do(t, "POST", "/users", tok, map[string]string{
		"name": "Ana", "email": "ana@example.com", "password": pw, "role": "viewer"})
	if resp.StatusCode != 201 {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	var created struct{ ID, Role string }
	decodeBody(t, resp, &created)

	if r := e.do(t, "POST", "/users", tok, map[string]string{
		"name": "Dup", "email": "ANA@example.com", "password": pw, "role": "viewer"}); r.StatusCode != 409 {
		t.Errorf("duplicate = %d", r.StatusCode)
	}
	if r := e.do(t, "POST", "/users", tok, map[string]string{
		"name": "X", "email": "x@example.com", "password": "short", "role": "viewer"}); r.StatusCode != 400 {
		t.Errorf("weak password = %d", r.StatusCode)
	}

	resp = e.do(t, "PATCH", "/users/"+created.ID, tok, map[string]any{"role": "operator", "active": true})
	var patched struct{ Role string }
	decodeBody(t, resp, &patched)
	if resp.StatusCode != 200 || patched.Role != "operator" {
		t.Fatalf("patch = %d %+v", resp.StatusCode, patched)
	}
	if r := e.do(t, "PATCH", "/users/does-not-exist", tok, map[string]any{"active": false}); r.StatusCode != 404 {
		t.Errorf("patch missing = %d", r.StatusCode)
	}

	resp = e.do(t, "GET", "/users", tok, nil)
	var list struct{ Data []struct{ Email string } }
	decodeBody(t, resp, &list)
	if len(list.Data) != 2 {
		t.Fatalf("list = %+v", list)
	}

	// Last-admin rule surfaces as 409.
	var me struct{ ID string }
	decodeBody(t, e.do(t, "GET", "/auth/me", tok, nil), &me)
	if r := e.do(t, "PATCH", "/users/"+me.ID, tok, map[string]any{"active": false}); r.StatusCode != 409 {
		t.Errorf("deactivate last admin = %d", r.StatusCode)
	}

	// Admin password reset lets the user sign in with the new password.
	if r := e.do(t, "POST", "/users/"+created.ID+"/password", tok, map[string]string{"new_password": "brand-new-password"}); r.StatusCode != 204 {
		t.Errorf("reset = %d", r.StatusCode)
	}
	if r := e.do(t, "POST", "/auth/login", "", map[string]string{"email": "ana@example.com", "password": "brand-new-password"}); r.StatusCode != 200 {
		t.Errorf("login with reset password = %d", r.StatusCode)
	}
}

func TestChangeOwnPassword(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "ana@example.com", domain.RoleViewer)
	tok, c := e.login(t, "ana@example.com")
	if r := e.do(t, "POST", "/auth/password", tok, map[string]string{"current_password": "wrong-wrong-1", "new_password": "another-password"}); r.StatusCode != 400 {
		t.Errorf("wrong current = %d", r.StatusCode)
	}
	if r := e.do(t, "POST", "/auth/password", tok, map[string]string{"current_password": pw, "new_password": "another-password"}); r.StatusCode != 204 {
		t.Fatalf("change = %d", r.StatusCode)
	}
	if r := e.do(t, "POST", "/auth/refresh", "", nil, c); r.StatusCode != 401 {
		t.Errorf("old refresh after password change = %d", r.StatusCode)
	}
}

func TestDeactivatedUserCannotUseMe(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "root@example.com", domain.RoleAdmin)
	e.seed(t, "ana@example.com", domain.RoleViewer)
	admin, _ := e.login(t, "root@example.com")
	ana, _ := e.login(t, "ana@example.com")
	var me struct{ ID string }
	decodeBody(t, e.do(t, "GET", "/auth/me", ana, nil), &me)
	if r := e.do(t, "PATCH", "/users/"+me.ID, admin, map[string]any{"active": false}); r.StatusCode != 200 {
		t.Fatalf("deactivate = %d", r.StatusCode)
	}
	if r := e.do(t, "GET", "/auth/me", ana, nil); r.StatusCode != 401 {
		t.Errorf("deactivated me = %d, want 401", r.StatusCode)
	}
}
