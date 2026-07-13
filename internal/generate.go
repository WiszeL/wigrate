package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// createMigration runs `migrate create` and returns the resulting file pair descriptor.
// Under --dry-run, migrate create is a no-op (see runCommandFunc), so the file is
// synthesized here instead of rediscovered from disk — it was never written.
func createMigration(module migrationModule, entityName string, migrationName string, kind migrationKind) (migrationFile, error) {
	if err := runCommandFunc("migrate", "create", "-ext", "sql", "-dir", module.migrationDir, "-seq", migrationName); err != nil {
		return migrationFile{}, err
	}

	if DryRun {
		// ponytail: baseName lacks the real NNNNNN_ seq prefix — preview path only, never written to disk.
		return migrationFile{
			path:      filepath.Join(module.migrationDir, migrationName+".up.sql"),
			baseName:  migrationName,
			kind:      kind,
			direction: "up",
		}, nil
	}

	// Rediscover the file pair instead of depending on migrate CLI output.
	latest, err := findEntityMigrationState(module, entityName)
	if err != nil {
		return migrationFile{}, err
	}
	if latest == nil || latest.kind != kind {
		return migrationFile{}, fmt.Errorf("created %s migration for entity %s not found", kind, entityName)
	}

	return *latest, nil
}

func makeInitMigration(module migrationModule, entityName string) error {
	// Parsing entity schema
	schema, err := parseEntitySchema(module, entityName)
	if err != nil {
		return err
	}

	file, err := createMigration(module, entityName, "init_"+entityName, migrationKindInit)
	if err != nil {
		return err
	}

	return writeMigrationFiles(file, buildCreateTableSQL(schema), buildDropTableSQL(schema))
}

func overwriteLatestMigration(module migrationModule, entries []os.DirEntry, entityName string, latest migrationFile) error {
	switch latest.kind {
	case migrationKindInit:
		schema, err := parseEntitySchema(module, entityName)
		if err != nil {
			return err
		}
		return writeMigrationFiles(latest, buildCreateTableSQL(schema), buildDropTableSQL(schema))
	case migrationKindAlter:
		return overwriteAlterMigration(module, entries, entityName, latest)
	default:
		return fmt.Errorf("unknown migration kind %s", latest.kind)
	}
}

func makeAlterMigration(module migrationModule, entries []os.DirEntry, entityName string) error {
	// Alter migrations are generated from the diff between migration history and current entity code.
	diff, err := buildSchemaDiff(module, entries, entityName, nil)
	if err != nil {
		return err
	}

	// Checking for schema changes
	if diff.empty() {
		fmt.Printf("No schema changes detected for entity %s in module %s.\n", entityName, module.name)
		return nil
	}

	// Creating new migration
	migrationName := alterMigrationName(diff.changedColumnNames(), entityName)
	fmt.Printf("Create new alter migration for entity %s in module %s.\n", entityName, module.name)
	file, err := createMigration(module, entityName, migrationName, migrationKindAlter)
	if err != nil {
		return err
	}

	return writeMigrationFiles(file, buildAlterTableSQL(diff), buildRevertAlterTableSQL(diff))
}

func overwriteAlterMigration(module migrationModule, entries []os.DirEntry, entityName string, latest migrationFile) error {
	// Rebuild only the latest alter by replaying migration history before it.
	diff, err := buildSchemaDiff(module, entries, entityName, &latest)
	if err != nil {
		return err
	}
	if diff.empty() {
		fmt.Printf("No schema changes detected for latest alter migration of entity %s in module %s.\n", entityName, module.name)
		return nil
	}

	return writeMigrationFiles(latest, buildAlterTableSQL(diff), buildRevertAlterTableSQL(diff))
}

func buildSchemaDiff(module migrationModule, entries []os.DirEntry, entityName string, before *migrationFile) (schemaDiff, error) {
	current, err := parseEntitySchema(module, entityName)
	if err != nil {
		return schemaDiff{}, err
	}

	existing, err := readGeneratedSchema(module, entries, entityName, before)
	if err != nil {
		return schemaDiff{}, err
	}

	return diffSchema(existing, current)
}

func writeMigrationFiles(file migrationFile, upSQL string, downSQL string) error {
	upPath, downPath := migrationFilePair(file)

	// Checking for dry run
	if DryRun {
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

func buildCreateTableSQL(schema tableSchema) string {
	lines := make([]string, 0, len(schema.columns)+len(schema.foreignKeys)+1+len(schema.uniques))

	// Building columns
	for _, column := range schema.columns {
		lines = append(lines, "    "+buildColumnDefinition(column))
	}

	// Composite primary key (single-column PK stays inline on the column)
	if len(schema.primaryKey) > 0 {
		lines = append(lines, "    PRIMARY KEY ("+strings.Join(schema.primaryKey, ", ")+")")
	}

	// Composite unique constraints
	for _, cols := range schema.uniques {
		lines = append(lines, "    CONSTRAINT "+buildUniqueConstraintDefinition(schema.name, cols))
	}

	// Building foreign keys
	for _, foreignKey := range schema.foreignKeys {
		lines = append(lines, "    "+buildCreateForeignKeyDefinition(schema.name, foreignKey))
	}

	statements := []string{fmt.Sprintf("CREATE TABLE %s (\n%s\n);", schema.name, strings.Join(lines, ",\n"))}

	// Indexes are not table constraints — always standalone statements, even at create time.
	for _, cols := range schema.indexes {
		statements = append(statements, createIndexStmt(schema.name, cols))
	}

	return strings.Join(statements, "\n") + "\n"
}

func buildDropTableSQL(schema tableSchema) string {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", schema.name)
}

// buildAlterTableSQL and buildRevertAlterTableSQL assemble the ALTER TABLE block
// (if any column/FK/unique changes exist) plus standalone CREATE/DROP INDEX
// statements — indexes are not table constraints, so they never nest inside
// ALTER TABLE, and an index-only diff must omit the (otherwise empty) block.
func buildAlterTableSQL(diff schemaDiff) string {
	statements := alterTableStatements(diff.tableName, buildAlterUpLines(diff))
	for _, cols := range diff.removedIndexes {
		statements = append(statements, dropIndexStmt(diff.tableName, cols))
	}
	for _, cols := range diff.addedIndexes {
		statements = append(statements, createIndexStmt(diff.tableName, cols))
	}

	return strings.Join(statements, "\n") + "\n"
}

func buildRevertAlterTableSQL(diff schemaDiff) string {
	statements := alterTableStatements(diff.tableName, buildAlterDownLines(diff))
	for i := len(diff.addedIndexes) - 1; i >= 0; i-- {
		statements = append(statements, dropIndexStmt(diff.tableName, diff.addedIndexes[i]))
	}
	for i := len(diff.removedIndexes) - 1; i >= 0; i-- {
		statements = append(statements, createIndexStmt(diff.tableName, diff.removedIndexes[i]))
	}

	return strings.Join(statements, "\n") + "\n"
}

func alterTableStatements(tableName string, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s\n%s;", tableName, strings.Join(lines, ",\n"))}
}

func buildAlterUpLines(diff schemaDiff) []string {
	lines := make([]string, 0)

	// Drop constraints before touching dependent columns.
	for _, foreignKey := range diff.removedForeignKeys {
		lines = append(lines, dropForeignKeyLine(diff.tableName, foreignKey))
	}
	for _, change := range diff.changedForeignKeys {
		lines = append(lines, dropForeignKeyLine(diff.tableName, change.before))
	}
	for _, cols := range diff.removedUniques {
		lines = append(lines, dropCompositeUniqueLine(diff.tableName, cols))
	}

	// Removing columns
	for _, column := range diff.removedColumns {
		lines = append(lines, dropColumnLine(column))
	}

	// Column changes stay between removals and additions for predictable diffs.
	for _, change := range diff.changedColumns {
		lines = append(lines, buildAlterColumnChangeLines(diff.tableName, change.before, change.after)...)
	}

	// Adding columns
	for _, column := range diff.addedColumns {
		lines = append(lines, addColumnLine(column))
	}

	// Add constraints after their columns are guaranteed to exist.
	for _, cols := range diff.addedUniques {
		lines = append(lines, addCompositeUniqueLine(diff.tableName, cols))
	}
	for _, foreignKey := range diff.addedForeignKeys {
		lines = append(lines, addForeignKeyLine(diff.tableName, foreignKey))
	}
	for _, change := range diff.changedForeignKeys {
		lines = append(lines, addForeignKeyLine(diff.tableName, change.after))
	}

	return lines
}

func buildAlterDownLines(diff schemaDiff) []string {
	lines := make([]string, 0)

	// Down migrations reverse up operations in dependency-safe order.

	// Dropping added constraints
	for i := len(diff.changedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, dropForeignKeyLine(diff.tableName, diff.changedForeignKeys[i].after))
	}
	for i := len(diff.addedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, dropForeignKeyLine(diff.tableName, diff.addedForeignKeys[i]))
	}
	for i := len(diff.addedUniques) - 1; i >= 0; i-- {
		lines = append(lines, dropCompositeUniqueLine(diff.tableName, diff.addedUniques[i]))
	}

	// Removing added columns
	for i := len(diff.addedColumns) - 1; i >= 0; i-- {
		lines = append(lines, dropColumnLine(diff.addedColumns[i]))
	}
	// Reverting column changes
	for i := len(diff.changedColumns) - 1; i >= 0; i-- {
		change := diff.changedColumns[i]
		lines = append(lines, buildAlterColumnChangeLines(diff.tableName, change.after, change.before)...)
	}
	// Adding back removed columns
	for i := len(diff.removedColumns) - 1; i >= 0; i-- {
		lines = append(lines, addColumnLine(diff.removedColumns[i]))
	}
	// Adding back removed constraints
	for i := len(diff.removedUniques) - 1; i >= 0; i-- {
		lines = append(lines, addCompositeUniqueLine(diff.tableName, diff.removedUniques[i]))
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
	// Removing unique
	if before.unique && !after.unique {
		lines = append(lines, dropCompositeUniqueLine(tableName, []string{before.name}))
	}

	// Changing type
	if before.dataType != after.dataType {
		lines = append(lines, fmt.Sprintf("    ALTER COLUMN %s TYPE %s", after.name, after.dataType))
	}

	// Changing nullability
	lines = append(lines, buildAlterColumnNullLine(before, after)...)

	// Adding unique
	if !before.unique && after.unique {
		lines = append(lines, addCompositeUniqueLine(tableName, []string{after.name}))
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
	return "    ADD " + buildCreateForeignKeyDefinition(tableName, foreignKey)
}

func dropForeignKeyLine(tableName string, foreignKey foreignKeySchema) string {
	return "    DROP CONSTRAINT IF EXISTS " + foreignKeyConstraintName(tableName, foreignKey.column)
}

func addCompositeUniqueLine(tableName string, cols []string) string {
	return "    ADD CONSTRAINT " + buildUniqueConstraintDefinition(tableName, cols)
}

func dropCompositeUniqueLine(tableName string, cols []string) string {
	return "    DROP CONSTRAINT IF EXISTS " + uniqueConstraintName(tableName, cols...)
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

func buildCreateForeignKeyDefinition(tableName string, foreignKey foreignKeySchema) string {
	return "CONSTRAINT " + foreignKeyConstraintName(tableName, foreignKey.column) +
		" FOREIGN KEY (" + foreignKey.column + ") REFERENCES " +
		foreignKey.refTable + "(" + foreignKey.refColumn + ")" +
		func() string {
			if foreignKey.onDelete != "" {
				return " ON DELETE " + foreignKey.onDelete
			}
			return ""
		}()
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

func foreignKeyConstraintName(tableName string, column string) string {
	return "fk_" + tableName + "_" + column
}

func buildUniqueConstraintDefinition(tableName string, columns []string) string {
	return uniqueConstraintName(tableName, columns...) + " UNIQUE (" + strings.Join(columns, ", ") + ")"
}

func uniqueConstraintName(tableName string, columns ...string) string {
	return "uq_" + tableName + "_" + strings.Join(columns, "_")
}

func indexName(tableName string, columns []string) string {
	return "idx_" + tableName + "_" + strings.Join(columns, "_")
}

func createIndexStmt(tableName string, columns []string) string {
	return "CREATE INDEX " + indexName(tableName, columns) + " ON " + tableName + " (" + strings.Join(columns, ", ") + ");"
}

// dropIndexStmt takes the index name only — Postgres index names are schema-scoped, not table-scoped.
func dropIndexStmt(tableName string, columns []string) string {
	return "DROP INDEX IF EXISTS " + indexName(tableName, columns) + ";"
}
