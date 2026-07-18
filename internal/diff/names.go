package diff

import (
	"fmt"
	"os"
	"strings"

	"github.com/wiszel/wigrate/internal/schema"
)

// ChangedColumnNames lists all columns touched by the diff (used for migration filenames)
func (diff Result) ChangedColumnNames() []string {
	var names []string
	seen := make(map[string]struct{})
	appendName := func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	for _, column := range diff.AddedColumns {
		appendName(column.Name)
	}
	for _, column := range diff.RemovedColumns {
		appendName(column.Name)
	}
	for _, change := range diff.ChangedColumns {
		appendName(change.After.Name)
	}
	for _, foreignKey := range diff.AddedForeignKeys {
		appendName(foreignKey.Column)
	}
	for _, foreignKey := range diff.RemovedForeignKeys {
		appendName(foreignKey.Column)
	}
	for _, change := range diff.ChangedForeignKeys {
		appendName(change.After.Column)
	}
	for _, cols := range diff.AddedUniques {
		appendName(strings.Join(cols, "_"))
	}
	for _, cols := range diff.RemovedUniques {
		appendName(strings.Join(cols, "_"))
	}
	for _, cols := range diff.AddedIndexes {
		appendName(strings.Join(cols, "_"))
	}
	for _, cols := range diff.RemovedIndexes {
		appendName(strings.Join(cols, "_"))
	}
	for _, col := range diff.AddedTrgmIndexes {
		appendName(col)
	}
	for _, col := range diff.RemovedTrgmIndexes {
		appendName(col)
	}

	return names
}

func trgmNameSet(tableName string, columns []string) map[string]struct{} {
	names := make(map[string]struct{}, len(columns))
	for _, col := range columns {
		names[schema.TrgmIndexName(tableName, col)] = struct{}{}
	}

	return names
}

func uniqueNameSet(tableName string, uniques [][]string) map[string]struct{} {
	names := make(map[string]struct{}, len(uniques))
	for _, cols := range uniques {
		names[schema.UniqueConstraintName(tableName, cols...)] = struct{}{}
	}

	return names
}

func indexNameSet(tableName string, indexes [][]string) map[string]struct{} {
	names := make(map[string]struct{}, len(indexes))
	for _, cols := range indexes {
		names[schema.IndexName(tableName, cols)] = struct{}{}
	}

	return names
}

func columnsByName(columns []schema.Column) map[string]schema.Column {
	byName := make(map[string]schema.Column, len(columns))
	for _, column := range columns {
		byName[column.Name] = column
	}

	return byName
}

func foreignKeysByColumn(foreignKeys []schema.ForeignKey) map[string]schema.ForeignKey {
	byColumn := make(map[string]schema.ForeignKey, len(foreignKeys))
	for _, foreignKey := range foreignKeys {
		byColumn[foreignKey.Column] = foreignKey
	}

	return byColumn
}

func warnColumnRename(removed []schema.Column, added []schema.Column) {
	for _, r := range removed {
		warned := false
		for _, a := range added {
			if r.DataType == a.DataType {
				fmt.Fprintf(os.Stderr, "warning: column %q removed and %q added with same type %q — if this is a rename, data will be lost\n", r.Name, a.Name, r.DataType)
				warned = true
			}
		}
		if !warned {
			fmt.Fprintf(os.Stderr, "warning: column %q dropped — data will be lost\n", r.Name)
		}
	}
}
