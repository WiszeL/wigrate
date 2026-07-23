// Package sqlgen turns schema changes into SQL for migration files
package sqlgen

import (
	"fmt"
	"strings"

	"github.com/wiszel/wigrate/internal/diff"
	"github.com/wiszel/wigrate/internal/schema"
)

func CreateTableSQL(table schema.Table) string {
	lines := make([]string, 0, len(table.Columns)+len(table.ForeignKeys)+1+len(table.Uniques))

	// Building columns
	for _, column := range table.Columns {
		lines = append(lines, "    "+buildColumnDefinition(column))
	}

	// Composite primary key (single-column PK stays inline)
	if len(table.PrimaryKey) > 0 {
		lines = append(lines, "    PRIMARY KEY ("+strings.Join(table.PrimaryKey, ", ")+")")
	}

	// Composite unique constraints
	for _, cols := range table.Uniques {
		lines = append(lines, "    CONSTRAINT "+buildUniqueConstraintDefinition(table.Name, cols))
	}

	// Enum columns get a CHECK constraint
	for _, column := range table.Columns {
		if column.Check != "" {
			lines = append(lines, "    CONSTRAINT "+buildCheckConstraintDefinition(table.Name, column))
		}
	}

	// Building foreign keys
	for _, foreignKey := range table.ForeignKeys {
		lines = append(lines, "    "+buildCreateForeignKeyDefinition(table.Name, foreignKey))
	}

	statements := []string{fmt.Sprintf("CREATE TABLE %s (\n%s\n);", table.Name, strings.Join(lines, ",\n"))}

	// Indexes are always standalone statements
	for _, cols := range table.Indexes {
		statements = append(statements, createIndexStmt(table.Name, cols))
	}

	// Trigram indexes: load extension once per file, before creating indexes
	if len(table.TrgmIndexes) > 0 {
		statements = append(statements, createTrgmExtensionStmt)
		for _, col := range table.TrgmIndexes {
			statements = append(statements, createTrgmIndexStmt(table.Name, col))
		}
	}

	return strings.Join(statements, "\n") + "\n"
}

func DropTableSQL(table schema.Table) string {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", table.Name)
}

// AlterTableSQL builds ALTER TABLE for column/constraint changes plus standalone index statements
func AlterTableSQL(d diff.Result) string {
	statements := alterTableStatements(d.TableName, buildAlterUpLines(d))
	for _, cols := range d.RemovedIndexes {
		statements = append(statements, dropIndexStmt(d.TableName, cols))
	}
	for _, cols := range d.AddedIndexes {
		statements = append(statements, createIndexStmt(d.TableName, cols))
	}
	for _, col := range d.RemovedTrgmIndexes {
		statements = append(statements, dropTrgmIndexStmt(d.TableName, col))
	}
	if len(d.AddedTrgmIndexes) > 0 {
		statements = append(statements, createTrgmExtensionStmt)
		for _, col := range d.AddedTrgmIndexes {
			statements = append(statements, createTrgmIndexStmt(d.TableName, col))
		}
	}

	return strings.Join(statements, "\n") + "\n"
}

// RevertAlterTableSQL builds ALTER TABLE to undo the changes
func RevertAlterTableSQL(d diff.Result) string {
	statements := alterTableStatements(d.TableName, buildAlterDownLines(d))
	for i := len(d.AddedIndexes) - 1; i >= 0; i-- {
		statements = append(statements, dropIndexStmt(d.TableName, d.AddedIndexes[i]))
	}
	for i := len(d.RemovedIndexes) - 1; i >= 0; i-- {
		statements = append(statements, createIndexStmt(d.TableName, d.RemovedIndexes[i]))
	}
	for i := len(d.AddedTrgmIndexes) - 1; i >= 0; i-- {
		statements = append(statements, dropTrgmIndexStmt(d.TableName, d.AddedTrgmIndexes[i]))
	}
	if len(d.RemovedTrgmIndexes) > 0 {
		statements = append(statements, createTrgmExtensionStmt)
		for i := len(d.RemovedTrgmIndexes) - 1; i >= 0; i-- {
			statements = append(statements, createTrgmIndexStmt(d.TableName, d.RemovedTrgmIndexes[i]))
		}
	}

	return strings.Join(statements, "\n") + "\n"
}

func alterTableStatements(tableName string, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s\n%s;", tableName, strings.Join(lines, ",\n"))}
}

func buildAlterUpLines(d diff.Result) []string {
	lines := make([]string, 0)

	// Drop constraints before touching dependent columns
	for _, foreignKey := range d.RemovedForeignKeys {
		lines = append(lines, dropForeignKeyLine(d.TableName, foreignKey))
	}
	for _, change := range d.ChangedForeignKeys {
		lines = append(lines, dropForeignKeyLine(d.TableName, change.Before))
	}
	for _, cols := range d.RemovedUniques {
		lines = append(lines, dropCompositeUniqueLine(d.TableName, cols))
	}

	// Removing columns
	for _, column := range d.RemovedColumns {
		lines = append(lines, dropColumnLine(column))
	}

	// Column changes come between removals and additions for consistent output
	for _, change := range d.ChangedColumns {
		lines = append(lines, buildAlterColumnChangeLines(d.TableName, change.Before, change.After)...)
	}

	// Adding columns
	for _, column := range d.AddedColumns {
		lines = append(lines, addColumnLine(column))
		if column.Check != "" {
			lines = append(lines, "    ADD CONSTRAINT "+buildCheckConstraintDefinition(d.TableName, column))
		}
	}

	// Add constraints after columns exist
	for _, cols := range d.AddedUniques {
		lines = append(lines, addCompositeUniqueLine(d.TableName, cols))
	}
	for _, foreignKey := range d.AddedForeignKeys {
		lines = append(lines, addForeignKeyLine(d.TableName, foreignKey))
	}
	for _, change := range d.ChangedForeignKeys {
		lines = append(lines, addForeignKeyLine(d.TableName, change.After))
	}

	return lines
}

func buildAlterDownLines(d diff.Result) []string {
	lines := make([]string, 0)

	// Down migrations reverse up operations in dependency-safe order

	// Dropping constraints added in the up migration
	for i := len(d.ChangedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, dropForeignKeyLine(d.TableName, d.ChangedForeignKeys[i].After))
	}
	for i := len(d.AddedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, dropForeignKeyLine(d.TableName, d.AddedForeignKeys[i]))
	}
	for i := len(d.AddedUniques) - 1; i >= 0; i-- {
		lines = append(lines, dropCompositeUniqueLine(d.TableName, d.AddedUniques[i]))
	}

	// Removing columns added in the up migration
	for i := len(d.AddedColumns) - 1; i >= 0; i-- {
		lines = append(lines, dropColumnLine(d.AddedColumns[i]))
	}

	// Reverting column changes
	for i := len(d.ChangedColumns) - 1; i >= 0; i-- {
		change := d.ChangedColumns[i]
		lines = append(lines, buildAlterColumnChangeLines(d.TableName, change.After, change.Before)...)
	}

	// Adding back columns that were removed in the up migration
	for i := len(d.RemovedColumns) - 1; i >= 0; i-- {
		lines = append(lines, addColumnLine(d.RemovedColumns[i]))
	}

	// Adding back constraints that were removed in the up migration
	for i := len(d.RemovedUniques) - 1; i >= 0; i-- {
		lines = append(lines, addCompositeUniqueLine(d.TableName, d.RemovedUniques[i]))
	}
	for i := len(d.ChangedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, addForeignKeyLine(d.TableName, d.ChangedForeignKeys[i].Before))
	}
	for i := len(d.RemovedForeignKeys) - 1; i >= 0; i-- {
		lines = append(lines, addForeignKeyLine(d.TableName, d.RemovedForeignKeys[i]))
	}

	return lines
}

func buildAlterColumnChangeLines(tableName string, before schema.Column, after schema.Column) []string {
	var lines []string

	// Removing unique
	if before.Unique && !after.Unique {
		lines = append(lines, dropCompositeUniqueLine(tableName, []string{before.Name}))
	}

	// Dropping the old CHECK before the type/value change (enum values changed or enum removed)
	if before.Check != "" && before.Check != after.Check {
		lines = append(lines, "    DROP CONSTRAINT IF EXISTS "+schema.CheckConstraintName(tableName, before.Name))
	}

	// Changing data type
	if before.DataType != after.DataType {
		lines = append(lines, fmt.Sprintf("    ALTER COLUMN %s TYPE %s", after.Name, after.DataType))
	}

	// Changing whether column can be null
	lines = append(lines, buildAlterColumnNullLine(before, after)...)

	// Adding unique
	if !before.Unique && after.Unique {
		lines = append(lines, addCompositeUniqueLine(tableName, []string{after.Name}))
	}

	// Re-adding the CHECK with the new value list (enum values changed or enum added)
	if after.Check != "" && before.Check != after.Check {
		lines = append(lines, "    ADD CONSTRAINT "+buildCheckConstraintDefinition(tableName, after))
	}

	return lines
}
