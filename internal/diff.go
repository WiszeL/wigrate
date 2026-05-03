package internal

import "fmt"

type schemaDiff struct {
	tableName          string
	addedColumns       []columnSchema
	removedColumns     []columnSchema
	changedColumns     []columnChange
	addedForeignKeys   []foreignKeySchema
	removedForeignKeys []foreignKeySchema
	changedForeignKeys []foreignKeyChange
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
	// Primary key changes are intentionally manual in v1.
	diff := schemaDiff{tableName: desired.name}

	previousColumns := columnsByName(previous.columns)
	desiredColumns := columnsByName(desired.columns)

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
		if !sameColumn(before, column) {
			diff.changedColumns = append(diff.changedColumns, columnChange{before: before, after: column})
		}
	}

	for _, column := range previous.columns {
		if _, ok := desiredColumns[column.name]; ok {
			continue
		}
		if column.primary {
			return schemaDiff{}, fmt.Errorf("removing primary key column %s is not supported in alter migration", column.name)
		}
		diff.removedColumns = append(diff.removedColumns, column)
	}

	previousForeignKeys := foreignKeysByColumn(previous.foreignKeys)
	desiredForeignKeys := foreignKeysByColumn(desired.foreignKeys)

	for _, foreignKey := range desired.foreignKeys {
		before, ok := previousForeignKeys[foreignKey.column]
		if !ok {
			diff.addedForeignKeys = append(diff.addedForeignKeys, foreignKey)
			continue
		}
		if !sameForeignKey(before, foreignKey) {
			diff.changedForeignKeys = append(diff.changedForeignKeys, foreignKeyChange{before: before, after: foreignKey})
		}
	}

	for _, foreignKey := range previous.foreignKeys {
		if _, ok := desiredForeignKeys[foreignKey.column]; !ok {
			diff.removedForeignKeys = append(diff.removedForeignKeys, foreignKey)
		}
	}

	return diff, nil
}

func (diff schemaDiff) empty() bool {
	return len(diff.addedColumns) == 0 &&
		len(diff.removedColumns) == 0 &&
		len(diff.changedColumns) == 0 &&
		len(diff.addedForeignKeys) == 0 &&
		len(diff.removedForeignKeys) == 0 &&
		len(diff.changedForeignKeys) == 0
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

	return names
}

func (diff schemaDiff) operationCount() int {
	return len(diff.addedColumns) +
		len(diff.removedColumns) +
		len(diff.changedColumns)*3 +
		len(diff.addedForeignKeys) +
		len(diff.removedForeignKeys) +
		len(diff.changedForeignKeys)*2
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

func sameColumn(left columnSchema, right columnSchema) bool {
	return left.name == right.name &&
		left.dataType == right.dataType &&
		left.notNull == right.notNull &&
		left.primary == right.primary &&
		left.unique == right.unique
}

func sameForeignKey(left foreignKeySchema, right foreignKeySchema) bool {
	return left.column == right.column &&
		left.refTable == right.refTable &&
		left.refColumn == right.refColumn &&
		left.onDelete == right.onDelete
}
