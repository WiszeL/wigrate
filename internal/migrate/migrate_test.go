package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiszel/wigrate/internal"
)

func Test_Migration_MigrateUp(t *testing.T) {
	t.Run("runs all modules", func(t *testing.T) {
		// ===== Arrange ===== //
		root := makeTestProject(t)
		t.Chdir(root)
		writeTestDotEnv(t, root)
		writeTestMigrationFile(t, root, "iam", "000001_init_user.up.sql")
		writeTestMigrationFile(t, root, "billing", "000001_init_invoice.up.sql")

		var calls [][]string
		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			assert.Equal(t, "migrate", cmd)
			calls = append(calls, args)
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := Up()

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, calls, 2)
		assert.Contains(t, calls[0], "up")
		assert.Contains(t, calls[1], "up")
	})

	t.Run("runs selected module", func(t *testing.T) {
		// ===== Arrange ===== //
		root := makeTestProject(t)
		t.Chdir(root)
		writeTestDotEnv(t, root)
		writeTestMigrationFile(t, root, "iam", "000001_init_user.up.sql")
		writeTestMigrationFile(t, root, "billing", "000001_init_invoice.up.sql")

		var calls [][]string
		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			assert.Equal(t, "migrate", cmd)
			calls = append(calls, args)
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := Up("iam")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, calls, 1)
		expected := []string{
			"-path", filepath.Join(root, "module", "iam", "migration"),
			"-database", "postgres://postgres:secret@localhost:5432/wibee?sslmode=disable&x-migrations-table=schema_migrations_iam",
			"up",
		}
		require.Len(t, calls[0], len(expected))
		for i := range expected {
			assert.Equal(t, expected[i], calls[0][i])
		}
	})

	t.Run("skips module without migration files", func(t *testing.T) {
		// ===== Arrange ===== //
		root := makeTestProject(t)
		t.Chdir(root)
		writeTestDotEnv(t, root)
		assert.NoError(t, os.MkdirAll(filepath.Join(root, "module", "iam", "migration"), 0755))

		called := false
		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			called = true
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := Up()

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("ignores no change error", func(t *testing.T) {
		// ===== Arrange ===== //
		root := makeTestProject(t)
		t.Chdir(root)
		writeTestDotEnv(t, root)
		writeTestMigrationFile(t, root, "iam", "000001_init_user.up.sql")

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			return errors.New("no change")
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := Up()

		// ===== Assert ===== //
		assert.NoError(t, err)
	})
}

func Test_Migration_MigrateDown(t *testing.T) {
	t.Run("runs selected module with steps", func(t *testing.T) {
		// ===== Arrange ===== //
		root := makeTestProject(t)
		t.Chdir(root)
		writeTestDotEnv(t, root)
		writeTestMigrationFile(t, root, "iam", "000001_init_user.down.sql")

		var calls [][]string
		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			assert.Equal(t, "migrate", cmd)
			calls = append(calls, args)
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := Down(1, "iam")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, calls, 1)
		expected := []string{
			"-path", filepath.Join(root, "module", "iam", "migration"),
			"-database", "postgres://postgres:secret@localhost:5432/wibee?sslmode=disable&x-migrations-table=schema_migrations_iam",
			"down", "1",
		}
		require.Len(t, calls[0], len(expected))
		for i := range expected {
			assert.Equal(t, expected[i], calls[0][i])
		}
	})

	t.Run("rejects invalid steps", func(t *testing.T) {
		// ===== Arrange ===== //

		// ===== Act ===== //
		err := Down(0)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "down steps must be greater than zero")
	})
}

func makeTestProject(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644))
	assert.NoError(t, os.MkdirAll(filepath.Join(root, "module"), 0755))

	return root
}

func writeTestDotEnv(t *testing.T, root string) {
	t.Helper()

	unsetDatabaseEnv(t)
	assert.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte(`
DB_HOST=localhost
DB_PORT=5432
DB_NAME=wibee
DB_USER=postgres
DB_PASSWORD=secret
`), 0644))
}

func writeTestMigrationFile(t *testing.T, root string, moduleName string, fileName string) {
	t.Helper()

	migrationDir := filepath.Join(root, "module", moduleName, "migration")
	assert.NoError(t, os.MkdirAll(migrationDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(migrationDir, fileName), []byte("-- test\n"), 0644))
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

func stubRunCommand(t *testing.T, stub func(cmd string, args ...string) error) func() {
	t.Helper()

	original := config.RunCommandFunc
	config.RunCommandFunc = func(cmd string, args ...string) error {
		return stub(cmd, slices.Clone(args)...)
	}

	return func() {
		config.RunCommandFunc = original
	}
}
