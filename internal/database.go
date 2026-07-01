package internal

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type databaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

func loadDatabaseConfig(root string) (databaseConfig, error) {
	if err := loadEnvFile(filepath.Join(root, ".env")); err != nil {
		return databaseConfig{}, fmt.Errorf("load .env: %w", err)
	}

	cfg := databaseConfig{
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

	for _, pair := range []struct{ key, val string }{
		{"DB_HOST", cfg.Host},
		{"DB_PORT", cfg.Port},
		{"DB_NAME", cfg.Name},
		{"DB_USER", cfg.User},
		{"DB_PASSWORD", cfg.Password},
	} {
		if pair.val == "" {
			return databaseConfig{}, fmt.Errorf("required env var %s is not set", pair.key)
		}
	}

	return cfg, nil
}

// loadEnvFile reads KEY=VALUE pairs from path into the environment.
// Handles: blank lines, # comments, export prefix, single/double-quoted values.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = unquote(val)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

// unquote strips surrounding quotes and expands escape sequences.
// Single-quoted values are returned verbatim (no escape expansion).
// Double-quoted values use strconv.Unquote (Go string semantics: \\, \n, \t, \").
func unquote(s string) string {
	if len(s) < 2 {
		return s
	}
	if s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	if s[0] == '"' && s[len(s)-1] == '"' {
		if v, err := strconv.Unquote(s); err == nil {
			return v
		}
		return s[1 : len(s)-1]
	}
	return s
}

func (config databaseConfig) urlForModule(module migrationModule) string {
	postgresURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   net.JoinHostPort(config.Host, config.Port),
		Path:   "/" + config.Name,
	}

	query := postgresURL.Query()
	query.Set("sslmode", config.SSLMode)
	query.Set("x-migrations-table", migrationTableName(module.name))
	postgresURL.RawQuery = query.Encode()

	return postgresURL.String()
}

func migrationTableName(moduleName string) string {
	return "schema_migrations_" + moduleName
}
