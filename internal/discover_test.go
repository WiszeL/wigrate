package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Migration_ParseMigrationFile(t *testing.T) {
	t.Run("parses golang migrate init file", func(t *testing.T) {
		// ===== Arrange ===== //
		path := "/tmp/000001_init_user.up.sql"

		// ===== Act ===== //
		file, ok := parseMigrationFile(path, "user")

		// ===== Assert ===== //
		assert.True(t, ok)
		assert.Equal(t, path, file.path)
		assert.Equal(t, "000001_init_user", file.baseName)
		assert.Equal(t, migrationKindInit, file.kind)
		assert.Equal(t, "up", file.direction)
	})

	t.Run("parses golang migrate alter file", func(t *testing.T) {
		// ===== Arrange ===== //
		path := "/tmp/000002_alter_email_name_user.down.sql"

		// ===== Act ===== //
		file, ok := parseMigrationFile(path, "user")

		// ===== Assert ===== //
		assert.True(t, ok)
		assert.Equal(t, path, file.path)
		assert.Equal(t, "000002_alter_email_name_user", file.baseName)
		assert.Equal(t, migrationKindAlter, file.kind)
		assert.Equal(t, "down", file.direction)
	})

	t.Run("does not match entity name inside another entity suffix", func(t *testing.T) {
		// ===== Arrange ===== //
		path := "/tmp/000001_init_super_user.up.sql"

		// ===== Act ===== //
		_, ok := parseMigrationFile(path, "user")

		// ===== Assert ===== //
		assert.False(t, ok)
	})
}

func Test_Discover_FindModules(t *testing.T) {
	t.Run("discovers multiple modules", func(t *testing.T) {
		// ===== Arrange ===== //
		root := t.TempDir()
		t.Chdir(root)
		assert.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644))
		assert.NoError(t, os.MkdirAll(filepath.Join(root, "module", "iam", "internal", "domain", "entity"), 0755))
		assert.NoError(t, os.MkdirAll(filepath.Join(root, "module", "billing", "internal", "domain", "entity"), 0755))

		// ===== Act ===== //
		modules, err := findModules()

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, modules, 2)
		names := make(map[string]bool)
		for _, m := range modules {
			names[m.name] = true
		}
		assert.True(t, names["iam"])
		assert.True(t, names["billing"])
	})

	t.Run("filters out non-directory entries", func(t *testing.T) {
		// ===== Arrange ===== //
		root := t.TempDir()
		t.Chdir(root)
		assert.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644))
		assert.NoError(t, os.MkdirAll(filepath.Join(root, "module"), 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(root, "module", "file.txt"), []byte(""), 0644))

		// ===== Act ===== //
		modules, err := findModules()

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Empty(t, modules)
	})

	t.Run("returns error when module dir is missing", func(t *testing.T) {
		// ===== Arrange ===== //
		root := t.TempDir()
		t.Chdir(root)
		assert.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644))

		// ===== Act ===== //
		_, err := findModules()

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "module path is not found")
	})
}

func Test_Discover_EntityNameFromFile(t *testing.T) {
	t.Run("trims .go extension", func(t *testing.T) {
		// ===== Arrange ===== //
		// ===== Act ===== //
		result := entityNameFromFile("user.go")
		// ===== Assert ===== //
		assert.Equal(t, "user", result)
	})

	t.Run("trims .go extension with multiple dots", func(t *testing.T) {
		// ===== Arrange ===== //
		// ===== Act ===== //
		result := entityNameFromFile("test.data.go")
		// ===== Assert ===== //
		assert.Equal(t, "test.data", result)
	})

	t.Run("returns empty for no extension", func(t *testing.T) {
		// ===== Arrange ===== //
		// ===== Act ===== //
		result := entityNameFromFile("user")
		// ===== Assert ===== //
		assert.Equal(t, "user", result)
	})
}

func Test_Discover_FindEntityMigrationState(t *testing.T) {
	t.Run("finds single init migration", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte(""), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.down.sql"), []byte(""), 0644))
		module := migrationModule{migrationDir: dir}

		// ===== Act ===== //
		state, err := findEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.NotNil(t, state.latest)
		assert.Equal(t, migrationKindInit, state.latest.kind)
	})

	t.Run("finds latest migration from init+alter", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte(""), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000002_alter_email_user.up.sql"), []byte(""), 0644))
		module := migrationModule{migrationDir: dir}

		// ===== Act ===== //
		state, err := findEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.NotNil(t, state.latest)
		assert.Equal(t, "000002_alter_email_user", state.latest.baseName)
		assert.Equal(t, migrationKindAlter, state.latest.kind)
	})

	t.Run("prefers up file over down when same base", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte(""), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.down.sql"), []byte(""), 0644))
		module := migrationModule{migrationDir: dir}

		// ===== Act ===== //
		state, err := findEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.NotNil(t, state.latest)
		assert.Equal(t, "up", state.latest.direction)
	})

	t.Run("returns empty state when no migrations exist", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		module := migrationModule{migrationDir: dir}

		// ===== Act ===== //
		state, err := findEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Nil(t, state.latest)
	})

	t.Run("ignores files for other entities", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_post.up.sql"), []byte(""), 0644))
		module := migrationModule{migrationDir: dir}

		// ===== Act ===== //
		state, err := findEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Nil(t, state.latest)
	})
}

func Test_Discover_MigrationFilePair(t *testing.T) {
	t.Run("returns up and down file paths", func(t *testing.T) {
		// ===== Arrange ===== //
		file := migrationFile{
			path:     "/tmp/migration/000001_init_user.up.sql",
			baseName: "000001_init_user",
		}

		// ===== Act ===== //
		upPath, downPath := migrationFilePair(file)

		// ===== Assert ===== //
		assert.Equal(t, "/tmp/migration/000001_init_user.up.sql", upPath)
		assert.Equal(t, "/tmp/migration/000001_init_user.down.sql", downPath)
	})
}
