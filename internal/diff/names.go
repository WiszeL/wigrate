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

// nameSet builds a lookup set of constraint/index names, keyed by whatever
// naming function the caller passes (unique/index/trgm all share this shape).
func nameSet[T any](items []T, name func(T) string) map[string]struct{} {
	names := make(map[string]struct{}, len(items))
	for _, item := range items {
		names[name(item)] = struct{}{}
	}

	return names
}

// diffByName compares two slices by name rather than deep equality (a
// same-name, different-column-set change is a remove+add, not an update) —
// shared by uniques, indexes, and trgm indexes in Compute.
func diffByName[T any](previous []T, desired []T, name func(T) string) (added []T, removed []T) {
	previousNames := nameSet(previous, name)
	desiredNames := nameSet(desired, name)

	for _, item := range desired {
		if _, ok := previousNames[name(item)]; !ok {
			added = append(added, item)
		}
	}
	for _, item := range previous {
		if _, ok := desiredNames[name(item)]; !ok {
			removed = append(removed, item)
		}
	}

	return added, removed
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

// warnEnumValueRemoval warns when a changed column's CHECK constraint dropped
// an enum value — existing rows holding it will fail the migration on apply,
// since the CHECK is only widened/narrowed, never validated against data here.
func warnEnumValueRemoval(changed []ColumnChange) {
	for _, c := range changed {
		if c.Before.Check == "" {
			continue
		}
		removed := removedEnumValues(c.Before.Check, c.After.Check)
		if len(removed) > 0 {
			fmt.Fprintf(os.Stderr, "warning: enum value(s) %s removed from column %q — rows holding them will fail migration\n",
				strings.Join(removed, ", "), c.After.Name)
		}
	}
}

func removedEnumValues(before string, after string) []string {
	afterSet := make(map[string]bool)
	for v := range strings.SplitSeq(after, ",") {
		if v != "" {
			afterSet[v] = true
		}
	}

	var removed []string
	for v := range strings.SplitSeq(before, ",") {
		if v != "" && !afterSet[v] {
			removed = append(removed, v)
		}
	}
	return removed
}
