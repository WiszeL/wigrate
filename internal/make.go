package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func MakeMigration(overwriteLatest bool, moduleNames ...string) error {
	modules, err := findModules()
	if err != nil {
		return err
	}

	modules, err = filterModules(modules, moduleNames...)
	if err != nil {
		return err
	}

	for _, module := range modules {
		if err := makePerModule(module, overwriteLatest); err != nil {
			return err
		}
	}

	return nil
}

func filterModules(modules []migrationModule, moduleNames ...string) ([]migrationModule, error) {
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

	var filtered []migrationModule
	for _, module := range modules {
		if _, ok := wanted[module.name]; ok {
			filtered = append(filtered, module)
			delete(wanted, module.name)
		}
	}
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
	// Migration folders are generated infrastructure, so create them lazily.
	if err := os.MkdirAll(module.migrationDir, 0755); err != nil {
		return err
	}

	// Entity source is required because migrations are generated from structs.
	if _, err := os.Stat(module.entityDir); os.IsNotExist(err) {
		return fmt.Errorf("entity not found in module %s", module.name)
	}

	entries, err := os.ReadDir(module.entityDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		// Entity discovery is intentionally file-based for now.
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}

		if err := generateMigrationForEntity(module, entry.Name(), overwriteLatest); err != nil {
			return err
		}
	}

	return nil
}

func generateMigrationForEntity(module migrationModule, goName string, overwriteLatest bool) error {
	// Migrations are named after entity files, without the `.go` suffix.
	entityName := entityNameFromFile(goName)
	if entityName == "" {
		return fmt.Errorf("invalid entity file name %s", goName)
	}

	state, err := findEntityMigrationState(module, entityName)
	if err != nil {
		return err
	}

	// No migration history means this entity needs its init migration.
	if state.latest == nil {
		return makeInitMigration(module, entityName)
	}

	// Overwrite is always scoped to the latest migration for this entity.
	if overwriteLatest {
		return overwriteLatestMigration(module, entityName, *state.latest)
	}

	// Existing history plus no overwrite means a new alter migration.
	return makeAlterMigration(module, entityName)
}

func runCommand(cmd string, args ...string) error {
	return runCommandFunc(cmd, args...)
}

var runCommandFunc = func(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return err
	}

	return nil
}
