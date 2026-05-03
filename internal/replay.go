package internal

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func readGeneratedSchema(module migrationModule, entityName string, before *migrationFile) (tableSchema, error) {
	files, err := findEntityMigrationFiles(module, entityName)
	if err != nil {
		return tableSchema{}, err
	}

	sort.Slice(files, func(i int, j int) bool {
		return files[i].baseName < files[j].baseName
	})

	// Replay generated up migrations to reconstruct the schema state.
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
			return tableSchema{}, err
		}
		applyGeneratedSQL(&schema, string(content))
	}

	return schema, nil
}

func findEntityMigrationFiles(module migrationModule, entityName string) ([]migrationFile, error) {
	entries, err := os.ReadDir(module.migrationDir)
	if err != nil {
		return nil, err
	}

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

	return files, nil
}

func applyGeneratedSQL(schema *tableSchema, sql string) {
	// This parser only needs to understand SQL emitted by this package.
	for line := range strings.SplitSeq(sql, "\n") {
		line = cleanGeneratedSQLLine(line)
		if shouldSkipGeneratedSQLLine(line) {
			continue
		}

		switch {
		case strings.HasPrefix(line, "ADD CONSTRAINT ") && strings.Contains(line, " UNIQUE "):
			if columnName, ok := parseGeneratedUniqueConstraint(line); ok {
				applyGeneratedUniqueConstraint(schema, columnName, true)
			}
		case strings.HasPrefix(line, "FOREIGN KEY "):
			if fk, ok := parseGeneratedForeignKey(line); ok {
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

	column := columnSchema{name: parts[0]}
	var dataTypeParts []string
	for i := 1; i < len(parts); i++ {
		// Data types may contain spaces, e.g. DOUBLE PRECISION.
		if parts[i] == "PRIMARY" || parts[i] == "NOT" {
			break
		}
		dataTypeParts = append(dataTypeParts, parts[i])
	}
	if len(dataTypeParts) == 0 {
		return columnSchema{}, false
	}
	column.dataType = strings.Join(dataTypeParts, " ")

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
	columnStart := strings.Index(line, "(")
	columnEnd := strings.Index(line, ")")
	_, after, ok := strings.Cut(line, " REFERENCES ")
	if columnStart == -1 || columnEnd == -1 || !ok || columnEnd <= columnStart {
		return foreignKeySchema{}, false
	}

	reference := after
	refTableEnd := strings.Index(reference, "(")
	refColumnEnd := strings.Index(reference, ")")
	if refTableEnd == -1 || refColumnEnd == -1 || refColumnEnd <= refTableEnd {
		return foreignKeySchema{}, false
	}

	foreignKey := foreignKeySchema{
		column:    line[columnStart+1 : columnEnd],
		refTable:  reference[:refTableEnd],
		refColumn: reference[refTableEnd+1 : refColumnEnd],
	}

	if _, after, ok := strings.Cut(line, " ON DELETE "); ok {
		foreignKey.onDelete = after
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
	if _, ok := findColumnIndex(schema.columns, column.name); ok {
		return
	}
	schema.columns = append(schema.columns, column)
}

func removeColumn(schema *tableSchema, columnName string) {
	if index, ok := findColumnIndex(schema.columns, columnName); ok {
		schema.columns = append(schema.columns[:index], schema.columns[index+1:]...)
	}
	removeForeignKey(schema, columnName)
}

func updateColumn(schema *tableSchema, columnName string, update func(*columnSchema)) {
	index, ok := findColumnIndex(schema.columns, columnName)
	if !ok {
		return
	}
	update(&schema.columns[index])
}

func applyGeneratedUniqueConstraint(schema *tableSchema, columnName string, unique bool) {
	updateColumn(schema, columnName, func(column *columnSchema) {
		column.unique = unique
	})
}

func appendForeignKeyIfMissing(schema *tableSchema, foreignKey foreignKeySchema) {
	if _, ok := findForeignKeyIndex(schema.foreignKeys, foreignKey.column); ok {
		return
	}
	schema.foreignKeys = append(schema.foreignKeys, foreignKey)
}

func removeForeignKeyByConstraintName(schema *tableSchema, constraintName string) {
	prefix := "fk_" + schema.name + "_"
	if !strings.HasPrefix(constraintName, prefix) {
		return
	}
	removeForeignKey(schema, strings.TrimPrefix(constraintName, prefix))
}

func removeUniqueByConstraintName(schema *tableSchema, constraintName string) {
	prefix := "uq_" + schema.name + "_"
	if !strings.HasPrefix(constraintName, prefix) {
		return
	}
	applyGeneratedUniqueConstraint(schema, strings.TrimPrefix(constraintName, prefix), false)
}

func removeForeignKey(schema *tableSchema, columnName string) {
	if index, ok := findForeignKeyIndex(schema.foreignKeys, columnName); ok {
		schema.foreignKeys = append(schema.foreignKeys[:index], schema.foreignKeys[index+1:]...)
	}
}

func findColumnIndex(columns []columnSchema, name string) (int, bool) {
	for i, column := range columns {
		if column.name == name {
			return i, true
		}
	}
	return 0, false
}

func findForeignKeyIndex(foreignKeys []foreignKeySchema, columnName string) (int, bool) {
	for i, foreignKey := range foreignKeys {
		if foreignKey.column == columnName {
			return i, true
		}
	}
	return 0, false
}
