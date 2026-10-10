// Package config loads runtime configuration from environment variables.
package config

import "os"

// Config holds process-level settings.
type Config struct {
	Port        string
	DatabaseURL string
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", ""),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
