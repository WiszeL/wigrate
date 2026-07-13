package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Migration_DiffSchema(t *testing.T) {
	t.Run("rejects primary key changes", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", notNull: true},
			},
		}

		// ===== Act ===== //
		_, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary key change")
	})

	t.Run("detects added columns", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name:    "users",
			columns: []columnSchema{{name: "id", dataType: "UUID", primary: true}},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT", notNull: true},
				{name: "name", dataType: "VARCHAR(50)", notNull: true},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.addedColumns, 2)
		assert.Equal(t, "email", diff.addedColumns[0].name)
		assert.Equal(t, "name", diff.addedColumns[1].name)
		assert.False(t, diff.empty())
	})

	t.Run("rejects primary key flag change on existing column", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name:    "users",
			columns: []columnSchema{{name: "id", dataType: "UUID"}},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
			},
		}

		// ===== Act ===== //
		_, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary key change")
	})

	t.Run("detects removed columns", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "obsolete", dataType: "TEXT", notNull: true},
				{name: "also_removed", dataType: "INTEGER"},
			},
		}
		desired := tableSchema{
			name:    "users",
			columns: []columnSchema{{name: "id", dataType: "UUID", primary: true}},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.removedColumns, 2)
		assert.Equal(t, "obsolete", diff.removedColumns[0].name)
		assert.Equal(t, "also_removed", diff.removedColumns[1].name)
	})

	t.Run("rejects removing primary key column", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT"},
			},
		}
		desired := tableSchema{
			name:    "users",
			columns: []columnSchema{{name: "email", dataType: "TEXT"}},
		}

		// ===== Act ===== //
		_, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "removing primary key column")
	})

	t.Run("detects changed column type", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT", notNull: true},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "VARCHAR(20)", notNull: true},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.changedColumns, 1)
		assert.Equal(t, "TEXT", diff.changedColumns[0].before.dataType)
		assert.Equal(t, "VARCHAR(20)", diff.changedColumns[0].after.dataType)
	})

	t.Run("detects changed column nullable", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT", notNull: true},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT"},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.changedColumns, 1)
		assert.True(t, diff.changedColumns[0].before.notNull)
		assert.False(t, diff.changedColumns[0].after.notNull)
	})

	t.Run("detects changed column unique", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT", notNull: true},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT", notNull: true, unique: true},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.changedColumns, 1)
		assert.False(t, diff.changedColumns[0].before.unique)
		assert.True(t, diff.changedColumns[0].after.unique)
	})

	t.Run("detects composite column change", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT", notNull: true},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "VARCHAR(100)"},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.changedColumns, 1)
		assert.Equal(t, "TEXT", diff.changedColumns[0].before.dataType)
		assert.Equal(t, "VARCHAR(100)", diff.changedColumns[0].after.dataType)
		assert.True(t, diff.changedColumns[0].before.notNull)
		assert.False(t, diff.changedColumns[0].after.notNull)
	})

	t.Run("detects added foreign key", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID", notNull: true},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID", notNull: true},
			},
			foreignKeys: []foreignKeySchema{
				{column: "role_id", refTable: "roles", refColumn: "id", onDelete: "CASCADE"},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.addedForeignKeys, 1)
		assert.Equal(t, "role_id", diff.addedForeignKeys[0].column)
	})

	t.Run("detects removed foreign key", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID"},
			},
			foreignKeys: []foreignKeySchema{
				{column: "role_id", refTable: "roles", refColumn: "id"},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID"},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.removedForeignKeys, 1)
		assert.Equal(t, "role_id", diff.removedForeignKeys[0].column)
	})

	t.Run("detects changed foreign key", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID"},
			},
			foreignKeys: []foreignKeySchema{
				{column: "role_id", refTable: "roles", refColumn: "id", onDelete: "CASCADE"},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID"},
			},
			foreignKeys: []foreignKeySchema{
				{column: "role_id", refTable: "teams", refColumn: "id", onDelete: "RESTRICT"},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.changedForeignKeys, 1)
		assert.Equal(t, "roles", diff.changedForeignKeys[0].before.refTable)
		assert.Equal(t, "teams", diff.changedForeignKeys[0].after.refTable)
		assert.Equal(t, "CASCADE", diff.changedForeignKeys[0].before.onDelete)
		assert.Equal(t, "RESTRICT", diff.changedForeignKeys[0].after.onDelete)
	})

	t.Run("returns empty diff for identical schemas", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "VARCHAR(20)", notNull: true, unique: true},
			},
			foreignKeys: []foreignKeySchema{
				{column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "VARCHAR(20)", notNull: true, unique: true},
			},
			foreignKeys: []foreignKeySchema{
				{column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, diff.empty())
	})

	t.Run("detects mixed changes in single diff", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "TEXT", notNull: true},
				{name: "removed_col", dataType: "INTEGER"},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "email", dataType: "VARCHAR(20)", notNull: true},
				{name: "added_col", dataType: "BOOLEAN", notNull: true},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.addedColumns, 1)
		assert.Equal(t, "added_col", diff.addedColumns[0].name)
		assert.Len(t, diff.removedColumns, 1)
		assert.Equal(t, "removed_col", diff.removedColumns[0].name)
		assert.Len(t, diff.changedColumns, 1)
		assert.Equal(t, "email", diff.changedColumns[0].after.name)
	})

	t.Run("detects mixed foreign key changes", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID"},
				{name: "org_id", dataType: "UUID"},
			},
			foreignKeys: []foreignKeySchema{
				{column: "role_id", refTable: "roles", refColumn: "id", onDelete: "CASCADE"},
				{column: "removed_fk", refTable: "old_table", refColumn: "id"},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "role_id", dataType: "UUID"},
				{name: "org_id", dataType: "UUID"},
			},
			foreignKeys: []foreignKeySchema{
				{column: "role_id", refTable: "roles", refColumn: "id", onDelete: "NO ACTION"},
				{column: "added_fk", refTable: "new_table", refColumn: "id"},
			},
		}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.changedForeignKeys, 1)
		assert.Equal(t, "role_id", diff.changedForeignKeys[0].before.column)
		assert.Len(t, diff.removedForeignKeys, 1)
		assert.Equal(t, "removed_fk", diff.removedForeignKeys[0].column)
		assert.Len(t, diff.addedForeignKeys, 1)
		assert.Equal(t, "added_fk", diff.addedForeignKeys[0].column)
	})

	t.Run("warns on potential column rename", func(t *testing.T) {
		// ===== Arrange ===== //
		removed := []columnSchema{{name: "old_name", dataType: "TEXT"}}
		added := []columnSchema{{name: "new_name", dataType: "TEXT"}}

		// ===== Act ===== //
		warnColumnRename(removed, added)

		// ===== Assert ===== //
	})

	t.Run("warns on potential FK column rename", func(t *testing.T) {
		// ===== Arrange ===== //
		removed := []columnSchema{{name: "old_fk", dataType: "UUID"}}
		added := []columnSchema{{name: "new_fk", dataType: "UUID"}}

		// ===== Act ===== //
		warnColumnRename(removed, added)

		// ===== Assert ===== //
	})

	t.Run("does not warn on different types", func(t *testing.T) {
		// ===== Arrange ===== //
		removed := []columnSchema{{name: "old_name", dataType: "TEXT"}}
		added := []columnSchema{{name: "new_name", dataType: "INTEGER"}}

		// ===== Act ===== //
		warnColumnRename(removed, added)

		// ===== Assert ===== //
	})

	t.Run("rejects composite primary key changes", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{name: "memberships", primaryKey: []string{"team", "user"}}
		desired := tableSchema{name: "memberships", primaryKey: []string{"team", "role"}}

		// ===== Act ===== //
		_, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary key change")
	})

	t.Run("detects added composite unique constraint", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{name: "memberships"}
		desired := tableSchema{name: "memberships", uniques: [][]string{{"team", "user"}}}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"team", "user"}}, diff.addedUniques)
		assert.Empty(t, diff.removedUniques)
		assert.False(t, diff.empty())
	})

	t.Run("detects removed composite unique constraint", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{name: "memberships", uniques: [][]string{{"team", "user"}}}
		desired := tableSchema{name: "memberships"}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"team", "user"}}, diff.removedUniques)
		assert.Empty(t, diff.addedUniques)
	})

	t.Run("column-set change on a composite unique diffs as remove+add", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{name: "memberships", uniques: [][]string{{"team", "user"}}}
		desired := tableSchema{name: "memberships", uniques: [][]string{{"team", "role"}}}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"team", "role"}}, diff.addedUniques)
		assert.Equal(t, [][]string{{"team", "user"}}, diff.removedUniques)
	})

	t.Run("detects added index", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{name: "users"}
		desired := tableSchema{name: "users", indexes: [][]string{{"email"}}}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"email"}}, diff.addedIndexes)
		assert.Empty(t, diff.removedIndexes)
		assert.False(t, diff.empty())
	})

	t.Run("detects removed index", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{name: "users", indexes: [][]string{{"email"}}}
		desired := tableSchema{name: "users"}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"email"}}, diff.removedIndexes)
		assert.Empty(t, diff.addedIndexes)
	})

	t.Run("column-set change on a composite index diffs as remove+add", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := tableSchema{name: "events", indexes: [][]string{{"tenant_id", "happened"}}}
		desired := tableSchema{name: "events", indexes: [][]string{{"tenant_id", "kind"}}}

		// ===== Act ===== //
		diff, err := diffSchema(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"tenant_id", "kind"}}, diff.addedIndexes)
		assert.Equal(t, [][]string{{"tenant_id", "happened"}}, diff.removedIndexes)
	})
}
