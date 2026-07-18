package replay

import (
	"slices"
	"strings"

	"github.com/wiszel/wigrate/internal/schema"
)

// Applying ALTER COLUMN changes to a column in the table.
func applyGeneratedColumnAlter(table *schema.Table, line string) {
	columnName, rest, ok := strings.Cut(line, " ")
	if !ok {
		return
	}

	switch {
	case strings.HasPrefix(rest, "TYPE "):
		updateColumn(table, columnName, func(column *schema.Column) {
			column.DataType = strings.TrimPrefix(rest, "TYPE ")
		})
	case rest == "SET NOT NULL":
		updateColumn(table, columnName, func(column *schema.Column) {
			column.NotNull = true
		})
	case rest == "DROP NOT NULL":
		updateColumn(table, columnName, func(column *schema.Column) {
			column.NotNull = false
		})
	}
}

// Adding a column if it doesn't already exist.
func appendColumnIfMissing(table *schema.Table, column schema.Column) {
	if slices.IndexFunc(table.Columns, func(c schema.Column) bool { return c.Name == column.Name }) >= 0 {
		return
	}
	table.Columns = append(table.Columns, column)
}

// Removing a column and its foreign keys.
func removeColumn(table *schema.Table, columnName string) {
	// Removing from columns list
	if i := slices.IndexFunc(table.Columns, func(c schema.Column) bool { return c.Name == columnName }); i >= 0 {
		table.Columns = slices.Delete(table.Columns, i, i+1)
	}
	// Removing any foreign keys on this column
	removeForeignKey(table, columnName)
}

// Updating a column by name with a modification function.
func updateColumn(table *schema.Table, columnName string, update func(*schema.Column)) {
	i := slices.IndexFunc(table.Columns, func(c schema.Column) bool { return c.Name == columnName })
	if i < 0 {
		return
	}
	update(&table.Columns[i])
}

// Applying a UNIQUE constraint (single or composite column).
func applyGeneratedUniqueConstraint(table *schema.Table, columns []string) {
	// Single column UNIQUE: mark the column itself
	if len(columns) == 1 {
		updateColumn(table, columns[0], func(column *schema.Column) {
			column.Unique = true
		})
		return
	}

	// Composite UNIQUE: add as constraint if not already present
	name := schema.UniqueConstraintName(table.Name, columns...)
	for _, existing := range table.Uniques {
		if schema.UniqueConstraintName(table.Name, existing...) == name {
			return
		}
	}
	table.Uniques = append(table.Uniques, columns)
}

// Adding an index if it doesn't already exist.
func applyGeneratedIndex(table *schema.Table, columns []string) {
	name := schema.IndexName(table.Name, columns)
	for _, existing := range table.Indexes {
		if schema.IndexName(table.Name, existing) == name {
			return
		}
	}
	table.Indexes = append(table.Indexes, columns)
}

// Adding a foreign key if this column doesn't already have one.
func appendForeignKeyIfMissing(table *schema.Table, foreignKey schema.ForeignKey) {
	if slices.IndexFunc(table.ForeignKeys, func(fk schema.ForeignKey) bool { return fk.Column == foreignKey.Column }) >= 0 {
		return
	}
	table.ForeignKeys = append(table.ForeignKeys, foreignKey)
}

// Removing a foreign key by its constraint name.
func removeForeignKeyByConstraintName(table *schema.Table, constraintName string) {
	i := slices.IndexFunc(table.ForeignKeys, func(fk schema.ForeignKey) bool {
		return schema.ForeignKeyConstraintName(table.Name, fk.Column) == constraintName
	})
	if i >= 0 {
		table.ForeignKeys = slices.Delete(table.ForeignKeys, i, i+1)
	}
}

// Removing a UNIQUE constraint by its name (column or composite).
func removeUniqueByConstraintName(table *schema.Table, constraintName string) {
	// Check single-column UNIQUE on a column
	if i := slices.IndexFunc(table.Columns, func(col schema.Column) bool {
		return schema.UniqueConstraintName(table.Name, col.Name) == constraintName
	}); i >= 0 {
		table.Columns[i].Unique = false
		return
	}

	// Check composite UNIQUE constraints
	if i := slices.IndexFunc(table.Uniques, func(cols []string) bool {
		return schema.UniqueConstraintName(table.Name, cols...) == constraintName
	}); i >= 0 {
		table.Uniques = slices.Delete(table.Uniques, i, i+1)
	}
}

// Removing an index by its name.
func removeIndexByName(table *schema.Table, name string) {
	if i := slices.IndexFunc(table.Indexes, func(cols []string) bool {
		return schema.IndexName(table.Name, cols) == name
	}); i >= 0 {
		table.Indexes = slices.Delete(table.Indexes, i, i+1)
	}
}

// Adding a trigram index if it doesn't already exist.
func applyGeneratedTrgmIndex(table *schema.Table, column string) {
	name := schema.TrgmIndexName(table.Name, column)
	for _, existing := range table.TrgmIndexes {
		if schema.TrgmIndexName(table.Name, existing) == name {
			return
		}
	}
	table.TrgmIndexes = append(table.TrgmIndexes, column)
}

// Removing a trigram index by its name.
func removeTrgmIndexByName(table *schema.Table, name string) {
	if i := slices.IndexFunc(table.TrgmIndexes, func(col string) bool {
		return schema.TrgmIndexName(table.Name, col) == name
	}); i >= 0 {
		table.TrgmIndexes = slices.Delete(table.TrgmIndexes, i, i+1)
	}
}

// Removing all foreign keys on a column.
func removeForeignKey(table *schema.Table, columnName string) {
	if i := slices.IndexFunc(table.ForeignKeys, func(fk schema.ForeignKey) bool { return fk.Column == columnName }); i >= 0 {
		table.ForeignKeys = slices.Delete(table.ForeignKeys, i, i+1)
	}
}
