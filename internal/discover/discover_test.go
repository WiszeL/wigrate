package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Migration_ParseMigrationFile(t *testing.T) {
	t.Run("parses golang migrate init file", func(t *testing.T) {
		// ===== Arrange ===== //
		path := "/tmp/000001_init_user.up.sql"

		// ===== Act ===== //
		file, ok := ParseMigrationFile(path, "user")

		// ===== Assert ===== //
		assert.True(t, ok)
		assert.Equal(t, path, file.Path)
		assert.Equal(t, "000001_init_user", file.BaseName)
		assert.Equal(t, KindInit, file.Kind)
		assert.Equal(t, "up", file.Direction)
	})

	t.Run("parses golang migrate alter file", func(t *testing.T) {
		// ===== Arrange ===== //
		path := "/tmp/000002_alter_email_name_user.down.sql"

		// ===== Act ===== //
		file, ok := ParseMigrationFile(path, "user")

		// ===== Assert ===== //
		assert.True(t, ok)
		assert.Equal(t, path, file.Path)
		assert.Equal(t, "000002_alter_email_name_user", file.BaseName)
		assert.Equal(t, KindAlter, file.Kind)
		assert.Equal(t, "down", file.Direction)
	})

	t.Run("does not match entity name inside another entity suffix", func(t *testing.T) {
		// ===== Arrange ===== //
		path := "/tmp/000001_init_super_user.up.sql"

		// ===== Act ===== //
		_, ok := ParseMigrationFile(path, "user")

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
		modules, err := FindModules()

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, modules, 2)
		moduleMap := make(map[string]Module)
		for _, m := range modules {
			moduleMap[m.Name] = m
		}
		assert.True(t, moduleMap["iam"].Name == "iam")
		assert.True(t, moduleMap["billing"].Name == "billing")
		assert.Equal(t, filepath.Join(root, "module", "iam", "migration"), moduleMap["iam"].MigrationDir)
		assert.Equal(t, filepath.Join(root, "module", "iam", "internal", "domain", "entity"), moduleMap["iam"].EntityDir)
		assert.Equal(t, filepath.Join(root, "module", "billing", "migration"), moduleMap["billing"].MigrationDir)
		assert.Equal(t, filepath.Join(root, "module", "billing", "internal", "domain", "entity"), moduleMap["billing"].EntityDir)
	})

	t.Run("filters out non-directory entries", func(t *testing.T) {
		// ===== Arrange ===== //
		root := t.TempDir()
		t.Chdir(root)
		assert.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644))
		assert.NoError(t, os.MkdirAll(filepath.Join(root, "module"), 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(root, "module", "file.txt"), []byte(""), 0644))

		// ===== Act ===== //
		modules, err := FindModules()

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
		_, err := FindModules()

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "module path is not found")
	})
}

func Test_Discover_EntityNameFromFile(t *testing.T) {
	entityNameFromFile := func(goName string) string {
		return strings.TrimSuffix(goName, filepath.Ext(goName))
	}

	t.Run("trims .go extension", func(t *testing.T) {
		assert.Equal(t, "user", entityNameFromFile("user.go"))
	})

	t.Run("trims .go extension with multiple dots", func(t *testing.T) {
		assert.Equal(t, "test.data", entityNameFromFile("test.data.go"))
	})

	t.Run("returns empty for no extension", func(t *testing.T) {
		assert.Equal(t, "user", entityNameFromFile("user"))
	})
}

func Test_Discover_FindEntityMigrationState(t *testing.T) {
	t.Run("finds single init migration", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte(""), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.down.sql"), []byte(""), 0644))
		module := Module{MigrationDir: dir}

		// ===== Act ===== //
		state, err := FindEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.NotNil(t, state)
		assert.Equal(t, KindInit, state.Kind)
		assert.Equal(t, "000001_init_user", state.BaseName)
		assert.Equal(t, "up", state.Direction)
	})

	t.Run("finds latest migration from init+alter", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte(""), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000002_alter_email_user.up.sql"), []byte(""), 0644))
		module := Module{MigrationDir: dir}

		// ===== Act ===== //
		state, err := FindEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.NotNil(t, state)
		assert.Equal(t, "000002_alter_email_user", state.BaseName)
		assert.Equal(t, KindAlter, state.Kind)
		assert.Equal(t, "up", state.Direction)
	})

	t.Run("prefers up file over down when same base", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte(""), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.down.sql"), []byte(""), 0644))
		module := Module{MigrationDir: dir}

		// ===== Act ===== //
		state, err := FindEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.NotNil(t, state)
		assert.Equal(t, "up", state.Direction)
		assert.Equal(t, "000001_init_user", state.BaseName)
		assert.Equal(t, KindInit, state.Kind)
	})

	t.Run("returns empty state when no migrations exist", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		module := Module{MigrationDir: dir}

		// ===== Act ===== //
		state, err := FindEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Nil(t, state)
	})

	t.Run("ignores files for other entities", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_post.up.sql"), []byte(""), 0644))
		module := Module{MigrationDir: dir}

		// ===== Act ===== //
		state, err := FindEntityMigrationState(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Nil(t, state)
	})
}

func Test_Discover_MigrationFilePair(t *testing.T) {
	t.Run("returns up and down file paths", func(t *testing.T) {
		// ===== Arrange ===== //
		file := File{
			Path:     "/tmp/migration/000001_init_user.up.sql",
			BaseName: "000001_init_user",
		}

		// ===== Act ===== //
		upPath, downPath := MigrationFilePair(file)

		// ===== Assert ===== //
		assert.Equal(t, "/tmp/migration/000001_init_user.up.sql", upPath)
		assert.Equal(t, "/tmp/migration/000001_init_user.down.sql", downPath)
	})
}

func Test_Migration_FilterModules(t *testing.T) {
	t.Run("returns all modules when no module is selected", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []Module{
			{Name: "iam"},
			{Name: "billing"},
		}

		// ===== Act ===== //
		filtered, err := FilterModules(modules, "")

		// ===== Assert ===== //
		assert.NoError(t, err)

		assert.Equal(t, modules, filtered)
	})

	t.Run("returns selected module", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []Module{
			{Name: "iam"},
			{Name: "billing"},
		}

		// ===== Act ===== //
		filtered, err := FilterModules(modules, "iam")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []Module{{Name: "iam"}}, filtered)
	})

	t.Run("returns error when module is missing", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []Module{
			{Name: "iam"},
			{Name: "billing"},
		}

		// ===== Act ===== //
		_, err := FilterModules(modules, "catalog")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "module not found: catalog")
	})
}
