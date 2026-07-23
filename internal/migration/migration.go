// Package migration generates SQL migration files from entity structs.
// It discovers modules, parses entities, diffs against migration history, and writes SQL.
package migration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wiszel/wigrate/internal/discover"
	"github.com/wiszel/wigrate/internal/schema"
)

func Make(overwriteLatest bool, moduleNames ...string) error {
	// Discovering the modules
	modules, err := discover.FindModules()
	if err != nil {
		return err
	}

	// Filtering the modules
	modules, err = discover.FilterModules(modules, moduleNames...)
	if err != nil {
		return err
	}

	// Making migration per module
	for _, module := range modules {
		if err := makePerModule(module, overwriteLatest); err != nil {
			return err
		}
	}

	return nil
}

func makePerModule(module discover.Module, overwriteLatest bool) error {
	// Creating migration directory
	if err := os.MkdirAll(module.MigrationDir, 0755); err != nil {
		return err
	}

	// Verifying entity source directory
	_, err := os.Stat(module.EntityDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("entity not found in module %s", module.Name)
	}
	if err != nil {
		return fmt.Errorf("stat entity dir: %w", err)
	}

	// Reading migration files
	migrationEntries, err := os.ReadDir(module.MigrationDir)
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}

	// Reading entity files
	entityEntries, err := os.ReadDir(module.EntityDir)
	if err != nil {
		return fmt.Errorf("read entity dir: %w", err)
	}

	// Loading ignore list from .wigrateignore
	ignore, err := loadIgnoreSet(module.MigrationDir)
	if err != nil {
		return err
	}

	// Generating migration per entity
	for _, entry := range entityEntries {
		if !isGoEntityFile(entry.Name()) {
			continue
		}

		entityName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, skip := ignore[entityName]; skip {
			continue
		}

		// Skipping support files (e.g. an enum's type+const block) that declare no matching struct
		isEntity, err := schema.IsEntityFile(module, entityName)
		if err != nil {
			return err
		}
		if !isEntity {
			continue
		}

		if err := generateMigrationForEntity(module, migrationEntries, entry.Name(), overwriteLatest); err != nil {
			return err
		}
	}

	return nil
}

// loadIgnoreSet reads .wigrateignore and returns entity names to skip.
func loadIgnoreSet(migrationDir string) (map[string]struct{}, error) {
	data, err := os.ReadFile(filepath.Join(migrationDir, ".wigrateignore"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read .wigrateignore: %w", err)
	}

	set := make(map[string]struct{})
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = struct{}{}
	}
	return set, nil
}

func generateMigrationForEntity(module discover.Module, entries []os.DirEntry, goName string, overwriteLatest bool) error {
	// Extracting entity name from file
	entityName := strings.TrimSuffix(goName, filepath.Ext(goName))
	if entityName == "" {
		return fmt.Errorf("invalid entity file name %s", goName)
	}

	latest := discover.LatestMigrationFile(module, entries, entityName)

	// Creating init migration for new entities
	if latest == nil {
		return makeInitMigration(module, entityName)
	}

	// Overwriting latest migration if requested
	if overwriteLatest {
		return overwriteLatestMigration(module, entries, entityName, *latest)
	}

	// Creating alter migration for existing entities
	return makeAlterMigration(module, entries, entityName)
}

func isGoEntityFile(name string) bool {
	return filepath.Ext(name) == ".go" && !strings.HasSuffix(name, "_test.go")
}
