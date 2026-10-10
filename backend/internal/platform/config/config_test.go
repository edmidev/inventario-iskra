package config

import "testing"

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	if got := Load().Port; got != "8080" {
		t.Fatalf("default port = %q, want 8080", got)
	}
	t.Setenv("PORT", "9000")
	t.Setenv("DATABASE_URL", "postgres://x")
	c := Load()
	if c.Port != "9000" || c.DatabaseURL != "postgres://x" {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestValidateServer(t *testing.T) {
	ok := Config{DatabaseURL: "postgres://x", JWTSecret: "0123456789abcdef0123456789abcdef"}
	if err := ok.ValidateServer(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	short := ok
	short.JWTSecret = "short"
	if short.ValidateServer() == nil {
		t.Fatal("short JWT secret accepted")
	}
	noDB := ok
	noDB.DatabaseURL = ""
	if noDB.ValidateServer() == nil {
		t.Fatal("missing DATABASE_URL accepted")
	}
}

func TestLoadSecurityDefaults(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("ACCESS_TOKEN_TTL", "")
	c := Load()
	if !c.CookieSecure {
		t.Fatal("cookies must be Secure by default")
	}
	if c.AccessTTL.Minutes() != 15 {
		t.Fatalf("access TTL = %v, want 15m", c.AccessTTL)
	}
	t.Setenv("COOKIE_SECURE", "false")
	if Load().CookieSecure {
		t.Fatal("COOKIE_SECURE=false ignored")
	}
}
