package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Migration_DatabaseConfigFromEnv(t *testing.T) {
	t.Run("loads database config from env", func(t *testing.T) {
		// ===== Arrange =====
		env := map[string]string{
			"DB_HOST":     "localhost",
			"DB_PORT":     "5432",
			"DB_NAME":     "wibee",
			"DB_USER":     "postgres",
			"DB_PASSWORD": "secret",
			"DB_SSLMODE":  "require",
		}

		// ===== Act =====
		config, err := databaseConfigFromEnv(mapLookup(env), nil)

		// ===== Assert =====
		require.NoError(t, err)
		assert.Equal(t, "localhost", config.host)
		assert.Equal(t, "5432", config.port)
		assert.Equal(t, "wibee", config.name)
		assert.Equal(t, "postgres", config.user)
		assert.Equal(t, "secret", config.password)
		assert.Equal(t, "require", config.sslMode)
	})

	t.Run("loads missing env values from dot env", func(t *testing.T) {
		// ===== Arrange =====
		dotEnv := map[string]string{
			"DB_HOST":     "localhost",
			"DB_PORT":     "5432",
			"DB_NAME":     "wibee",
			"DB_USER":     "postgres",
			"DB_PASSWORD": "secret",
		}

		// ===== Act =====
		config, err := databaseConfigFromEnv(mapLookup(nil), dotEnv)

		// ===== Assert =====
		require.NoError(t, err)
		assert.Equal(t, "localhost", config.host)
		assert.Equal(t, "disable", config.sslMode)
	})

	t.Run("process env overrides dot env", func(t *testing.T) {
		// ===== Arrange =====
		env := map[string]string{
			"DB_NAME": "process_db",
		}
		dotEnv := map[string]string{
			"DB_HOST":     "localhost",
			"DB_PORT":     "5432",
			"DB_NAME":     "dot_env_db",
			"DB_USER":     "postgres",
			"DB_PASSWORD": "secret",
		}

		// ===== Act =====
		config, err := databaseConfigFromEnv(mapLookup(env), dotEnv)

		// ===== Assert =====
		require.NoError(t, err)
		assert.Equal(t, "process_db", config.name)
	})

	t.Run("returns error when required env is missing", func(t *testing.T) {
		// ===== Arrange =====
		env := map[string]string{
			"DB_HOST": "localhost",
			"DB_PORT": "5432",
		}

		// ===== Act =====
		_, err := databaseConfigFromEnv(mapLookup(env), nil)

		// ===== Assert =====
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DB_NAME is required")
	})
}

func Test_Migration_LoadDatabaseConfig(t *testing.T) {
	t.Run("loads project root dot env", func(t *testing.T) {
		// ===== Arrange =====
		root := t.TempDir()
		unsetDatabaseEnv(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte(`
DB_HOST=localhost
DB_PORT=5432
DB_NAME=wibee
DB_USER=postgres
DB_PASSWORD="secret:with@chars"
`), 0644))

		// ===== Act =====
		config, err := loadDatabaseConfig(root)

		// ===== Assert =====
		require.NoError(t, err)
		assert.Equal(t, "secret:with@chars", config.password)
	})
}

func Test_Migration_DatabaseURLForModule(t *testing.T) {
	t.Run("builds escaped postgres url with module migration table", func(t *testing.T) {
		// ===== Arrange =====
		config := databaseConfig{
			host:     "localhost",
			port:     "5432",
			name:     "wibee",
			user:     "postgres",
			password: "secret:with@chars",
			sslMode:  "disable",
		}
		module := migrationModule{name: "iam"}

		// ===== Act =====
		databaseURL := config.urlForModule(module)

		// ===== Assert =====
		assert.Equal(t, "postgres://postgres:secret%3Awith%40chars@localhost:5432/wibee?sslmode=disable&x-migrations-table=schema_migrations_iam", databaseURL)
	})
}

func mapLookup(values map[string]string) envLookupFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func unsetDatabaseEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSLMODE"} {
		previous, existed := os.LookupEnv(key)
		require.NoError(t, os.Unsetenv(key))
		t.Cleanup(func() {
			if existed {
				require.NoError(t, os.Setenv(key, previous))
				return
			}
			require.NoError(t, os.Unsetenv(key))
		})
	}
}
