package database

import (
	"net"
	"net/url"

	"github.com/wiszel/wigrate/internal/discover"
)

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
	query.Set("x-migrations-table", migrationTableName(module.Name))
	postgresURL.RawQuery = query.Encode()

	return postgresURL.String()
}

// migrationTableName returns the migration history table name for a module.
func migrationTableName(moduleName string) string {
	return "schema_migrations_" + moduleName
}
