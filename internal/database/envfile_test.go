package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_EnvParse_LoadEnvFile(t *testing.T) {
	// ===== Arrange ===== //
	write := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		return path
	}

	tests := []struct {
		name    string
		content string
		key     string
		want    string
	}{
		{"bare value", "KEY=value\n", "KEY", "value"},
		{"single-quoted value", "KEY='hello world'\n", "KEY", "hello world"},
		{"double-quoted value", `KEY="hello world"`, "KEY", "hello world"},
		{"double-quoted with escape", `KEY="say \"hi\""`, "KEY", `say "hi"`},
		{"export prefix", "export KEY=value\n", "KEY", "value"},
		{"comment skipped", "# this is a comment\nKEY=value\n", "KEY", "value"},
		{"blank line skipped", "\n\nKEY=value\n", "KEY", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ===== Arrange ===== //
			t.Setenv(tt.key, "")
			os.Unsetenv(tt.key)
			path := write(t, tt.content)

			// ===== Act ===== //
			err := loadEnvFile(path)

			// ===== Assert ===== //
			assert.NoError(t, err)
			assert.Equal(t, tt.want, os.Getenv(tt.key))
		})
	}

	t.Run("existing env not overwritten", func(t *testing.T) {
		// ===== Arrange ===== //
		t.Setenv("KEY", "existing")
		path := write(t, "KEY=from_file\n")

		// ===== Act ===== //
		err := loadEnvFile(path)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "existing", os.Getenv("KEY"))
	})

	t.Run("missing file is not an error", func(t *testing.T) {
		// ===== Act ===== //
		err := loadEnvFile("/tmp/nonexistent_wigrate_test.env")

		// ===== Assert ===== //
		assert.NoError(t, err)
	})
}
