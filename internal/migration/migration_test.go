package migration

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
		assert.NoError(t, err)
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
		require.NoError(t, os.WriteFile(filepath.Join(module.EntityDir, "session.go"), []byte(`package entity

type Session struct {
	ID          string
	Thumbprint string // DPoP key thumbprint bound at login
}
`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(module.MigrationDir, ".wigrateignore"), []byte("session\n"), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			upPath := filepath.Join(module.MigrationDir, "000001_init_user.up.sql")
			downPath := filepath.Join(module.MigrationDir, "000001_init_user.down.sql")
			require.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			require.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makePerModule(module, false)

		// ===== Assert ===== //
		assert.NoError(t, err, "session.go's invalid DSL comment must not abort generation once ignored")

		upSQL, readErr := os.ReadFile(filepath.Join(module.MigrationDir, "000001_init_user.up.sql"))
		assert.NoError(t, readErr)
		assert.Contains(t, string(upSQL), "CREATE TABLE users")
	})
}
