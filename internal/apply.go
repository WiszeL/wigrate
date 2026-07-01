package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func MigrateUp(moduleNames ...string) error {
	return migrateModules("up", "", moduleNames...)
}

func MigrateStatus(moduleNames ...string) error {
	return migrateModules("version", "", moduleNames...)
}

func MigrateDown(steps int, moduleNames ...string) error {
	if steps <= 0 {
		return fmt.Errorf("down steps must be greater than zero")
	}

	return migrateModules("down", strconv.Itoa(steps), moduleNames...)
}

func migrateModules(direction string, steps string, moduleNames ...string) error {
	// Finding the project root
	root, err := FindRoot()
	if err != nil {
		return err
	}

	// Loading the database config
	config, err := loadDatabaseConfig(root)
	if err != nil {
		return err
	}

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

	for _, module := range modules {
		// Running the migration
		if ok, err := hasMigrationFiles(module); err != nil {
			return err
		} else if !ok {
			fmt.Printf("No migration files found for module %s.\n", module.name)
			continue
		}

		if err := runMigrateCommand(module, config, direction, steps); err != nil {
			return err
		}
	}

	return nil
}

func runMigrateCommand(module migrationModule, config databaseConfig, direction string, steps string) error {
	// Building the migrate args
	args := []string{
		"-path", module.migrationDir,
		"-database", config.urlForModule(module),
		direction,
	}
	if steps != "" {
		args = append(args, steps)
	}

	// Executing the migrate command
	if err := runCommandFunc("migrate", args...); err != nil {
		if isNoChangeError(err) {
			return nil
		}

		return err
	}

	return nil
}

func hasMigrationFiles(module migrationModule) (bool, error) {
	// Reading the migration directory
	entries, err := os.ReadDir(module.migrationDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read migration dir: %w", err)
	}

	// Scanning for .sql files
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext == ".sql" {
			return true, nil
		}
	}

	return false, nil
}

func isNoChangeError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "no change")
}
