// Package database loads the Postgres connection config from environment variables.
package database

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

// Load reads the database config from environment variables.
func Load(root string) (Config, error) {
	// Loading the .env file
	if err := loadEnvFile(filepath.Join(root, ".env")); err != nil {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	// Building the config
	cfg := Config{
		Host:     os.Getenv("DB_HOST"),
		Port:     os.Getenv("DB_PORT"),
		Name:     os.Getenv("DB_NAME"),
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		SSLMode:  "disable",
	}
	if v := os.Getenv("DB_SSLMODE"); v != "" {
		cfg.SSLMode = v
	}

	// Validating required fields
	for _, pair := range []struct{ key, val string }{
		{"DB_HOST", cfg.Host},
		{"DB_PORT", cfg.Port},
		{"DB_NAME", cfg.Name},
		{"DB_USER", cfg.User},
		{"DB_PASSWORD", cfg.Password},
	} {
		if pair.val == "" {
			return Config{}, fmt.Errorf("required env var %s is not set", pair.key)
		}
	}

	return cfg, nil
}
