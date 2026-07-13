package internal

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type schemaDiff struct {
	tableName          string
	addedColumns       []columnSchema
	removedColumns     []columnSchema
	changedColumns     []columnChange
	addedForeignKeys   []foreignKeySchema
	removedForeignKeys []foreignKeySchema
	changedForeignKeys []foreignKeyChange
	addedUniques       [][]string
	removedUniques     [][]string
	addedIndexes       [][]string
	removedIndexes     [][]string
}

type columnChange struct {
	before columnSchema
	after  columnSchema
}

type foreignKeyChange struct {
	before foreignKeySchema
	after  foreignKeySchema
}

func diffSchema(previous tableSchema, desired tableSchema) (schemaDiff, error) {
	// Primary key changes are intentionally manual in v1 (single-column and composite alike).
	if !slices.Equal(previous.primaryKey, desired.primaryKey) {
		return schemaDiff{}, fmt.Errorf("primary key change is not supported in alter migration")
	}

	diff := schemaDiff{tableName: desired.name}

	previousColumns := columnsByName(previous.columns)
	desiredColumns := columnsByName(desired.columns)

	// Identifying added and changed columns
	for _, column := range desired.columns {
		before, ok := previousColumns[column.name]
		if !ok {
			if column.primary {
				return schemaDiff{}, fmt.Errorf("adding primary key column %s is not supported in alter migration", column.name)
			}
			diff.addedColumns = append(diff.addedColumns, column)
			continue
		}

		if before.primary != column.primary {
			return schemaDiff{}, fmt.Errorf("primary key change for column %s is not supported in alter migration", column.name)
		}
		if before != column {
			diff.changedColumns = append(diff.changedColumns, columnChange{before: before, after: column})
		}
	}

	// Identifying removed columns
	for _, column := range previous.columns {
		if _, ok := desiredColumns[column.name]; ok {
			continue
		}
		if column.primary {
			return schemaDiff{}, fmt.Errorf("removing primary key column %s is not supported in alter migration", column.name)
		}
		diff.removedColumns = append(diff.removedColumns, column)
	}

	// Warn on potential renames
	warnColumnRename(diff.removedColumns, diff.addedColumns)

	previousForeignKeys := foreignKeysByColumn(previous.foreignKeys)
	desiredForeignKeys := foreignKeysByColumn(desired.foreignKeys)

	// Identifying added and changed foreign keys
	for _, foreignKey := range desired.foreignKeys {
		before, ok := previousForeignKeys[foreignKey.column]
		if !ok {
			diff.addedForeignKeys = append(diff.addedForeignKeys, foreignKey)
			continue
		}
		if before != foreignKey {
			diff.changedForeignKeys = append(diff.changedForeignKeys, foreignKeyChange{before: before, after: foreignKey})
		}
	}

	// Diff foreign keys: removed
	for _, foreignKey := range previous.foreignKeys {
		if _, ok := desiredForeignKeys[foreignKey.column]; !ok {
			diff.removedForeignKeys = append(diff.removedForeignKeys, foreignKey)
		}
	}

	// Composite unique constraints, keyed by their constraint name (a column-set
	// change yields a different name, so it naturally diffs as remove+add).
	// Iterate the original slices (not the name maps) to keep output order deterministic.
	previousUniqueNames := uniqueNameSet(desired.name, previous.uniques)
	desiredUniqueNames := uniqueNameSet(desired.name, desired.uniques)

	for _, cols := range desired.uniques {
		if _, ok := previousUniqueNames[uniqueConstraintName(desired.name, cols...)]; !ok {
			diff.addedUniques = append(diff.addedUniques, cols)
		}
	}
	for _, cols := range previous.uniques {
		if _, ok := desiredUniqueNames[uniqueConstraintName(desired.name, cols...)]; !ok {
			diff.removedUniques = append(diff.removedUniques, cols)
		}
	}

	// Plain indexes, keyed by their constraint-style name (same rationale as uniques).
	previousIndexNames := indexNameSet(desired.name, previous.indexes)
	desiredIndexNames := indexNameSet(desired.name, desired.indexes)

	for _, cols := range desired.indexes {
		if _, ok := previousIndexNames[indexName(desired.name, cols)]; !ok {
			diff.addedIndexes = append(diff.addedIndexes, cols)
		}
	}
	for _, cols := range previous.indexes {
		if _, ok := desiredIndexNames[indexName(desired.name, cols)]; !ok {
			diff.removedIndexes = append(diff.removedIndexes, cols)
		}
	}

	return diff, nil
}

func uniqueNameSet(tableName string, uniques [][]string) map[string]struct{} {
	names := make(map[string]struct{}, len(uniques))
	for _, cols := range uniques {
		names[uniqueConstraintName(tableName, cols...)] = struct{}{}
	}

	return names
}

func indexNameSet(tableName string, indexes [][]string) map[string]struct{} {
	names := make(map[string]struct{}, len(indexes))
	for _, cols := range indexes {
		names[indexName(tableName, cols)] = struct{}{}
	}

	return names
}

func (diff schemaDiff) empty() bool {
	return len(diff.addedColumns) == 0 &&
		len(diff.removedColumns) == 0 &&
		len(diff.changedColumns) == 0 &&
		len(diff.addedForeignKeys) == 0 &&
		len(diff.removedForeignKeys) == 0 &&
		len(diff.changedForeignKeys) == 0 &&
		len(diff.addedUniques) == 0 &&
		len(diff.removedUniques) == 0 &&
		len(diff.addedIndexes) == 0 &&
		len(diff.removedIndexes) == 0
}

func (diff schemaDiff) changedColumnNames() []string {
	// Migration filenames summarize affected columns without duplicates.
	var names []string
	seen := make(map[string]struct{})
	appendName := func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	for _, column := range diff.addedColumns {
		appendName(column.name)
	}
	for _, column := range diff.removedColumns {
		appendName(column.name)
	}
	for _, change := range diff.changedColumns {
		appendName(change.after.name)
	}
	for _, foreignKey := range diff.addedForeignKeys {
		appendName(foreignKey.column)
	}
	for _, foreignKey := range diff.removedForeignKeys {
		appendName(foreignKey.column)
	}
	for _, change := range diff.changedForeignKeys {
		appendName(change.after.column)
	}
	for _, cols := range diff.addedUniques {
		appendName(strings.Join(cols, "_"))
	}
	for _, cols := range diff.removedUniques {
		appendName(strings.Join(cols, "_"))
	}
	for _, cols := range diff.addedIndexes {
		appendName(strings.Join(cols, "_"))
	}
	for _, cols := range diff.removedIndexes {
		appendName(strings.Join(cols, "_"))
	}

	return names
}

func columnsByName(columns []columnSchema) map[string]columnSchema {
	byName := make(map[string]columnSchema, len(columns))
	for _, column := range columns {
		byName[column.name] = column
	}

	return byName
}

func foreignKeysByColumn(foreignKeys []foreignKeySchema) map[string]foreignKeySchema {
	byColumn := make(map[string]foreignKeySchema, len(foreignKeys))
	for _, foreignKey := range foreignKeys {
		byColumn[foreignKey.column] = foreignKey
	}

	return byColumn
}

func warnColumnRename(removed []columnSchema, added []columnSchema) {
	for _, r := range removed {
		warned := false
		for _, a := range added {
			if r.dataType == a.dataType {
				fmt.Fprintf(os.Stderr, "warning: column %q removed and %q added with same type %q — if this is a rename, data will be lost\n", r.name, a.name, r.dataType)
				warned = true
			}
		}
		if !warned {
			fmt.Fprintf(os.Stderr, "warning: column %q dropped — data will be lost\n", r.name)
		}
	}
}
