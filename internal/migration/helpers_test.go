package migration

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wiszel/wigrate/internal"
	"github.com/wiszel/wigrate/internal/discover"
)

func makeTestMigrationModule(t *testing.T, moduleName string, entityFile string, entitySource string) discover.Module {
	t.Helper()

	root := t.TempDir()
	entityDir := filepath.Join(root, "module", moduleName, "internal", "domain", "entity")
	migrationDir := filepath.Join(root, "module", moduleName, "migration")
	assert.NoError(t, os.MkdirAll(entityDir, 0755))
	assert.NoError(t, os.MkdirAll(migrationDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(entityDir, entityFile), []byte(entitySource), 0644))

	return discover.Module{
		Name:         moduleName,
		EntityDir:    entityDir,
		MigrationDir: migrationDir,
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

func stubDryRun(t *testing.T, value bool) func() {
	t.Helper()

	original := config.DryRun
	config.DryRun = value

	return func() {
		config.DryRun = original
	}
}
