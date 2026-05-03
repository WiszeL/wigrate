package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Migration_ParseMigrationFile(t *testing.T) {
	t.Run("parses golang migrate init file", func(t *testing.T) {
		// ===== Arrange =====
		path := "/tmp/000001_init_user.up.sql"

		// ===== Act =====
		file, ok := parseMigrationFile(path, "user")

		// ===== Assert =====
		require.True(t, ok)

		assert.Equal(t, path, file.path)
		assert.Equal(t, "000001_init_user", file.baseName)
		assert.Equal(t, migrationKindInit, file.kind)
		assert.Equal(t, "up", file.direction)
	})

	t.Run("parses golang migrate alter file", func(t *testing.T) {
		// ===== Arrange =====
		path := "/tmp/000002_alter_email_name_user.down.sql"

		// ===== Act =====
		file, ok := parseMigrationFile(path, "user")

		// ===== Assert =====
		require.True(t, ok)

		assert.Equal(t, path, file.path)
		assert.Equal(t, "000002_alter_email_name_user", file.baseName)
		assert.Equal(t, migrationKindAlter, file.kind)
		assert.Equal(t, "down", file.direction)
	})

	t.Run("does not match entity name inside another entity suffix", func(t *testing.T) {
		// ===== Arrange =====
		path := "/tmp/000001_init_super_user.up.sql"

		// ===== Act =====
		_, ok := parseMigrationFile(path, "user")

		// ===== Assert =====
		assert.False(t, ok)
	})
}
