package diff

import (
	"fmt"
	"slices"

	"github.com/wiszel/wigrate/internal/schema"
)

type Result struct {
	TableName          string
	AddedColumns       []schema.Column
	RemovedColumns     []schema.Column
	ChangedColumns     []ColumnChange
	AddedForeignKeys   []schema.ForeignKey
	RemovedForeignKeys []schema.ForeignKey
	ChangedForeignKeys []ForeignKeyChange
	AddedUniques       [][]string
	RemovedUniques     [][]string
	AddedIndexes       [][]string
	RemovedIndexes     [][]string
	AddedTrgmIndexes   []string
	RemovedTrgmIndexes []string
}

type ColumnChange struct {
	Before schema.Column
	After  schema.Column
}

type ForeignKeyChange struct {
	Before schema.ForeignKey
	After  schema.ForeignKey
}

func Compute(previous schema.Table, desired schema.Table) (Result, error) {
	// Primary key changes are intentionally manual in v1
	if !slices.Equal(previous.PrimaryKey, desired.PrimaryKey) {
		return Result{}, fmt.Errorf("primary key change is not supported in alter migration")
	}

	diff := Result{TableName: desired.Name}

	previousColumns := columnsByName(previous.Columns)
	desiredColumns := columnsByName(desired.Columns)

	// Identifying added and changed columns
	for _, column := range desired.Columns {
		before, ok := previousColumns[column.Name]
		if !ok {
			if column.Primary {
				return Result{}, fmt.Errorf("adding primary key column %s is not supported in alter migration", column.Name)
			}
			diff.AddedColumns = append(diff.AddedColumns, column)
			continue
		}

		if before.Primary != column.Primary {
			return Result{}, fmt.Errorf("primary key change for column %s is not supported in alter migration", column.Name)
		}
		if before != column {
			diff.ChangedColumns = append(diff.ChangedColumns, ColumnChange{Before: before, After: column})
		}
	}

	// Identifying removed columns
	for _, column := range previous.Columns {
		if _, ok := desiredColumns[column.Name]; ok {
			continue
		}
		if column.Primary {
			return Result{}, fmt.Errorf("removing primary key column %s is not supported in alter migration", column.Name)
		}
		diff.RemovedColumns = append(diff.RemovedColumns, column)
	}

	// Warn on potential renames
	warnColumnRename(diff.RemovedColumns, diff.AddedColumns)

	// Warn on enum values dropped by a changed CHECK constraint
	warnEnumValueRemoval(diff.ChangedColumns)

	previousForeignKeys := foreignKeysByColumn(previous.ForeignKeys)
	desiredForeignKeys := foreignKeysByColumn(desired.ForeignKeys)

	// Identifying added and changed foreign keys
	for _, foreignKey := range desired.ForeignKeys {
		before, ok := previousForeignKeys[foreignKey.Column]
		if !ok {
			diff.AddedForeignKeys = append(diff.AddedForeignKeys, foreignKey)
			continue
		}
		if before != foreignKey {
			diff.ChangedForeignKeys = append(diff.ChangedForeignKeys, ForeignKeyChange{Before: before, After: foreignKey})
		}
	}

	// Identifying removed foreign keys
	for _, foreignKey := range previous.ForeignKeys {
		if _, ok := desiredForeignKeys[foreignKey.Column]; !ok {
			diff.RemovedForeignKeys = append(diff.RemovedForeignKeys, foreignKey)
		}
	}

	// Unique constraints, keyed by name (different column sets get different names)
	diff.AddedUniques, diff.RemovedUniques = diffByName(previous.Uniques, desired.Uniques, func(cols []string) string {
		return schema.UniqueConstraintName(desired.Name, cols...)
	})

	// Indexes, keyed by name (different column sets get different names)
	diff.AddedIndexes, diff.RemovedIndexes = diffByName(previous.Indexes, desired.Indexes, func(cols []string) string {
		return schema.IndexName(desired.Name, cols)
	})

	// Trigram indexes, keyed by name to avoid collision with plain indexes
	diff.AddedTrgmIndexes, diff.RemovedTrgmIndexes = diffByName(previous.TrgmIndexes, desired.TrgmIndexes, func(col string) string {
		return schema.TrgmIndexName(desired.Name, col)
	})

	return diff, nil
}

func (diff Result) Empty() bool {
	return len(diff.AddedColumns) == 0 &&
		len(diff.RemovedColumns) == 0 &&
		len(diff.ChangedColumns) == 0 &&
		len(diff.AddedForeignKeys) == 0 &&
		len(diff.RemovedForeignKeys) == 0 &&
		len(diff.ChangedForeignKeys) == 0 &&
		len(diff.AddedUniques) == 0 &&
		len(diff.RemovedUniques) == 0 &&
		len(diff.AddedIndexes) == 0 &&
		len(diff.RemovedIndexes) == 0 &&
		len(diff.AddedTrgmIndexes) == 0 &&
		len(diff.RemovedTrgmIndexes) == 0
}
