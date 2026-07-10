package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_IsGoEntityFile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "entity file", input: "user.go", expected: true},
		{name: "entity file nested", input: "role.go", expected: true},
		{name: "test file", input: "user_test.go", expected: false},
		{name: "non-go file", input: "README.md", expected: false},
		{name: "go test with extra dots", input: "user_service_test.go", expected: false},
		{name: "go file with dots", input: "user.service.go", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGoEntityFile(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func Test_Migration_FilterModules(t *testing.T) {
	t.Run("returns all modules when no module is selected", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []migrationModule{
			{name: "iam"},
			{name: "billing"},
		}

		// ===== Act ===== //
		filtered, err := filterModules(modules, "")

		// ===== Assert ===== //
		assert.NoError(t, err)

		assert.Equal(t, modules, filtered)
	})

	t.Run("returns selected module", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []migrationModule{
			{name: "iam"},
			{name: "billing"},
		}

		// ===== Act ===== //
		filtered, err := filterModules(modules, "iam")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []migrationModule{{name: "iam"}}, filtered)
	})

	t.Run("returns error when module is missing", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []migrationModule{
			{name: "iam"},
			{name: "billing"},
		}

		// ===== Act ===== //
		_, err := filterModules(modules, "catalog")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "module not found: catalog")
	})
}

func Test_LoadIgnoreSet(t *testing.T) {
	t.Run("returns nil when .wigrateignore is missing", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()

		// ===== Act ===== //
		set, err := loadIgnoreSet(dir)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Nil(t, set)
	})

	t.Run("parses entity names, skipping blanks and comments", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".wigrateignore"), []byte("session\n\n# not migrated\ncache_entry\n"), 0644))

		// ===== Act ===== //
		set, err := loadIgnoreSet(dir)

		// ===== Assert ===== //
		require.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"session": {}, "cache_entry": {}}, set)
	})
}

func Test_MakePerModule_WigrateIgnore(t *testing.T) {
	t.Run("skips entities listed in .wigrateignore", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID uuid.UUID
}
`)
		// Session is Redis-only: plain (non-DSL) inline comment would otherwise abort generation.
		require.NoError(t, os.WriteFile(filepath.Join(module.entityDir, "session.go"), []byte(`package entity

type Session struct {
	ID          string
	Thumbprint string // DPoP key thumbprint bound at login
}
`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, ".wigrateignore"), []byte("session\n"), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			upPath := filepath.Join(module.migrationDir, "000001_init_user.up.sql")
			downPath := filepath.Join(module.migrationDir, "000001_init_user.down.sql")
			require.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			require.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makePerModule(module, false)

		// ===== Assert ===== //
		require.NoError(t, err, "session.go's invalid DSL comment must not abort generation once ignored")

		upSQL, readErr := os.ReadFile(filepath.Join(module.migrationDir, "000001_init_user.up.sql"))
		require.NoError(t, readErr)
		assert.Contains(t, string(upSQL), "CREATE TABLE users")
	})
}
