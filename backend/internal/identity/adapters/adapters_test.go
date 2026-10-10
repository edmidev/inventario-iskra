package adapters

import (
	"strings"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/golang-jwt/jwt/v5"
)

func TestArgon2idRoundTrip(t *testing.T) {
	h := Argon2idHasher{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 32} // cheap for tests
	hash, err := h.Hash("s3cret-password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, "s3cret") {
		t.Fatalf("unexpected encoding %q", hash)
	}
	if !h.Verify("s3cret-password", hash) {
		t.Fatal("correct password rejected")
	}
	if h.Verify("other", hash) {
		t.Fatal("wrong password accepted")
	}
	other, _ := h.Hash("s3cret-password")
	if other == hash {
		t.Fatal("salt must differ between hashes")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=999999999,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=8,t=1,p=1$!!$!!"} {
		if h.Verify("x", bad) {
			t.Errorf("malformed hash %q verified", bad)
		}
	}
}

func TestJWTIssueParse(t *testing.T) {
	j, _ := NewJWTIssuer("0123456789abcdef0123456789abcdef", 15*time.Minute)
	now := time.Now()
	tok, exp, err := j.IssueAccess(domain.User{ID: "u1", Role: domain.RoleOperator}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("exp = %v", exp)
	}
	c, err := j.ParseAccess(tok, now.Add(time.Minute))
	if err != nil || c.UserID != "u1" || c.Role != domain.RoleOperator {
		t.Fatalf("claims = %+v, %v", c, err)
	}
	if _, err := j.ParseAccess(tok, now.Add(16*time.Minute)); err == nil {
		t.Error("expired token accepted")
	}
	other, _ := NewJWTIssuer("ffffffffffffffffffffffffffffffff", time.Minute)
	if _, err := other.ParseAccess(tok, now); err == nil {
		t.Error("token signed with another secret accepted")
	}
	// alg=none must be rejected.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, accessClaims{Role: domain.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{Subject: "u1", ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}}).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := j.ParseAccess(none, now); err == nil {
		t.Error("unsigned token accepted")
	}
	if _, err := j.ParseAccess("garbage", now); err == nil {
		t.Error("garbage accepted")
	}
}
