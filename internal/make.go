package internal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func MakeMigration(overwriteLatest bool, moduleNames ...string) error {
	// Discovering the modules
	modules, err := findModules()
	if err != nil {
		return err
	}

	// Filtering the modules
	modules, err = filterModules(modules, moduleNames...)
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

func filterModules(modules []migrationModule, moduleNames ...string) ([]migrationModule, error) {
	// Building the wanted set from provided names
	// Empty module names mean the generator should run for every module.
	wanted := make(map[string]struct{})
	for _, moduleName := range moduleNames {
		moduleName = strings.TrimSpace(moduleName)
		if moduleName == "" {
			continue
		}
		wanted[moduleName] = struct{}{}
	}
	if len(wanted) == 0 {
		return modules, nil
	}

	// Filtering modules against the wanted set
	var filtered []migrationModule
	for _, module := range modules {
		if _, ok := wanted[module.name]; ok {
			filtered = append(filtered, module)
			delete(wanted, module.name)
		}
	}
	// Reporting modules not found
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for moduleName := range wanted {
			missing = append(missing, moduleName)
		}

		return nil, fmt.Errorf("module not found: %s", strings.Join(missing, ", "))
	}

	return filtered, nil
}

func makePerModule(module migrationModule, overwriteLatest bool) error {
	// Creating migration directory
	// Migration folders are generated infrastructure, so create them lazily.
	if err := os.MkdirAll(module.migrationDir, 0755); err != nil {
		return err
	}

	// Verifying entity source directory
	// Entity source is required because migrations are generated from structs.
	_, err := os.Stat(module.entityDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("entity not found in module %s", module.name)
	}
	if err != nil {
		return fmt.Errorf("stat entity dir: %w", err)
	}

	// Scan migration dir once; pass to each entity to avoid O(E²) repeated ReadDir.
	migrationEntries, err := os.ReadDir(module.migrationDir)
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}

	// Reading entity files
	entityEntries, err := os.ReadDir(module.entityDir)
	if err != nil {
		return fmt.Errorf("read entity dir: %w", err)
	}

	// Entities not backed by this Postgres schema (e.g. Redis-only) opt out via
	// migration/.wigrateignore — kept infra-side so the domain entity stays clean.
	ignore, err := loadIgnoreSet(module.migrationDir)
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

		if err := generateMigrationForEntity(module, migrationEntries, entry.Name(), overwriteLatest); err != nil {
			return err
		}
	}

	return nil
}

// loadIgnoreSet reads migration/.wigrateignore — one entity name per line, blank
// lines and #-comments skipped — into a lookup set. Missing file is not an error.
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

func generateMigrationForEntity(module migrationModule, entries []os.DirEntry, goName string, overwriteLatest bool) error {
	// Extracting entity name from file
	// Migrations are named after entity files, without the `.go` suffix.
	entityName := strings.TrimSuffix(goName, filepath.Ext(goName))
	if entityName == "" {
		return fmt.Errorf("invalid entity file name %s", goName)
	}

	latest := latestMigrationFile(module, entries, entityName)

	// No migration history means this entity needs its init migration.
	if latest == nil {
		return makeInitMigration(module, entityName)
	}

	// Overwrite is always scoped to the latest migration for this entity.
	if overwriteLatest {
		return overwriteLatestMigration(module, entries, entityName, *latest)
	}

	// Existing history plus no overwrite means a new alter migration.
	return makeAlterMigration(module, entries, entityName)
}

func isGoEntityFile(name string) bool {
	return filepath.Ext(name) == ".go" && !strings.HasSuffix(name, "_test.go")
}

var runCommandFunc = func(cmd string, args ...string) error {
	if DryRun {
		fmt.Printf("[dry-run] %s %s\n", cmd, strings.Join(args, " "))
		return nil
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("%s not found in PATH — install golang-migrate: https://github.com/golang-migrate/migrate", cmd)
	}
	command := exec.Command(cmd, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
