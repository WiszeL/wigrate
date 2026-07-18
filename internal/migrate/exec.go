package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wiszel/wigrate/internal"
	"github.com/wiszel/wigrate/internal/database"
	"github.com/wiszel/wigrate/internal/discover"
)

// runMigrateCommand executes the migrate CLI tool with the given direction and steps.
func runMigrateCommand(module discover.Module, dbConfig database.Config, direction string, steps string) error {
	// Building the migrate args
	args := []string{
		"-path", module.MigrationDir,
		"-database", dbConfig.URLForModule(module),
		direction,
	}
	if steps != "" {
		args = append(args, steps)
	}

	// Executing the migrate command
	if err := config.RunCommandFunc("migrate", args...); err != nil {
		if isNoChangeError(err) {
			return nil
		}

		return err
	}

	return nil
}

// hasMigrationFiles checks if the module has any .sql migration files.
func hasMigrationFiles(module discover.Module) (bool, error) {
	// Reading the migration directory
	entries, err := os.ReadDir(module.MigrationDir)
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

// isNoChangeError checks if the error indicates no migrations were applied.
func isNoChangeError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "no change")
}
