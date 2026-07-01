package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

func readGeneratedSchema(module migrationModule, entries []os.DirEntry, entityName string, before *migrationFile) (tableSchema, error) {
	files := findEntityMigrationFiles(module, entries, entityName)

	// Sorting migration files by name
	sort.Slice(files, func(i int, j int) bool {
		return files[i].baseName < files[j].baseName
	})

	// Replaying up migration files to reconstruct schema state
	schema := tableSchema{name: tableNameFromEntity(entityName)}
	for _, file := range files {
		if file.direction != "up" {
			continue
		}
		if before != nil && file.baseName >= before.baseName {
			continue
		}

		content, err := os.ReadFile(file.path)
		if err != nil {
			return tableSchema{}, fmt.Errorf("read migration file %s: %w", file.path, err)
		}
		applyGeneratedSQL(&schema, string(content))
	}

	return schema, nil
}

func findEntityMigrationFiles(module migrationModule, entries []os.DirEntry, entityName string) []migrationFile {
	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		file, ok := parseMigrationFile(filepath.Join(module.migrationDir, entry.Name()), entityName)
		if ok {
			files = append(files, *file)
		}
	}

	return files
}

func applyGeneratedSQL(schema *tableSchema, sql string) {
	// This parser only needs to understand SQL emitted by this package
	// Cleaning and skipping blank/DDL lines
	for line := range strings.SplitSeq(sql, "\n") {
		line = cleanGeneratedSQLLine(line)
		if shouldSkipGeneratedSQLLine(line) {
			continue
		}

		// Dispatching by SQL prefix
		switch {
		case strings.HasPrefix(line, "ADD CONSTRAINT ") && strings.Contains(line, " UNIQUE "):
			if columnName, ok := parseGeneratedUniqueConstraint(line); ok {
				applyGeneratedUniqueConstraint(schema, columnName)
			}
		case strings.HasPrefix(line, "CONSTRAINT ") && strings.Contains(line, " FOREIGN KEY "):
			if fk, ok := parseGeneratedAlterForeignKey(line); ok {
				appendForeignKeyIfMissing(schema, fk)
			}
		case strings.HasPrefix(line, "ADD CONSTRAINT "):
			if fk, ok := parseGeneratedAlterForeignKey(line); ok {
				appendForeignKeyIfMissing(schema, fk)
			}
		case strings.HasPrefix(line, "DROP CONSTRAINT IF EXISTS "):
			constraintName := strings.TrimPrefix(line, "DROP CONSTRAINT IF EXISTS ")
			removeForeignKeyByConstraintName(schema, constraintName)
			removeUniqueByConstraintName(schema, constraintName)
		case strings.HasPrefix(line, "ADD COLUMN "):
			if column, ok := parseGeneratedColumn(strings.TrimPrefix(line, "ADD COLUMN ")); ok {
				appendColumnIfMissing(schema, column)
			}
		case strings.HasPrefix(line, "DROP COLUMN IF EXISTS "):
			removeColumn(schema, strings.TrimPrefix(line, "DROP COLUMN IF EXISTS "))
		case strings.HasPrefix(line, "ALTER COLUMN "):
			applyGeneratedColumnAlter(schema, strings.TrimPrefix(line, "ALTER COLUMN "))
		default:
			if column, ok := parseGeneratedColumn(line); ok {
				appendColumnIfMissing(schema, column)
			}
		}
	}
}

func cleanGeneratedSQLLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ",")
	line = strings.TrimSuffix(line, ";")

	return line
}

func shouldSkipGeneratedSQLLine(line string) bool {
	return line == "" ||
		strings.HasPrefix(line, "CREATE TABLE ") ||
		strings.HasPrefix(line, "ALTER TABLE ") ||
		line == ")"
}

func parseGeneratedColumn(line string) (columnSchema, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return columnSchema{}, false
	}
	if parts[0] == "FOREIGN" || parts[0] == "CONSTRAINT" {
		return columnSchema{}, false
	}

	// Extracting column name
	column := columnSchema{name: parts[0]}

	// Extracting data type (may contain spaces, e.g. DOUBLE PRECISION)
	var dataTypeParts []string
	for i := 1; i < len(parts); i++ {
		if parts[i] == "PRIMARY" || parts[i] == "NOT" || parts[i] == "UNIQUE" {
			break
		}
		dataTypeParts = append(dataTypeParts, parts[i])
	}
	if len(dataTypeParts) == 0 {
		return columnSchema{}, false
	}
	column.dataType = strings.Join(dataTypeParts, " ")

	// Parsing constraints
	for i := 1 + len(dataTypeParts); i < len(parts); i++ {
		switch parts[i] {
		case "PRIMARY":
			column.primary = true
		case "NOT":
			if i+1 < len(parts) && parts[i+1] == "NULL" {
				column.notNull = true
			}
		case "UNIQUE":
			column.unique = true
		}
	}

	return column, true
}

func parseGeneratedUniqueConstraint(line string) (string, bool) {
	parts := strings.Fields(line)
	if len(parts) != 5 || parts[0] != "ADD" || parts[1] != "CONSTRAINT" || parts[3] != "UNIQUE" {
		return "", false
	}

	column := strings.TrimPrefix(parts[4], "(")
	column = strings.TrimSuffix(column, ")")
	if column == "" {
		return "", false
	}

	return column, true
}

func parseGeneratedAlterForeignKey(line string) (foreignKeySchema, bool) {
	parts := strings.SplitN(line, " FOREIGN KEY ", 2)
	if len(parts) != 2 {
		return foreignKeySchema{}, false
	}

	return parseGeneratedForeignKey("FOREIGN KEY " + parts[1])
}

func parseGeneratedForeignKey(line string) (foreignKeySchema, bool) {
	// Extracting referencing column
	columnStart := strings.Index(line, "(")
	columnEnd := strings.Index(line, ")")
	_, after, ok := strings.Cut(line, " REFERENCES ")
	if columnStart == -1 || columnEnd == -1 || !ok || columnEnd <= columnStart {
		return foreignKeySchema{}, false
	}

	// Extracting referenced table and column
	reference := after
	refTableEnd := strings.Index(reference, "(")
	refColumnEnd := strings.Index(reference, ")")
	if refTableEnd == -1 || refColumnEnd == -1 || refColumnEnd <= refTableEnd {
		return foreignKeySchema{}, false
	}

	foreignKey := foreignKeySchema{
		column:    strings.TrimSpace(line[columnStart+1 : columnEnd]),
		refTable:  strings.TrimSpace(reference[:refTableEnd]),
		refColumn: strings.TrimSpace(reference[refTableEnd+1 : refColumnEnd]),
	}

	// Extracting ON DELETE action
	if _, after, ok := strings.Cut(line, " ON DELETE "); ok {
		foreignKey.onDelete = strings.TrimSpace(after)
	}

	return foreignKey, true
}

func applyGeneratedColumnAlter(schema *tableSchema, line string) {
	columnName, rest, ok := strings.Cut(line, " ")
	if !ok {
		return
	}

	switch {
	case strings.HasPrefix(rest, "TYPE "):
		updateColumn(schema, columnName, func(column *columnSchema) {
			column.dataType = strings.TrimPrefix(rest, "TYPE ")
		})
	case rest == "SET NOT NULL":
		updateColumn(schema, columnName, func(column *columnSchema) {
			column.notNull = true
		})
	case rest == "DROP NOT NULL":
		updateColumn(schema, columnName, func(column *columnSchema) {
			column.notNull = false
		})
	}
}

func appendColumnIfMissing(schema *tableSchema, column columnSchema) {
	if slices.IndexFunc(schema.columns, func(c columnSchema) bool { return c.name == column.name }) >= 0 {
		return
	}
	schema.columns = append(schema.columns, column)
}

func removeColumn(schema *tableSchema, columnName string) {
	if i := slices.IndexFunc(schema.columns, func(c columnSchema) bool { return c.name == columnName }); i >= 0 {
		schema.columns = slices.Delete(schema.columns, i, i+1)
	}
	removeForeignKey(schema, columnName)
}

func updateColumn(schema *tableSchema, columnName string, update func(*columnSchema)) {
	i := slices.IndexFunc(schema.columns, func(c columnSchema) bool { return c.name == columnName })
	if i < 0 {
		return
	}
	update(&schema.columns[i])
}

func applyGeneratedUniqueConstraint(schema *tableSchema, columnName string) {
	updateColumn(schema, columnName, func(column *columnSchema) {
		column.unique = true
	})
}

func appendForeignKeyIfMissing(schema *tableSchema, foreignKey foreignKeySchema) {
	if slices.IndexFunc(schema.foreignKeys, func(fk foreignKeySchema) bool { return fk.column == foreignKey.column }) >= 0 {
		return
	}
	schema.foreignKeys = append(schema.foreignKeys, foreignKey)
}

func removeForeignKeyByConstraintName(schema *tableSchema, constraintName string) {
	i := slices.IndexFunc(schema.foreignKeys, func(fk foreignKeySchema) bool {
		return foreignKeyConstraintName(schema.name, fk.column) == constraintName
	})
	if i >= 0 {
		schema.foreignKeys = slices.Delete(schema.foreignKeys, i, i+1)
	}
}

func removeUniqueByConstraintName(schema *tableSchema, constraintName string) {
	i := slices.IndexFunc(schema.columns, func(col columnSchema) bool {
		return uniqueConstraintName(schema.name, col.name) == constraintName
	})
	if i >= 0 {
		schema.columns[i].unique = false
	}
}

func removeForeignKey(schema *tableSchema, columnName string) {
	if i := slices.IndexFunc(schema.foreignKeys, func(fk foreignKeySchema) bool { return fk.column == columnName }); i >= 0 {
		schema.foreignKeys = slices.Delete(schema.foreignKeys, i, i+1)
	}
}
