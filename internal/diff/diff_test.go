package diff

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiszel/wigrate/internal/schema"
)

func Test_Migration_DiffSchema(t *testing.T) {
	t.Run("rejects primary key changes", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", NotNull: true},
			},
		}

		// ===== Act ===== //
		_, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary key change")
	})

	t.Run("detects added columns", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name:    "users",
			Columns: []schema.Column{{Name: "id", DataType: "UUID", Primary: true}},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT", NotNull: true},
				{Name: "name", DataType: "VARCHAR(50)", NotNull: true},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, diff.AddedColumns, 2)
		assert.Equal(t, desired.Columns[1].Name, diff.AddedColumns[0].Name)
		assert.Equal(t, desired.Columns[1].DataType, diff.AddedColumns[0].DataType)
		assert.Equal(t, desired.Columns[1].NotNull, diff.AddedColumns[0].NotNull)
		assert.Equal(t, desired.Columns[2].Name, diff.AddedColumns[1].Name)
		assert.Equal(t, desired.Columns[2].DataType, diff.AddedColumns[1].DataType)
		assert.Equal(t, desired.Columns[2].NotNull, diff.AddedColumns[1].NotNull)
		assert.False(t, diff.Empty())
	})

	t.Run("rejects primary key flag change on existing column", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name:    "users",
			Columns: []schema.Column{{Name: "id", DataType: "UUID"}},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
			},
		}

		// ===== Act ===== //
		_, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary key change")
	})

	t.Run("detects removed columns", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "obsolete", DataType: "TEXT", NotNull: true},
				{Name: "also_removed", DataType: "INTEGER"},
			},
		}
		desired := schema.Table{
			Name:    "users",
			Columns: []schema.Column{{Name: "id", DataType: "UUID", Primary: true}},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, diff.RemovedColumns, 2)
		assert.Equal(t, previous.Columns[1].Name, diff.RemovedColumns[0].Name)
		assert.Equal(t, previous.Columns[1].DataType, diff.RemovedColumns[0].DataType)
		assert.Equal(t, previous.Columns[1].NotNull, diff.RemovedColumns[0].NotNull)
		assert.Equal(t, previous.Columns[2].Name, diff.RemovedColumns[1].Name)
		assert.Equal(t, previous.Columns[2].DataType, diff.RemovedColumns[1].DataType)
		assert.Equal(t, previous.Columns[2].NotNull, diff.RemovedColumns[1].NotNull)
	})

	t.Run("rejects removing primary key column", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT"},
			},
		}
		desired := schema.Table{
			Name:    "users",
			Columns: []schema.Column{{Name: "email", DataType: "TEXT"}},
		}

		// ===== Act ===== //
		_, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "removing primary key column")
	})

	t.Run("detects changed column type", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT", NotNull: true},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "VARCHAR(20)", NotNull: true},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.ChangedColumns, 1)
		assert.Equal(t, previous.Columns[1].Name, diff.ChangedColumns[0].Before.Name)
		assert.Equal(t, previous.Columns[1].DataType, diff.ChangedColumns[0].Before.DataType)
		assert.Equal(t, previous.Columns[1].NotNull, diff.ChangedColumns[0].Before.NotNull)
		assert.Equal(t, desired.Columns[1].Name, diff.ChangedColumns[0].After.Name)
		assert.Equal(t, desired.Columns[1].DataType, diff.ChangedColumns[0].After.DataType)
		assert.Equal(t, desired.Columns[1].NotNull, diff.ChangedColumns[0].After.NotNull)
	})

	t.Run("detects changed column nullable", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT", NotNull: true},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT"},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.ChangedColumns, 1)
		assert.Equal(t, previous.Columns[1].Name, diff.ChangedColumns[0].Before.Name)
		assert.Equal(t, previous.Columns[1].DataType, diff.ChangedColumns[0].Before.DataType)
		assert.Equal(t, previous.Columns[1].NotNull, diff.ChangedColumns[0].Before.NotNull)
		assert.Equal(t, desired.Columns[1].Name, diff.ChangedColumns[0].After.Name)
		assert.Equal(t, desired.Columns[1].DataType, diff.ChangedColumns[0].After.DataType)
		assert.Equal(t, desired.Columns[1].NotNull, diff.ChangedColumns[0].After.NotNull)
	})

	t.Run("detects changed column unique", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT", NotNull: true},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT", NotNull: true, Unique: true},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.ChangedColumns, 1)
		assert.Equal(t, previous.Columns[1].Name, diff.ChangedColumns[0].Before.Name)
		assert.Equal(t, previous.Columns[1].DataType, diff.ChangedColumns[0].Before.DataType)
		assert.Equal(t, previous.Columns[1].NotNull, diff.ChangedColumns[0].Before.NotNull)
		assert.Equal(t, previous.Columns[1].Unique, diff.ChangedColumns[0].Before.Unique)
		assert.Equal(t, desired.Columns[1].Name, diff.ChangedColumns[0].After.Name)
		assert.Equal(t, desired.Columns[1].DataType, diff.ChangedColumns[0].After.DataType)
		assert.Equal(t, desired.Columns[1].NotNull, diff.ChangedColumns[0].After.NotNull)
		assert.Equal(t, desired.Columns[1].Unique, diff.ChangedColumns[0].After.Unique)
	})

	t.Run("detects composite column change", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT", NotNull: true},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "VARCHAR(100)"},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.ChangedColumns, 1)
		assert.Equal(t, "TEXT", diff.ChangedColumns[0].Before.DataType)
		assert.Equal(t, "VARCHAR(100)", diff.ChangedColumns[0].After.DataType)
		assert.True(t, diff.ChangedColumns[0].Before.NotNull)
		assert.False(t, diff.ChangedColumns[0].After.NotNull)
	})

	t.Run("detects added foreign key", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID", NotNull: true},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID", NotNull: true},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "role_id", RefTable: "roles", RefColumn: "id", OnDelete: "CASCADE"},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, diff.AddedForeignKeys, 1)
		assert.Equal(t, desired.ForeignKeys[0].Column, diff.AddedForeignKeys[0].Column)
		assert.Equal(t, desired.ForeignKeys[0].RefTable, diff.AddedForeignKeys[0].RefTable)
		assert.Equal(t, desired.ForeignKeys[0].RefColumn, diff.AddedForeignKeys[0].RefColumn)
		assert.Equal(t, desired.ForeignKeys[0].OnDelete, diff.AddedForeignKeys[0].OnDelete)
	})

	t.Run("detects removed foreign key", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID"},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "role_id", RefTable: "roles", RefColumn: "id"},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID"},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, diff.RemovedForeignKeys, 1)
		assert.Equal(t, previous.ForeignKeys[0].Column, diff.RemovedForeignKeys[0].Column)
		assert.Equal(t, previous.ForeignKeys[0].RefTable, diff.RemovedForeignKeys[0].RefTable)
		assert.Equal(t, previous.ForeignKeys[0].RefColumn, diff.RemovedForeignKeys[0].RefColumn)
	})

	t.Run("detects changed foreign key", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID"},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "role_id", RefTable: "roles", RefColumn: "id", OnDelete: "CASCADE"},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID"},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "role_id", RefTable: "teams", RefColumn: "id", OnDelete: "RESTRICT"},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.ChangedForeignKeys, 1)
		assert.Equal(t, previous.ForeignKeys[0].Column, diff.ChangedForeignKeys[0].Before.Column)
		assert.Equal(t, previous.ForeignKeys[0].RefTable, diff.ChangedForeignKeys[0].Before.RefTable)
		assert.Equal(t, previous.ForeignKeys[0].RefColumn, diff.ChangedForeignKeys[0].Before.RefColumn)
		assert.Equal(t, previous.ForeignKeys[0].OnDelete, diff.ChangedForeignKeys[0].Before.OnDelete)
		assert.Equal(t, desired.ForeignKeys[0].Column, diff.ChangedForeignKeys[0].After.Column)
		assert.Equal(t, desired.ForeignKeys[0].RefTable, diff.ChangedForeignKeys[0].After.RefTable)
		assert.Equal(t, desired.ForeignKeys[0].RefColumn, diff.ChangedForeignKeys[0].After.RefColumn)
		assert.Equal(t, desired.ForeignKeys[0].OnDelete, diff.ChangedForeignKeys[0].After.OnDelete)
	})

	t.Run("returns empty diff for identical schemas", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "VARCHAR(20)", NotNull: true, Unique: true},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "VARCHAR(20)", NotNull: true, Unique: true},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, diff.Empty())
	})

	t.Run("detects mixed changes in single diff", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "TEXT", NotNull: true},
				{Name: "removed_col", DataType: "INTEGER"},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "email", DataType: "VARCHAR(20)", NotNull: true},
				{Name: "added_col", DataType: "BOOLEAN", NotNull: true},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.AddedColumns, 1)
		assert.Equal(t, "added_col", diff.AddedColumns[0].Name)
		assert.Len(t, diff.RemovedColumns, 1)
		assert.Equal(t, "removed_col", diff.RemovedColumns[0].Name)
		assert.Len(t, diff.ChangedColumns, 1)
		assert.Equal(t, "email", diff.ChangedColumns[0].After.Name)
	})

	t.Run("detects mixed foreign key changes", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID"},
				{Name: "org_id", DataType: "UUID"},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "role_id", RefTable: "roles", RefColumn: "id", OnDelete: "CASCADE"},
				{Column: "removed_fk", RefTable: "old_table", RefColumn: "id"},
			},
		}
		desired := schema.Table{
			Name: "users",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "role_id", DataType: "UUID"},
				{Name: "org_id", DataType: "UUID"},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "role_id", RefTable: "roles", RefColumn: "id", OnDelete: "NO ACTION"},
				{Column: "added_fk", RefTable: "new_table", RefColumn: "id"},
			},
		}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, diff.ChangedForeignKeys, 1)
		assert.Equal(t, "role_id", diff.ChangedForeignKeys[0].Before.Column)
		assert.Len(t, diff.RemovedForeignKeys, 1)
		assert.Equal(t, "removed_fk", diff.RemovedForeignKeys[0].Column)
		assert.Len(t, diff.AddedForeignKeys, 1)
		assert.Equal(t, "added_fk", diff.AddedForeignKeys[0].Column)
	})

	t.Run("warns on potential column rename", func(t *testing.T) {
		// ===== Arrange ===== //
		removed := []schema.Column{{Name: "old_name", DataType: "TEXT"}}
		added := []schema.Column{{Name: "new_name", DataType: "TEXT"}}

		// ===== Act ===== //
		output := captureStderr(t, func() {
			warnColumnRename(removed, added)
		})

		// ===== Assert ===== //
		assert.Contains(t, output, `column "old_name" removed and "new_name" added with same type "TEXT"`)
	})

	t.Run("warns on potential FK column rename", func(t *testing.T) {
		// ===== Arrange ===== //
		removed := []schema.Column{{Name: "old_fk", DataType: "UUID"}}
		added := []schema.Column{{Name: "new_fk", DataType: "UUID"}}

		// ===== Act ===== //
		output := captureStderr(t, func() {
			warnColumnRename(removed, added)
		})

		// ===== Assert ===== //
		assert.Contains(t, output, `column "old_fk" removed and "new_fk" added with same type "UUID"`)
	})

	t.Run("does not warn on different types", func(t *testing.T) {
		// ===== Arrange ===== //
		removed := []schema.Column{{Name: "old_name", DataType: "TEXT"}}
		added := []schema.Column{{Name: "new_name", DataType: "INTEGER"}}

		// ===== Act ===== //
		output := captureStderr(t, func() {
			warnColumnRename(removed, added)
		})

		// ===== Assert ===== //
		assert.Contains(t, output, `column "old_name" dropped`)
	})

	t.Run("rejects composite primary key changes", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "memberships", PrimaryKey: []string{"team", "user"}}
		desired := schema.Table{Name: "memberships", PrimaryKey: []string{"team", "role"}}

		// ===== Act ===== //
		_, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary key change")
	})

	t.Run("detects added composite unique constraint", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "memberships"}
		desired := schema.Table{Name: "memberships", Uniques: [][]string{{"team", "user"}}}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"team", "user"}}, diff.AddedUniques)
		assert.Empty(t, diff.RemovedUniques)
		assert.False(t, diff.Empty())
	})

	t.Run("detects removed composite unique constraint", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "memberships", Uniques: [][]string{{"team", "user"}}}
		desired := schema.Table{Name: "memberships"}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"team", "user"}}, diff.RemovedUniques)
		assert.Empty(t, diff.AddedUniques)
	})

	t.Run("column-set change on a composite unique diffs as remove+add", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "memberships", Uniques: [][]string{{"team", "user"}}}
		desired := schema.Table{Name: "memberships", Uniques: [][]string{{"team", "role"}}}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"team", "role"}}, diff.AddedUniques)
		assert.Equal(t, [][]string{{"team", "user"}}, diff.RemovedUniques)
	})

	t.Run("detects added index", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "users"}
		desired := schema.Table{Name: "users", Indexes: [][]string{{"email"}}}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"email"}}, diff.AddedIndexes)
		assert.Empty(t, diff.RemovedIndexes)
		assert.False(t, diff.Empty())
	})

	t.Run("detects removed index", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "users", Indexes: [][]string{{"email"}}}
		desired := schema.Table{Name: "users"}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"email"}}, diff.RemovedIndexes)
		assert.Empty(t, diff.AddedIndexes)
	})

	t.Run("column-set change on a composite index diffs as remove+add", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "events", Indexes: [][]string{{"tenant_id", "happened"}}}
		desired := schema.Table{Name: "events", Indexes: [][]string{{"tenant_id", "kind"}}}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"tenant_id", "kind"}}, diff.AddedIndexes)
		assert.Equal(t, [][]string{{"tenant_id", "happened"}}, diff.RemovedIndexes)
	})

	t.Run("detects added trgm index", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "notes"}
		desired := schema.Table{Name: "notes", TrgmIndexes: []string{"body"}}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []string{"body"}, diff.AddedTrgmIndexes)
		assert.Empty(t, diff.RemovedTrgmIndexes)
		assert.False(t, diff.Empty())
	})

	t.Run("detects removed trgm index", func(t *testing.T) {
		// ===== Arrange ===== //
		previous := schema.Table{Name: "notes", TrgmIndexes: []string{"body"}}
		desired := schema.Table{Name: "notes"}

		// ===== Act ===== //
		diff, err := Compute(previous, desired)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []string{"body"}, diff.RemovedTrgmIndexes)
		assert.Empty(t, diff.AddedTrgmIndexes)
	})
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = original

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	return buf.String()
}
