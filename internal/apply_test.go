package internal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
		err := MigrateUp()

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
		err := MigrateUp("iam")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, calls, 1)
		assert.Contains(t, calls[0], filepath.Join(root, "module", "iam", "migration"))
		assert.Contains(t, calls[0], "up")
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
		err := MigrateUp()

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
		err := MigrateUp()

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
		err := MigrateDown(1, "iam")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, calls, 1)
		assert.Contains(t, calls[0], "down")
		assert.Contains(t, calls[0], "1")
	})

	t.Run("rejects invalid steps", func(t *testing.T) {
		// ===== Arrange ===== //

		// ===== Act ===== //
		err := MigrateDown(0)

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
