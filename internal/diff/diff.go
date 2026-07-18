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
	previousUniqueNames := uniqueNameSet(desired.Name, previous.Uniques)
	desiredUniqueNames := uniqueNameSet(desired.Name, desired.Uniques)

	for _, cols := range desired.Uniques {
		if _, ok := previousUniqueNames[schema.UniqueConstraintName(desired.Name, cols...)]; !ok {
			diff.AddedUniques = append(diff.AddedUniques, cols)
		}
	}
	for _, cols := range previous.Uniques {
		if _, ok := desiredUniqueNames[schema.UniqueConstraintName(desired.Name, cols...)]; !ok {
			diff.RemovedUniques = append(diff.RemovedUniques, cols)
		}
	}

	// Indexes, keyed by name (different column sets get different names)
	previousIndexNames := indexNameSet(desired.Name, previous.Indexes)
	desiredIndexNames := indexNameSet(desired.Name, desired.Indexes)

	for _, cols := range desired.Indexes {
		if _, ok := previousIndexNames[schema.IndexName(desired.Name, cols)]; !ok {
			diff.AddedIndexes = append(diff.AddedIndexes, cols)
		}
	}
	for _, cols := range previous.Indexes {
		if _, ok := desiredIndexNames[schema.IndexName(desired.Name, cols)]; !ok {
			diff.RemovedIndexes = append(diff.RemovedIndexes, cols)
		}
	}

	// Trigram indexes, keyed by name to avoid collision with plain indexes
	previousTrgmNames := trgmNameSet(desired.Name, previous.TrgmIndexes)
	desiredTrgmNames := trgmNameSet(desired.Name, desired.TrgmIndexes)

	for _, col := range desired.TrgmIndexes {
		if _, ok := previousTrgmNames[schema.TrgmIndexName(desired.Name, col)]; !ok {
			diff.AddedTrgmIndexes = append(diff.AddedTrgmIndexes, col)
		}
	}
	for _, col := range previous.TrgmIndexes {
		if _, ok := desiredTrgmNames[schema.TrgmIndexName(desired.Name, col)]; !ok {
			diff.RemovedTrgmIndexes = append(diff.RemovedTrgmIndexes, col)
		}
	}

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
