// Package database loads the Postgres connection config from environment variables.
package database

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/wiszel/wigrate/internal/discover"
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

// URLForModule returns a Postgres connection string for the given module.
func (config Config) URLForModule(module discover.Module) string {
	// Building the base URL
	postgresURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   net.JoinHostPort(config.Host, config.Port),
		Path:   "/" + config.Name,
	}

	// Adding query parameters
	query := postgresURL.Query()
	query.Set("sslmode", config.SSLMode)
	// Per-module tracking table avoids version collisions between modules
	query.Set("x-migrations-table", "schema_migrations_"+module.Name)
	postgresURL.RawQuery = query.Encode()

	return postgresURL.String()
}
