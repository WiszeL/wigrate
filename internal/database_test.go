package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Migration_LoadDatabaseConfig(t *testing.T) {
	t.Run("loads from .env file", func(t *testing.T) {
		// ===== Arrange ===== //
		unsetDatabaseEnv(t)
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte(`
			DB_HOST=localhost
			DB_PORT=5432
			DB_NAME=wibee
			DB_USER=postgres
			DB_PASSWORD="secret:with@chars"
		`), 0644))

		// ===== Act ===== //
		config, err := loadDatabaseConfig(root)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "localhost", config.Host)
		assert.Equal(t, "5432", config.Port)
		assert.Equal(t, "wibee", config.Name)
		assert.Equal(t, "postgres", config.User)
		assert.Equal(t, "secret:with@chars", config.Password)
		assert.Equal(t, "disable", config.SSLMode)
	})

	t.Run("env var overrides .env", func(t *testing.T) {
		// ===== Arrange ===== //
		unsetDatabaseEnv(t)
		t.Setenv("DB_NAME", "from_env")
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte(`
			DB_HOST=localhost
			DB_PORT=5432
			DB_NAME=from_dotenv
			DB_USER=postgres
			DB_PASSWORD=secret
		`), 0644))

		// ===== Act ===== //
		config, err := loadDatabaseConfig(root)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "from_env", config.Name)
	})

	t.Run("missing .env is not an error if env vars are set", func(t *testing.T) {
		// ===== Arrange ===== //
		t.Setenv("DB_HOST", "localhost")
		t.Setenv("DB_PORT", "5432")
		t.Setenv("DB_NAME", "wibee")
		t.Setenv("DB_USER", "postgres")
		t.Setenv("DB_PASSWORD", "secret")

		// ===== Act ===== //
		config, err := loadDatabaseConfig(t.TempDir())

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "wibee", config.Name)
	})

	t.Run("returns error when required env missing", func(t *testing.T) {
		// ===== Arrange ===== //
		unsetDatabaseEnv(t)
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte(`
			DB_HOST=localhost
			DB_PORT=5432
		`), 0644))

		// ===== Act ===== //
		_, err := loadDatabaseConfig(root)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DB_NAME")
	})
}

func unsetDatabaseEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSLMODE"} {
		previous, existed := os.LookupEnv(key)
		assert.NoError(t, os.Unsetenv(key))
		t.Cleanup(func() {
			if existed {
				assert.NoError(t, os.Setenv(key, previous))
				return
			}
			assert.NoError(t, os.Unsetenv(key))
		})
	}
}

func Test_Migration_DatabaseURLForModule(t *testing.T) {
	t.Run("builds escaped postgres url with module migration table", func(t *testing.T) {
		// ===== Arrange ===== //
		config := databaseConfig{
			Host:     "localhost",
			Port:     "5432",
			Name:     "wibee",
			User:     "postgres",
			Password: "secret:with@chars",
			SSLMode:  "disable",
		}
		module := migrationModule{name: "iam"}

		// ===== Act ===== //
		databaseURL := config.urlForModule(module)

		// ===== Assert ===== //
		assert.Equal(t, "postgres://postgres:secret%3Awith%40chars@localhost:5432/wibee?sslmode=disable&x-migrations-table=schema_migrations_iam", databaseURL)
	})
}
