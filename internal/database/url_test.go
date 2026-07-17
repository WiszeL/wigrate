package database

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wiszel/wigrate/internal/discover"
)

func Test_Migration_DatabaseURLForModule(t *testing.T) {
	t.Run("builds escaped postgres url with module migration table", func(t *testing.T) {
		// ===== Arrange ===== //
		config := Config{
			Host:     "localhost",
			Port:     "5432",
			Name:     "wibee",
			User:     "postgres",
			Password: "secret:with@chars",
			SSLMode:  "disable",
		}
		module := discover.Module{Name: "iam"}

		// ===== Act ===== //
		databaseURL := config.URLForModule(module)

		// ===== Assert ===== //
		assert.Equal(t, "postgres://postgres:secret%3Awith%40chars@localhost:5432/wibee?sslmode=disable&x-migrations-table=schema_migrations_iam", databaseURL)
	})
}
