// Package replay reads migration files and reconstructs the current schema.
package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wiszel/wigrate/internal/discover"
	"github.com/wiszel/wigrate/internal/schema"
)

func Read(module discover.Module, entries []os.DirEntry, entityName string, before *discover.File) (schema.Table, error) {
	files := findEntityMigrationFiles(module, entries, entityName)

	// Sorting migration files by name
	sort.Slice(files, func(i int, j int) bool {
		return files[i].BaseName < files[j].BaseName
	})

	// Replaying up migration files to reconstruct schema state
	table := schema.Table{Name: schema.TableNameFromEntity(entityName)}
	for _, file := range files {
		if file.Direction != "up" {
			continue
		}
		if before != nil && file.BaseName >= before.BaseName {
			continue
		}

		content, err := os.ReadFile(file.Path)
		if err != nil {
			return schema.Table{}, fmt.Errorf("read migration file %s: %w", file.Path, err)
		}
		applyGeneratedSQL(&table, string(content))
	}

	return table, nil
}

// Finding migration files for an entity.
func findEntityMigrationFiles(module discover.Module, entries []os.DirEntry, entityName string) []discover.File {
	var files []discover.File
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		file, ok := discover.ParseMigrationFile(filepath.Join(module.MigrationDir, entry.Name()), entityName)
		if ok {
			files = append(files, *file)
		}
	}

	return files
}

func applyGeneratedSQL(table *schema.Table, sql string) {
	// Cleaning and skipping blank/DDL lines
	for line := range strings.SplitSeq(sql, "\n") {
		line = cleanGeneratedSQLLine(line)
		if shouldSkipGeneratedSQLLine(line) {
			continue
		}

		// Dispatching by SQL prefix
		switch {
		case strings.HasPrefix(line, "PRIMARY KEY ("):
			if cols, ok := parseGeneratedColumnList(strings.TrimPrefix(line, "PRIMARY KEY ")); ok {
				table.PrimaryKey = cols
			}
		case isConstraintLine(line) && strings.Contains(line, " UNIQUE "):
			if cols, ok := parseGeneratedUniqueConstraint(line); ok {
				applyGeneratedUniqueConstraint(table, cols)
			}
		case strings.HasPrefix(line, "CONSTRAINT ") && strings.Contains(line, " FOREIGN KEY "):
			if fk, ok := parseGeneratedForeignKey(line); ok {
				appendForeignKeyIfMissing(table, fk)
			}
		case isConstraintLine(line) && strings.Contains(line, " CHECK ("):
			if col, body, ok := parseGeneratedCheckConstraint(line); ok {
				applyGeneratedCheckConstraint(table, col, body)
			}
		case strings.HasPrefix(line, "ADD CONSTRAINT "):
			if fk, ok := parseGeneratedForeignKey(line); ok {
				appendForeignKeyIfMissing(table, fk)
			}
		case strings.HasPrefix(line, "DROP CONSTRAINT IF EXISTS "):
			constraintName := strings.TrimPrefix(line, "DROP CONSTRAINT IF EXISTS ")
			removeForeignKeyByConstraintName(table, constraintName)
			removeUniqueByConstraintName(table, constraintName)
			clearCheckByConstraintName(table, constraintName)
		case strings.HasPrefix(line, "CREATE INDEX ") && strings.Contains(line, "USING GIN"):
			if col, ok := parseGeneratedTrgmIndex(line); ok {
				applyGeneratedTrgmIndex(table, col)
			}
		case strings.HasPrefix(line, "CREATE INDEX "):
			if cols, ok := parseGeneratedIndex(line); ok {
				applyGeneratedIndex(table, cols)
			}
		case strings.HasPrefix(line, "DROP INDEX IF EXISTS "):
			name := strings.TrimPrefix(line, "DROP INDEX IF EXISTS ")
			removeIndexByName(table, name)
			removeTrgmIndexByName(table, name)
		case strings.HasPrefix(line, "ADD COLUMN "):
			if column, ok := parseGeneratedColumn(strings.TrimPrefix(line, "ADD COLUMN ")); ok {
				appendColumnIfMissing(table, column)
			}
		case strings.HasPrefix(line, "DROP COLUMN IF EXISTS "):
			removeColumn(table, strings.TrimPrefix(line, "DROP COLUMN IF EXISTS "))
		case strings.HasPrefix(line, "ALTER COLUMN "):
			applyGeneratedColumnAlter(table, strings.TrimPrefix(line, "ALTER COLUMN "))
		default:
			if column, ok := parseGeneratedColumn(line); ok {
				appendColumnIfMissing(table, column)
			}
		}
	}
}

// A constraint line inside CREATE TABLE reads "CONSTRAINT ...", inside ALTER
// TABLE it reads "ADD CONSTRAINT ..." — both forms carry UNIQUE/CHECK/FOREIGN
// KEY constraints identically, only the prefix differs.
func isConstraintLine(line string) bool {
	return strings.HasPrefix(line, "CONSTRAINT ") || strings.HasPrefix(line, "ADD CONSTRAINT ")
}

// Stripping whitespace and trailing punctuation.
func cleanGeneratedSQLLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ",")
	line = strings.TrimSuffix(line, ";")

	return line
}

// Checking if a line should be skipped (blank, DDL wrapper, closing paren).
func shouldSkipGeneratedSQLLine(line string) bool {
	return line == "" ||
		strings.HasPrefix(line, "CREATE TABLE ") ||
		strings.HasPrefix(line, "ALTER TABLE ") ||
		strings.HasPrefix(line, "CREATE EXTENSION ") ||
		line == ")"
}
