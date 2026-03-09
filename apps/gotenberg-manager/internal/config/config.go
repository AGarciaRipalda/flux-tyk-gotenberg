package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// Server
	Port string

	// Database
	DatabaseURL string

	// Gotenberg
	GotenbergURL string

	// Tyk
	TykURL       string
	TykAdminKey  string

	// Admin auth
	AdminToken string

	// Health check interval in seconds
	HealthCheckInterval int
}

func Load() *Config {
	return &Config{
		Port:                getEnv("PORT", "9090"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://gotenberg_manager:gotenberg_manager@localhost:5432/gotenberg_manager?sslmode=disable"),
		GotenbergURL:        getEnv("GOTENBERG_URL", "http://localhost:3000"),
		TykURL:              getEnv("TYK_URL", "http://localhost:8080"),
		TykAdminKey:         getEnv("TYK_ADMIN_KEY", "foo"),
		AdminToken:          getEnv("ADMIN_TOKEN", "admin-secret"),
		HealthCheckInterval: getEnvInt("HEALTH_CHECK_INTERVAL", 30),
	}
}

func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.AdminToken == "" {
		return fmt.Errorf("ADMIN_TOKEN is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
