package internal

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type databaseConfig struct {
	Host     string `envconfig:"DB_HOST" required:"true"`
	Port     string `envconfig:"DB_PORT" required:"true"`
	Name     string `envconfig:"DB_NAME" required:"true"`
	User     string `envconfig:"DB_USER" required:"true"`
	Password string `envconfig:"DB_PASSWORD" required:"true"`
	SSLMode  string `envconfig:"DB_SSLMODE" default:"disable"`
}

func loadDatabaseConfig(root string) (databaseConfig, error) {
	// Loading the .env file
	if err := godotenv.Load(filepath.Join(root, ".env")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return databaseConfig{}, fmt.Errorf("load .env: %w", err)
	}

	// Reading env vars and building the config
	var cfg databaseConfig
	if err := envconfig.Process("", &cfg); err != nil {
		return databaseConfig{}, fmt.Errorf("process env: %w", err)
	}

	return cfg, nil
}

func (config databaseConfig) urlForModule(module migrationModule) string {
	// Building the postgres URL with connection parameters
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
