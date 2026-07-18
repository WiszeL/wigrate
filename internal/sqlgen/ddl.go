package sqlgen

import (
	"strings"

	"github.com/wiszel/wigrate/internal/schema"
)

func addColumnLine(column schema.Column) string {
	return "    ADD COLUMN " + buildColumnDefinition(column)
}

func dropColumnLine(column schema.Column) string {
	return "    DROP COLUMN IF EXISTS " + column.Name
}

func addForeignKeyLine(tableName string, foreignKey schema.ForeignKey) string {
	return "    ADD " + buildCreateForeignKeyDefinition(tableName, foreignKey)
}

func dropForeignKeyLine(tableName string, foreignKey schema.ForeignKey) string {
	return "    DROP CONSTRAINT IF EXISTS " + schema.ForeignKeyConstraintName(tableName, foreignKey.Column)
}

func addCompositeUniqueLine(tableName string, cols []string) string {
	return "    ADD CONSTRAINT " + buildUniqueConstraintDefinition(tableName, cols)
}

func dropCompositeUniqueLine(tableName string, cols []string) string {
	return "    DROP CONSTRAINT IF EXISTS " + schema.UniqueConstraintName(tableName, cols...)
}

func buildColumnDefinition(column schema.Column) string {
	parts := []string{column.Name, column.DataType}
	if column.Primary {
		parts = append(parts, "PRIMARY KEY")
	} else if column.NotNull {
		parts = append(parts, "NOT NULL")
	}
	if column.Unique {
		parts = append(parts, "UNIQUE")
	}

	return strings.Join(parts, " ")
}

func buildCreateForeignKeyDefinition(tableName string, foreignKey schema.ForeignKey) string {
	return "CONSTRAINT " + schema.ForeignKeyConstraintName(tableName, foreignKey.Column) +
		" FOREIGN KEY (" + foreignKey.Column + ") REFERENCES " +
		foreignKey.RefTable + "(" + foreignKey.RefColumn + ")" +
		func() string {
			if foreignKey.OnDelete != "" {
				return " ON DELETE " + foreignKey.OnDelete
			}
			return ""
		}()
}

func buildAlterColumnNullLine(before schema.Column, after schema.Column) []string {
	switch {
	case !before.NotNull && after.NotNull:
		return []string{"    ALTER COLUMN " + after.Name + " SET NOT NULL"}
	case before.NotNull && !after.NotNull:
		return []string{"    ALTER COLUMN " + after.Name + " DROP NOT NULL"}
	default:
		return nil
	}
}

func buildUniqueConstraintDefinition(tableName string, columns []string) string {
	return schema.UniqueConstraintName(tableName, columns...) + " UNIQUE (" + strings.Join(columns, ", ") + ")"
}

func createIndexStmt(tableName string, columns []string) string {
	return "CREATE INDEX " + schema.IndexName(tableName, columns) + " ON " + tableName + " (" + strings.Join(columns, ", ") + ");"
}

// dropIndexStmt drops an index (index names are schema-scoped in Postgres, not table-scoped)
func dropIndexStmt(tableName string, columns []string) string {
	return "DROP INDEX IF EXISTS " + schema.IndexName(tableName, columns) + ";"
}

// createTrgmExtensionStmt loads the extension once per file (it's database-wide and idempotent)
const createTrgmExtensionStmt = "CREATE EXTENSION IF NOT EXISTS pg_trgm;"

func createTrgmIndexStmt(tableName string, column string) string {
	return "CREATE INDEX " + schema.TrgmIndexName(tableName, column) + " ON " + tableName + " USING GIN (" + column + " gin_trgm_ops);"
}

// dropTrgmIndexStmt never drops the extension (other tables may still use it)
func dropTrgmIndexStmt(tableName string, column string) string {
	return "DROP INDEX IF EXISTS " + schema.TrgmIndexName(tableName, column) + ";"
}
