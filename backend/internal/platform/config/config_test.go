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
