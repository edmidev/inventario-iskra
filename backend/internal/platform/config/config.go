// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// MinJWTSecretLen is the minimum accepted length of JWT_SECRET.
const MinJWTSecretLen = 32

// Config holds process-level settings.
type Config struct {
	Port        string
	DatabaseURL string

	JWTSecret    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	CookieSecure bool

	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", ""),

		JWTSecret:    getenv("JWT_SECRET", ""),
		AccessTTL:    getDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:   getDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		CookieSecure: getenv("COOKIE_SECURE", "true") != "false",

		BootstrapAdminEmail:    getenv("BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapAdminPassword: getenv("BOOTSTRAP_ADMIN_PASSWORD", ""),
	}
}

// ValidateServer checks the settings required to run the API.
func (c Config) ValidateServer() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < MinJWTSecretLen {
		return errors.New("JWT_SECRET is required and must be at least 32 characters")
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
