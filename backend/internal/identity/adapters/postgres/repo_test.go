package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/adapters"
	"github.com/edgar/inventario/backend/internal/identity/adapters/memory"
	"github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(t *testing.T) (*postgres.Repo, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration tests")
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE refresh_tokens, users CASCADE`); err != nil {
		t.Fatal(err)
	}
	return postgres.New(pool), pool
}

func mkUser(t *testing.T, r *postgres.Repo, email string, role domain.Role) domain.User {
	t.Helper()
	u := domain.User{Name: "N", Email: email, PasswordHash: "hash", Role: role, Active: true}
	if err := r.Create(context.Background(), &u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestUserCRUDAndUniqueness(t *testing.T) {
	r, _ := setup(t)
	ctx := context.Background()
	u := mkUser(t, r, "ana@example.com", domain.RoleViewer)
	if !domain.ValidID(u.ID) || u.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", u)
	}
	dup := domain.User{Name: "X", Email: "ANA@example.com", PasswordHash: "h", Role: domain.RoleViewer, Active: true}
	if err := r.Create(ctx, &dup); !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("duplicate: %v", err)
	}
	got, err := r.GetByEmail(ctx, "Ana@Example.com")
	if err != nil || got.ID != u.ID {
		t.Fatalf("by email: %+v %v", got, err)
	}
	if _, err := r.GetByID(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if n, _ := r.Count(ctx); n != 1 {
		t.Fatalf("count = %d", n)
	}
	list, _ := r.List(ctx)
	if len(list) != 1 {
		t.Fatalf("list = %d", len(list))
	}
}

func TestMutateEnforcesInvariantAtomically(t *testing.T) {
	r, _ := setup(t)
	ctx := context.Background()
	a1 := mkUser(t, r, "a1@example.com", domain.RoleAdmin)
	a2 := mkUser(t, r, "a2@example.com", domain.RoleAdmin)

	// Two admins try to demote each other concurrently: exactly one may win.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{a1.ID, a2.ID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = r.Mutate(ctx, id, func(u *domain.User, admins int) error {
				if err := u.CanChangeTo(domain.RoleViewer, true, admins); err != nil {
					return err
				}
				u.Role = domain.RoleViewer
				return nil
			})
		}()
	}
	wg.Wait()
	failures := 0
	for _, err := range errs {
		if errors.Is(err, domain.ErrLastAdmin) {
			failures++
		} else if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if failures != 1 {
		t.Fatalf("expected exactly one ErrLastAdmin, got %d (%v)", failures, errs)
	}
	list, _ := r.List(ctx)
	admins := 0
	for _, u := range list {
		if u.Role == domain.RoleAdmin {
			admins++
		}
	}
	if admins != 1 {
		t.Fatalf("admins left = %d, want 1", admins)
	}
}

func TestMutateRollbackOnError(t *testing.T) {
	r, _ := setup(t)
	ctx := context.Background()
	u := mkUser(t, r, "ana@example.com", domain.RoleViewer)
	boom := errors.New("boom")
	if _, err := r.Mutate(ctx, u.ID, func(x *domain.User, _ int) error { x.Name = "Changed"; return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, u.ID)
	if got.Name != "N" {
		t.Fatalf("name = %q, change leaked", got.Name)
	}
	if _, err := r.Mutate(ctx, "00000000-0000-4000-8000-000000000000", func(*domain.User, int) error { return nil }); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestRefreshTokenLifecycle(t *testing.T) {
	r, pool := setup(t)
	ctx := context.Background()
	u := mkUser(t, r, "ana@example.com", domain.RoleViewer)
	tokens := r.Tokens()
	exp := time.Now().Add(time.Hour)

	if err := tokens.Create(ctx, app.RefreshToken{UserID: u.ID, TokenHash: "h1", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Create(ctx, app.RefreshToken{UserID: u.ID, TokenHash: "h2", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	t1, err := tokens.GetByHash(ctx, "h1")
	if err != nil || t1.RevokedAt != nil || t1.UserID != u.ID {
		t.Fatalf("get: %+v %v", t1, err)
	}
	if _, err := tokens.GetByHash(ctx, "missing"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("missing: %v", err)
	}
	if ok, err := tokens.Revoke(ctx, t1.ID, time.Now()); !ok || err != nil {
		t.Fatalf("first revoke: %v %v", ok, err)
	}
	if ok, _ := tokens.Revoke(ctx, t1.ID, time.Now()); ok {
		t.Fatal("second revoke must report false")
	}
	if err := tokens.RevokeAllForUser(ctx, u.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	t2, _ := tokens.GetByHash(ctx, "h2")
	if t2.RevokedAt == nil {
		t.Fatal("h2 not revoked")
	}
	// Deleting the user cascades to tokens.
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.GetByHash(ctx, "h1"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("token should cascade-delete: %v", err)
	}
}

func TestRotateIsAtomicAndSerialized(t *testing.T) {
	r, pool := setup(t)
	ctx := context.Background()
	u := mkUser(t, r, "ana@example.com", domain.RoleViewer)
	tokens := r.Tokens()
	exp := time.Now().Add(time.Hour)
	if err := tokens.Create(ctx, app.RefreshToken{UserID: u.ID, TokenHash: "parent", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	parent, _ := tokens.GetByHash(ctx, "parent")

	const n = 8
	var wg sync.WaitGroup
	won := make([]bool, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			won[i], errs[i] = tokens.Rotate(ctx, parent.ID,
				app.RefreshToken{UserID: u.ID, TokenHash: "child-" + string(rune('a'+i)), ExpiresAt: exp}, time.Now())
		}()
	}
	wg.Wait()
	winners := 0
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("rotate %d: %v", i, errs[i])
		}
		if won[i] {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want 1", winners)
	}
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("tokens stored = %d, want parent + exactly one child", total)
	}
	parent, _ = tokens.GetByHash(ctx, "parent")
	if parent.RevokedAt == nil || parent.RotatedAt == nil || parent.ReplacedBy == nil {
		t.Fatalf("parent not marked rotated: %+v", parent)
	}
	child, err := tokens.GetByID(ctx, *parent.ReplacedBy)
	if err != nil || child.RevokedAt != nil || child.UserID != u.ID {
		t.Fatalf("child: %+v %v", child, err)
	}
	if _, err := tokens.GetByID(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("missing id: %v", err)
	}
	// A plain Revoke leaves rotated_at unset, so it never qualifies for grace.
	if ok, _ := tokens.Revoke(ctx, child.ID, time.Now()); !ok {
		t.Fatal("revoke child")
	}
	child, _ = tokens.GetByID(ctx, child.ID)
	if child.RotatedAt != nil {
		t.Fatal("revoke must not set rotated_at")
	}
}

func TestRefreshServiceGraceOnPostgres(t *testing.T) {
	r, _ := setup(t)
	ctx := context.Background()
	issuer, err := adapters.NewJWTIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	clk := &now
	svc := app.NewService(app.Deps{
		Users: r, Tokens: r.Tokens(), Hasher: memory.PlainHasher{}, Issuer: issuer,
		RefreshTTL: time.Hour, RotationGrace: 10 * time.Second, Now: func() time.Time { return *clk },
	})
	if _, err := svc.CreateUser(ctx, app.CreateUserInput{Name: "N", Email: "ana@example.com", Password: "correct-horse-battery", Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	s1, err := svc.Login(ctx, "ana@example.com", "correct-horse-battery", "ip")
	if err != nil {
		t.Fatal(err)
	}

	// Concurrent refreshes of the same cookie all succeed.
	const n = 6
	var wg sync.WaitGroup
	sessions := make([]app.Session, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sessions[i], errs[i] = svc.Refresh(ctx, s1.RefreshToken)
		}()
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("concurrent refresh %d: %v", i, errs[i])
		}
	}
	for i := range n {
		if _, err := svc.Refresh(ctx, sessions[i].RefreshToken); err != nil {
			t.Fatalf("successor %d: %v", i, err)
		}
	}
	// Inside the window a later retry still works.
	*clk = now.Add(9 * time.Second)
	if _, err := svc.Refresh(ctx, s1.RefreshToken); err != nil {
		t.Fatalf("retry inside window: %v", err)
	}
	// After it, reuse is rejected and revokes everything.
	*clk = now.Add(11 * time.Second)
	if _, err := svc.Refresh(ctx, s1.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reuse after window: %v", err)
	}
	if _, err := svc.Refresh(ctx, sessions[0].RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("family should be revoked: %v", err)
	}
}

func TestLogoutClosesGraceOnPostgres(t *testing.T) {
	r, _ := setup(t)
	ctx := context.Background()
	issuer, _ := adapters.NewJWTIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	svc := app.NewService(app.Deps{Users: r, Tokens: r.Tokens(), Hasher: memory.PlainHasher{}, Issuer: issuer, RefreshTTL: time.Hour})
	if _, err := svc.CreateUser(ctx, app.CreateUserInput{Name: "N", Email: "ana@example.com", Password: "correct-horse-battery", Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	s1, _ := svc.Login(ctx, "ana@example.com", "correct-horse-battery", "ip")
	s2, err := svc.Refresh(ctx, s1.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(ctx, s2.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, s1.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("grace after logout: %v", err)
	}
	// Expired tokens are rejected.
	s3, _ := svc.Login(ctx, "ana@example.com", "correct-horse-battery", "ip")
	later := app.NewService(app.Deps{Users: r, Tokens: r.Tokens(), Hasher: memory.PlainHasher{}, Issuer: issuer,
		RefreshTTL: time.Hour, Now: func() time.Time { return time.Now().Add(2 * time.Hour) }})
	if _, err := later.Refresh(ctx, s3.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expired: %v", err)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	setup(t)
	if err := db.Migrate(context.Background(), os.Getenv("TEST_DATABASE_URL")); err != nil {
		t.Fatal(err)
	}
}
