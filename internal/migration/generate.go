package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wiszel/wigrate/internal"
	"github.com/wiszel/wigrate/internal/diff"
	"github.com/wiszel/wigrate/internal/discover"
	"github.com/wiszel/wigrate/internal/replay"
	"github.com/wiszel/wigrate/internal/schema"
	"github.com/wiszel/wigrate/internal/sqlgen"
)

// createMigration runs migrate create and returns the file pair descriptor.
func createMigration(module discover.Module, entityName string, migrationName string, kind discover.Kind) (discover.File, error) {
	if err := config.RunCommandFunc("migrate", "create", "-ext", "sql", "-dir", module.MigrationDir, "-seq", migrationName); err != nil {
		return discover.File{}, err
	}

	if config.DryRun {
		// ponytail: preview mode, file not written to disk
		return discover.File{
			Path:      filepath.Join(module.MigrationDir, migrationName+".up.sql"),
			BaseName:  migrationName,
			Kind:      kind,
			Direction: "up",
		}, nil
	}

	// Reading created migration file from disk
	entries, err := os.ReadDir(module.MigrationDir)
	if err != nil {
		return discover.File{}, fmt.Errorf("read migration dir: %w", err)
	}
	latest := discover.LatestMigrationFile(module, entries, entityName)
	if latest == nil || latest.Kind != kind {
		return discover.File{}, fmt.Errorf("created %s migration for entity %s not found", kind, entityName)
	}

	return *latest, nil
}

func makeInitMigration(module discover.Module, entityName string) error {
	// Parsing entity schema
	table, err := schema.Parse(module, entityName)
	if err != nil {
		return err
	}

	file, err := createMigration(module, entityName, "init_"+entityName, discover.KindInit)
	if err != nil {
		return err
	}

	return writeMigrationFiles(file, sqlgen.CreateTableSQL(table), sqlgen.DropTableSQL(table))
}

func overwriteLatestMigration(module discover.Module, entries []os.DirEntry, entityName string, latest discover.File) error {
	switch latest.Kind {
	case discover.KindInit:
		table, err := schema.Parse(module, entityName)
		if err != nil {
			return err
		}
		return writeMigrationFiles(latest, sqlgen.CreateTableSQL(table), sqlgen.DropTableSQL(table))
	case discover.KindAlter:
		return overwriteAlterMigration(module, entries, entityName, latest)
	default:
		return fmt.Errorf("unknown migration kind %s", latest.Kind)
	}
}

func makeAlterMigration(module discover.Module, entries []os.DirEntry, entityName string) error {
	// Computing schema diff
	result, err := buildSchemaDiff(module, entries, entityName, nil)
	if err != nil {
		return err
	}

	// Checking for schema changes
	if result.Empty() {
		fmt.Printf("No schema changes detected for entity %s in module %s.\n", entityName, module.Name)
		return nil
	}

	// Creating new migration
	migrationName := alterMigrationName(result.ChangedColumnNames(), entityName)
	fmt.Printf("Create new alter migration for entity %s in module %s.\n", entityName, module.Name)
	file, err := createMigration(module, entityName, migrationName, discover.KindAlter)
	if err != nil {
		return err
	}

	return writeMigrationFiles(file, sqlgen.AlterTableSQL(result), sqlgen.RevertAlterTableSQL(result))
}

func overwriteAlterMigration(module discover.Module, entries []os.DirEntry, entityName string, latest discover.File) error {
	// Recomputing schema diff for latest migration
	result, err := buildSchemaDiff(module, entries, entityName, &latest)
	if err != nil {
		return err
	}
	if result.Empty() {
		fmt.Printf("No schema changes detected for latest alter migration of entity %s in module %s.\n", entityName, module.Name)
		return nil
	}

	return writeMigrationFiles(latest, sqlgen.AlterTableSQL(result), sqlgen.RevertAlterTableSQL(result))
}

func buildSchemaDiff(module discover.Module, entries []os.DirEntry, entityName string, before *discover.File) (diff.Result, error) {
	current, err := schema.Parse(module, entityName)
	if err != nil {
		return diff.Result{}, err
	}

	existing, err := replay.Read(module, entries, entityName, before)
	if err != nil {
		return diff.Result{}, err
	}

	return diff.Compute(existing, current)
}

func writeMigrationFiles(file discover.File, upSQL string, downSQL string) error {
	upPath, downPath := discover.MigrationFilePair(file)

	// Handling dry run
	if config.DryRun {
		fmt.Printf("[dry-run] write %s\n%s\n", upPath, upSQL)
		fmt.Printf("[dry-run] write %s\n%s\n", downPath, downSQL)

		return nil
	}
	// Writing migration files
	if err := os.WriteFile(upPath, []byte(upSQL), 0644); err != nil {
		return err
	}

	return os.WriteFile(downPath, []byte(downSQL), 0644)
}

func alterMigrationName(columns []string, entityName string) string {
	parts := []string{"alter"}
	for i, column := range columns {
		if i == 2 {
			parts = append(parts, "etc")
			break
		}
		parts = append(parts, column)
	}
	parts = append(parts, entityName)

	return strings.Join(parts, "_")
}
