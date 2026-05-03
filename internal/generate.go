package internal

import (
	"fmt"
	"os"
	"strings"
)

func makeInitMigration(module migrationModule, entityName string) error {
	schema, err := parseEntitySchema(module, entityName)
	if err != nil {
		return err
	}

	if err := runCommand("migrate", "create", "-ext", "sql", "-dir", module.migrationDir, "-seq", "init_"+entityName); err != nil {
		return err
	}

	// Rediscover the file pair instead of depending on migrate CLI output.
	state, err := findEntityMigrationState(module, entityName)
	if err != nil {
		return err
	}
	if state.latest == nil || state.latest.kind != migrationKindInit {
		return fmt.Errorf("created init migration for entity %s not found", entityName)
	}

	return writeInitMigrationFiles(schema, *state.latest)
}

func overwriteLatestMigration(module migrationModule, entityName string, latest migrationFile) error {
	switch latest.kind {
	case migrationKindInit:
		return overwriteInitMigration(module, entityName, latest)
	case migrationKindAlter:
		return overwriteAlterMigration(module, entityName, latest)
	default:
		return fmt.Errorf("unknown migration kind %s", latest.kind)
	}
}

func overwriteInitMigration(module migrationModule, entityName string, latest migrationFile) error {
	schema, err := parseEntitySchema(module, entityName)
	if err != nil {
		return err
	}

	return writeInitMigrationFiles(schema, latest)
}

func writeInitMigrationFiles(schema tableSchema, file migrationFile) error {
	return writeMigrationFiles(file, buildCreateTableSQL(schema), buildDropTableSQL(schema))
}

func makeAlterMigration(module migrationModule, entityName string) error {
	// Alter migrations are generated from the diff between migration history and current entity code.
	diff, err := buildSchemaDiff(module, entityName, nil)
	if err != nil {
		return err
	}
	if diff.empty() {
		fmt.Printf("No schema changes detected for entity %s in module %s.\n", entityName, module.name)
		return nil
	}

	migrationName := alterMigrationName(diff.changedColumnNames(), entityName)
	fmt.Printf("Create new alter migration for entity %s in module %s.\n", entityName, module.name)
	if err := runCommand("migrate", "create", "-ext", "sql", "-dir", module.migrationDir, "-seq", migrationName); err != nil {
		return err
	}

	state, err := findEntityMigrationState(module, entityName)
	if err != nil {
		return err
	}
	if state.latest == nil || state.latest.kind != migrationKindAlter {
		return fmt.Errorf("created alter migration for entity %s not found", entityName)
	}

	return writeAlterMigrationFiles(diff, *state.latest)
}

func overwriteAlterMigration(module migrationModule, entityName string, latest migrationFile) error {
	// Rebuild only the latest alter by replaying migration history before it.
	diff, err := buildSchemaDiff(module, entityName, &latest)
	if err != nil {
		return err
	}
	if diff.empty() {
		fmt.Printf("No schema changes detected for latest alter migration of entity %s in module %s.\n", entityName, module.name)
		return nil
	}

	return writeAlterMigrationFiles(diff, latest)
}

func buildSchemaDiff(module migrationModule, entityName string, before *migrationFile) (schemaDiff, error) {
	current, err := parseEntitySchema(module, entityName)
	if err != nil {
		return schemaDiff{}, err
	}

	existing, err := readGeneratedSchema(module, entityName, before)
	if err != nil {
		return schemaDiff{}, err
	}

	return diffSchema(existing, current)
}

func writeAlterMigrationFiles(diff schemaDiff, file migrationFile) error {
	return writeMigrationFiles(file, buildAlterTableSQL(diff), buildRevertAlterTableSQL(diff))
}

func writeMigrationFiles(file migrationFile, upSQL string, downSQL string) error {
	upPath, downPath := migrationFilePair(file)
	if err := os.WriteFile(upPath, []byte(upSQL), 0644); err != nil {
		return err
	}
	return os.WriteFile(downPath, []byte(downSQL), 0644)
}

func buildCreateTableSQL(schema tableSchema) string {
	lines := make([]string, 0, len(schema.columns)+len(schema.foreignKeys))
	for _, column := range schema.columns {
		lines = append(lines, "    "+buildColumnDefinition(column))
	}

	for _, foreignKey := range schema.foreignKeys {
		lines = append(lines, "    "+buildCreateForeignKeyDefinition(foreignKey))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n);\n", schema.name, strings.Join(lines, ",\n"))
}

func buildDropTableSQL(schema tableSchema) string {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", schema.name)
}

func buildAlterTableSQL(diff schemaDiff) string {
	return fmt.Sprintf("ALTER TABLE %s\n%s;\n", diff.tableName, strings.Join(buildAlterUpLines(diff), ",\n"))
}

func buildRevertAlterTableSQL(diff schemaDiff) string {
	return fmt.Sprintf("ALTER TABLE %s\n%s;\n", diff.tableName, strings.Join(buildAlterDownLines(diff), ",\n"))
}

func buildAlterUpLines(diff schemaDiff) []string {
	lines := make([]string, 0, diff.operationCount())

	// Drop constraints before touching dependent columns.
	for _, foreignKey := range diff.removedForeignKeys {
		lines = append(lines, dropForeignKeyLine(diff.tableName, foreignKey))
	}
	for _, change := range diff.changedForeignKeys {
		lines = append(lines, dropForeignKeyLine(diff.tableName, change.before))
	}
	for _, column := range diff.removedColumns {
		lines = append(lines, dropColumnLine(column))
	}
	// Column changes stay between removals and additions for predictable diffs.
	for _, change := range diff.changedColumns {
		lines = append(lines, buildAlterColumnChangeLines(diff.tableName, change.before, change.after)...)
	}
	for _, column := range diff.addedColumns {
		lines = append(lines, addColumnLine(column))
	}
	// Add constraints after their columns are guaranteed to exist.
	for _, foreignKey := range diff.addedForeignKeys {
		lines = append(lines, addForeignKeyLine(diff.tableName, foreignKey))
	}
	for _, change := range diff.changedForeignKeys {
		lines = append(lines, addForeignKeyLine(diff.tableName, change.after))
	}

	return lines
}

func buildAlterDownLines(diff schemaDiff) []string {
	lines := make([]string, 0, diff.operationCount())

	// Down migrations reverse up operations in dependency-safe order.
	for i := len(diff.changedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, dropForeignKeyLine(diff.tableName, diff.changedForeignKeys[i].after))
	}
	for i := len(diff.addedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, dropForeignKeyLine(diff.tableName, diff.addedForeignKeys[i]))
	}
	for i := len(diff.addedColumns) - 1; i >= 0; i-- {
		lines = append(lines, dropColumnLine(diff.addedColumns[i]))
	}
	for i := len(diff.changedColumns) - 1; i >= 0; i-- {
		change := diff.changedColumns[i]
		lines = append(lines, buildAlterColumnChangeLines(diff.tableName, change.after, change.before)...)
	}
	for i := len(diff.removedColumns) - 1; i >= 0; i-- {
		lines = append(lines, addColumnLine(diff.removedColumns[i]))
	}
	for i := len(diff.changedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, addForeignKeyLine(diff.tableName, diff.changedForeignKeys[i].before))
	}
	for i := len(diff.removedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, addForeignKeyLine(diff.tableName, diff.removedForeignKeys[i]))
	}

	return lines
}

func buildAlterColumnChangeLines(tableName string, before columnSchema, after columnSchema) []string {
	var lines []string
	if before.unique && !after.unique {
		lines = append(lines, dropUniqueLine(tableName, before))
	}
	if before.dataType != after.dataType {
		lines = append(lines, buildAlterColumnTypeLine(after.name, after.dataType))
	}
	lines = append(lines, buildAlterColumnNullLine(before, after)...)
	if !before.unique && after.unique {
		lines = append(lines, addUniqueLine(tableName, after))
	}
	return lines
}

func addColumnLine(column columnSchema) string {
	return "    ADD COLUMN " + buildColumnDefinition(column)
}

func dropColumnLine(column columnSchema) string {
	return "    DROP COLUMN IF EXISTS " + column.name
}

func addForeignKeyLine(tableName string, foreignKey foreignKeySchema) string {
	return "    ADD CONSTRAINT " + buildForeignKeyDefinition(tableName, foreignKey)
}

func dropForeignKeyLine(tableName string, foreignKey foreignKeySchema) string {
	return "    DROP CONSTRAINT IF EXISTS " + foreignKeyConstraintName(tableName, foreignKey.column)
}

func addUniqueLine(tableName string, column columnSchema) string {
	return "    ADD CONSTRAINT " + buildUniqueConstraintDefinition(tableName, column.name)
}

func dropUniqueLine(tableName string, column columnSchema) string {
	return "    DROP CONSTRAINT IF EXISTS " + uniqueConstraintName(tableName, column.name)
}

func buildColumnDefinition(column columnSchema) string {
	parts := []string{column.name, column.dataType}
	if column.primary {
		parts = append(parts, "PRIMARY KEY")
	} else if column.notNull {
		parts = append(parts, "NOT NULL")
	}
	if column.unique {
		parts = append(parts, "UNIQUE")
	}

	return strings.Join(parts, " ")
}

func buildCreateForeignKeyDefinition(foreignKey foreignKeySchema) string {
	line := fmt.Sprintf(
		"FOREIGN KEY (%s) REFERENCES %s(%s)",
		foreignKey.column,
		foreignKey.refTable,
		foreignKey.refColumn,
	)
	if foreignKey.onDelete != "" {
		line += " ON DELETE " + foreignKey.onDelete
	}
	return line
}

func buildForeignKeyDefinition(tableName string, foreignKey foreignKeySchema) string {
	line := fmt.Sprintf(
		"%s FOREIGN KEY (%s) REFERENCES %s(%s)",
		foreignKeyConstraintName(tableName, foreignKey.column),
		foreignKey.column,
		foreignKey.refTable,
		foreignKey.refColumn,
	)
	if foreignKey.onDelete != "" {
		line += " ON DELETE " + foreignKey.onDelete
	}
	return line
}

func buildAlterColumnTypeLine(columnName string, dataType string) string {
	return fmt.Sprintf("    ALTER COLUMN %s TYPE %s", columnName, dataType)
}

func buildAlterColumnNullLine(before columnSchema, after columnSchema) []string {
	switch {
	case !before.notNull && after.notNull:
		return []string{"    ALTER COLUMN " + after.name + " SET NOT NULL"}
	case before.notNull && !after.notNull:
		return []string{"    ALTER COLUMN " + after.name + " DROP NOT NULL"}
	default:
		return nil
	}
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

func foreignKeyConstraintName(tableName string, columnName string) string {
	return "fk_" + tableName + "_" + columnName
}

func buildUniqueConstraintDefinition(tableName string, columnName string) string {
	return uniqueConstraintName(tableName, columnName) + " UNIQUE (" + columnName + ")"
}

func uniqueConstraintName(tableName string, columnName string) string {
	return "uq_" + tableName + "_" + columnName
}
