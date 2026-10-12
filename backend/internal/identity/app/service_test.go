package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/adapters"
	"github.com/edgar/inventario/backend/internal/identity/adapters/memory"
	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
)

const pw = "correct-horse-battery"

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type fixture struct {
	svc   *app.Service
	store *memory.Store
	clk   *clock
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	issuer, err := adapters.NewJWTIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewStore()
	clk := &clock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	svc := app.NewService(app.Deps{
		Users: store, Tokens: store.Tokens(), Hasher: memory.PlainHasher{}, Issuer: issuer,
		Limiter:    app.NewMemoryLoginLimiter(3, time.Minute, clk.now),
		RefreshTTL: 24 * time.Hour, Now: clk.now,
	})
	return fixture{svc: svc, store: store, clk: clk}
}

func (f fixture) user(t *testing.T, email string, role domain.Role) domain.User {
	t.Helper()
	u, err := f.svc.CreateUser(context.Background(), app.CreateUserInput{Name: "N", Email: email, Password: pw, Role: role})
	if err != nil {
		t.Fatalf("create %s: %v", email, err)
	}
	return u
}

func TestLoginSuccessIssuesTokens(t *testing.T) {
	f := newFixture(t)
	f.user(t, "ana@example.com", domain.RoleOperator)
	s, err := f.svc.Login(context.Background(), " Ana@Example.com ", pw, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if s.AccessToken == "" || s.RefreshToken == "" {
		t.Fatal("missing tokens")
	}
	claims, err := f.svc.VerifyAccess(s.AccessToken)
	if err != nil || claims.Role != domain.RoleOperator || claims.UserID != s.User.ID {
		t.Fatalf("claims = %+v, %v", claims, err)
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	f := newFixture(t)
	u := f.user(t, "ana@example.com", domain.RoleViewer)
	inactive := f.user(t, "off@example.com", domain.RoleViewer)
	admin := f.user(t, "root@example.com", domain.RoleAdmin)
	_ = admin
	off := false
	if _, err := f.svc.UpdateUser(context.Background(), inactive.ID, app.UpdateUserInput{Active: &off}); err != nil {
		t.Fatal(err)
	}
	cases := map[string][2]string{
		"unknown email":  {"nobody@example.com", pw},
		"wrong password": {u.Email, "wrong-password-1"},
		"deactivated":    {"off@example.com", pw},
		"malformed":      {"not-an-email", pw},
	}
	for name, c := range cases {
		_, err := f.svc.Login(context.Background(), c[0], c[1], "9.9.9.9"+name)
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", name, err)
		}
	}
}

func TestLoginRateLimited(t *testing.T) {
	f := newFixture(t)
	f.user(t, "ana@example.com", domain.RoleViewer)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := f.svc.Login(ctx, "ana@example.com", "bad-password-x", "ip"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	// Even the right password is refused while throttled.
	if _, err := f.svc.Login(ctx, "ana@example.com", pw, "ip"); !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Fatalf("throttled login: %v", err)
	}
	// Another client is unaffected.
	if _, err := f.svc.Login(ctx, "ana@example.com", pw, "other-ip"); err != nil {
		t.Fatalf("other client: %v", err)
	}
	// Window expiry lifts the block.
	f.clk.t = f.clk.t.Add(2 * time.Minute)
	if _, err := f.svc.Login(ctx, "ana@example.com", pw, "ip"); err != nil {
		t.Fatalf("after window: %v", err)
	}
}

func TestRefreshRotatesAndRejectsReuse(t *testing.T) {
	f := newFixture(t)
	f.user(t, "ana@example.com", domain.RoleViewer)
	ctx := context.Background()
	s1, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")

	s2, err := f.svc.Refresh(ctx, s1.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if s2.RefreshToken == s1.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	// Reusing the old token after the grace window fails and burns the family.
	f.clk.t = f.clk.t.Add(app.DefaultRotationGrace)
	if _, err := f.svc.Refresh(ctx, s1.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reuse: %v", err)
	}
	if _, err := f.svc.Refresh(ctx, s2.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("descendant after reuse should be revoked: %v", err)
	}
}

func TestRefreshReuseWithinGraceIsIdempotent(t *testing.T) {
	f := newFixture(t)
	f.user(t, "ana@example.com", domain.RoleViewer)
	ctx := context.Background()
	s1, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")

	s2, err := f.svc.Refresh(ctx, s1.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	f.clk.t = f.clk.t.Add(app.DefaultRotationGrace - time.Second)
	s3, err := f.svc.Refresh(ctx, s1.RefreshToken)
	if err != nil {
		t.Fatalf("reuse inside grace: %v", err)
	}
	if s3.RefreshToken == s1.RefreshToken || s3.RefreshToken == s2.RefreshToken || s3.AccessToken == "" {
		t.Fatal("grace refresh must issue a fresh session")
	}
	// Both successors are usable.
	if _, err := f.svc.Refresh(ctx, s2.RefreshToken); err != nil {
		t.Fatalf("first successor: %v", err)
	}
	if _, err := f.svc.Refresh(ctx, s3.RefreshToken); err != nil {
		t.Fatalf("second successor: %v", err)
	}
	// After the window the original token is theft evidence again.
	f.clk.t = f.clk.t.Add(time.Second)
	if _, err := f.svc.Refresh(ctx, s1.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reuse after grace: %v", err)
	}
}

func TestRefreshGraceClosedByLogoutAndPasswordChange(t *testing.T) {
	f := newFixture(t)
	u := f.user(t, "ana@example.com", domain.RoleViewer)
	ctx := context.Background()

	s1, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")
	s2, _ := f.svc.Refresh(ctx, s1.RefreshToken)
	if err := f.svc.Logout(ctx, s2.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(ctx, s1.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("grace after logout: %v", err)
	}

	s3, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")
	if _, err := f.svc.Refresh(ctx, s3.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ResetPassword(ctx, u.ID, "another-password-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(ctx, s3.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("grace after password reset: %v", err)
	}
}

func TestRefreshGraceDisabled(t *testing.T) {
	f := newFixture(t)
	f.user(t, "ana@example.com", domain.RoleViewer)
	ctx := context.Background()
	issuer, err := adapters.NewJWTIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc := app.NewService(app.Deps{
		Users: f.store, Tokens: f.store.Tokens(), Hasher: memory.PlainHasher{}, Issuer: issuer,
		RefreshTTL: time.Hour, RotationGrace: -1, Now: f.clk.now,
	})
	s1, _ := svc.Login(ctx, "ana@example.com", pw, "ip")
	if _, err := svc.Refresh(ctx, s1.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, s1.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reuse with grace disabled: %v", err)
	}
}

func TestRefreshConcurrentSameTokenSucceeds(t *testing.T) {
	f := newFixture(t)
	f.user(t, "ana@example.com", domain.RoleViewer)
	ctx := context.Background()
	s1, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")

	const n = 8
	var wg sync.WaitGroup
	sessions := make([]app.Session, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sessions[i], errs[i] = f.svc.Refresh(ctx, s1.RefreshToken)
		}()
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("refresh %d: %v", i, errs[i])
		}
		if _, err := f.svc.Refresh(ctx, sessions[i].RefreshToken); err != nil {
			t.Fatalf("successor %d unusable: %v", i, err)
		}
	}
}

func TestRefreshRejectsExpiredUnknownAndDeactivated(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "root@example.com", domain.RoleAdmin)
	u := f.user(t, "ana@example.com", domain.RoleViewer)

	if _, err := f.svc.Refresh(ctx, ""); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("empty: %v", err)
	}
	if _, err := f.svc.Refresh(ctx, "unknown"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("unknown: %v", err)
	}

	s, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")
	f.clk.t = f.clk.t.Add(25 * time.Hour)
	if _, err := f.svc.Refresh(ctx, s.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("expired: %v", err)
	}

	f.clk.t = f.clk.t.Add(-25 * time.Hour)
	s, _ = f.svc.Login(ctx, "ana@example.com", pw, "ip")
	off := false
	if _, err := f.svc.UpdateUser(ctx, u.ID, app.UpdateUserInput{Active: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(ctx, s.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("deactivated: %v", err)
	}
}

func TestLogoutRevokesRefreshToken(t *testing.T) {
	f := newFixture(t)
	f.user(t, "ana@example.com", domain.RoleViewer)
	ctx := context.Background()
	s, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")
	if err := f.svc.Logout(ctx, s.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(ctx, s.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("refresh after logout: %v", err)
	}
	if err := f.svc.Logout(ctx, "garbage"); err != nil {
		t.Fatalf("logout must be idempotent: %v", err)
	}
}

func TestLastActiveAdminProtected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a1 := f.user(t, "a1@example.com", domain.RoleAdmin)
	viewer := domain.RoleViewer
	off := false

	if _, err := f.svc.UpdateUser(ctx, a1.ID, app.UpdateUserInput{Role: &viewer}); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", err)
	}
	if _, err := f.svc.UpdateUser(ctx, a1.ID, app.UpdateUserInput{Active: &off}); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("deactivate last admin: %v", err)
	}
	a2 := f.user(t, "a2@example.com", domain.RoleAdmin)
	if _, err := f.svc.UpdateUser(ctx, a1.ID, app.UpdateUserInput{Role: &viewer}); err != nil {
		t.Fatalf("demote with two admins: %v", err)
	}
	// a2 is now the last one again.
	if _, err := f.svc.UpdateUser(ctx, a2.ID, app.UpdateUserInput{Active: &off}); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("deactivate new last admin: %v", err)
	}
	// A deactivated admin does not count as active.
	a3 := f.user(t, "a3@example.com", domain.RoleAdmin)
	if _, err := f.svc.UpdateUser(ctx, a3.ID, app.UpdateUserInput{Active: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateUser(ctx, a2.ID, app.UpdateUserInput{Role: &viewer}); !errors.Is(err, domain.ErrLastAdmin) {
		t.Fatalf("inactive admins must not count: %v", err)
	}
}

func TestUpdateUserNameRoleAndValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "root@example.com", domain.RoleAdmin)
	u := f.user(t, "ana@example.com", domain.RoleViewer)
	name, op := "Ana Gomez", domain.RoleOperator
	got, err := f.svc.UpdateUser(ctx, u.ID, app.UpdateUserInput{Name: &name, Role: &op})
	if err != nil || got.Name != "Ana Gomez" || got.Role != domain.RoleOperator {
		t.Fatalf("got %+v, %v", got, err)
	}
	bad := domain.Role("root")
	if _, err := f.svc.UpdateUser(ctx, u.ID, app.UpdateUserInput{Role: &bad}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad role: %v", err)
	}
	if _, err := f.svc.UpdateUser(ctx, "not-a-uuid", app.UpdateUserInput{}); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("bad id: %v", err)
	}
}

func TestCreateUserValidationAndDuplicates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "ana@example.com", domain.RoleViewer)
	cases := map[string]app.CreateUserInput{
		"short password": {Name: "N", Email: "x@example.com", Password: "short", Role: domain.RoleViewer},
		"bad role":       {Name: "N", Email: "x@example.com", Password: pw, Role: "boss"},
		"bad email":      {Name: "N", Email: "x", Password: pw, Role: domain.RoleViewer},
		"blank name":     {Name: " ", Email: "x@example.com", Password: pw, Role: domain.RoleViewer},
	}
	for name, in := range cases {
		if _, err := f.svc.CreateUser(ctx, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err := f.svc.CreateUser(ctx, app.CreateUserInput{Name: "N", Email: "ANA@example.com", Password: pw, Role: domain.RoleViewer})
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("duplicate email (case-insensitive): %v", err)
	}
}

func TestPasswordChangeAndResetRevokeSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	admin := f.user(t, "root@example.com", domain.RoleAdmin)
	u := f.user(t, "ana@example.com", domain.RoleViewer)
	s, _ := f.svc.Login(ctx, "ana@example.com", pw, "ip")

	if err := f.svc.ChangeOwnPassword(ctx, u.ID, "wrong-current-pw", "new-password-123"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong current: %v", err)
	}
	if err := f.svc.ChangeOwnPassword(ctx, u.ID, pw, "short"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("weak new: %v", err)
	}
	if err := f.svc.ChangeOwnPassword(ctx, u.ID, pw, "new-password-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(ctx, s.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("old session must be revoked: %v", err)
	}
	if _, err := f.svc.Login(ctx, "ana@example.com", pw, "ip2"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
	if err := f.svc.ResetPassword(ctx, u.ID, "admin-reset-pw-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Login(ctx, "ana@example.com", "admin-reset-pw-1", "ip3"); err != nil {
		t.Fatalf("login after reset: %v", err)
	}
	if err := f.svc.ResetPassword(ctx, admin.ID, "short"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("weak reset: %v", err)
	}
}

func TestBootstrapOnlyWhenEmpty(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if created, err := f.svc.Bootstrap(ctx, "", ""); created || err != nil {
		t.Fatalf("empty env: %v %v", created, err)
	}
	created, err := f.svc.Bootstrap(ctx, "root@example.com", pw)
	if !created || err != nil {
		t.Fatalf("first bootstrap: %v %v", created, err)
	}
	u, err := f.store.GetByEmail(ctx, "root@example.com")
	if err != nil || u.Role != domain.RoleAdmin || !u.Active {
		t.Fatalf("admin = %+v, %v", u, err)
	}
	if created, err := f.svc.Bootstrap(ctx, "other@example.com", pw); created || err != nil {
		t.Fatalf("second bootstrap must be a no-op: %v %v", created, err)
	}
}
