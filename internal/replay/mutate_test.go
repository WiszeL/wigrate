package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiszel/wigrate/internal/schema"
)

func TestReplayColumnAlter(t *testing.T) {
	t.Run("apply", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			name   string
			schema *schema.Table
			sql    string
			check  func(t *testing.T, s *schema.Table)
		}
		tests := []tc{
			{
				name:   "type change",
				schema: &schema.Table{Name: "t", Columns: []schema.Column{{Name: "e", DataType: "TEXT", NotNull: true}}},
				sql:    "e TYPE VARCHAR(20)",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "e", DataType: "VARCHAR(20)", NotNull: true}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
			{
				name:   "set not null",
				schema: &schema.Table{Name: "t", Columns: []schema.Column{{Name: "a", DataType: "INTEGER"}}},
				sql:    "a SET NOT NULL",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "a", DataType: "INTEGER", NotNull: true}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
			{
				name:   "drop not null",
				schema: &schema.Table{Name: "t", Columns: []schema.Column{{Name: "a", DataType: "INTEGER", NotNull: true}}},
				sql:    "a DROP NOT NULL",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "a", DataType: "INTEGER", NotNull: false}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
			{
				name:   "no-op unknown column",
				schema: &schema.Table{Name: "t", Columns: []schema.Column{{Name: "e", DataType: "TEXT"}}},
				sql:    "nonexist TYPE VARCHAR(20)",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "e", DataType: "TEXT"}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			applyGeneratedColumnAlter(tt.schema, tt.sql)

			// ===== Assert ===== //
			tt.check(t, tt.schema)
		}
	})
}

func TestReplayHelpers(t *testing.T) {
	t.Run("columns", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			setup  func() *schema.Table
			action func(*schema.Table)
			check  func(t *testing.T, s *schema.Table)
		}
		newColumn := schema.Column{Name: "e"}
		existingID := schema.Column{Name: "id"}
		tests := []tc{
			{
				setup:  func() *schema.Table { return &schema.Table{Name: "t"} },
				action: func(s *schema.Table) { appendColumnIfMissing(s, newColumn) },
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.Columns, 1)
					assert.Equal(t, newColumn.Name, s.Columns[0].Name)
					assert.Equal(t, newColumn.DataType, s.Columns[0].DataType)
					assert.Equal(t, newColumn.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, newColumn.Primary, s.Columns[0].Primary)
					assert.Equal(t, newColumn.Unique, s.Columns[0].Unique)
				},
			},
			{
				setup:  func() *schema.Table { return &schema.Table{Name: "t", Columns: []schema.Column{newColumn}} },
				action: func(s *schema.Table) { appendColumnIfMissing(s, newColumn) },
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.Columns, 1)
					assert.Equal(t, newColumn.Name, s.Columns[0].Name)
					assert.Equal(t, newColumn.DataType, s.Columns[0].DataType)
					assert.Equal(t, newColumn.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, newColumn.Primary, s.Columns[0].Primary)
					assert.Equal(t, newColumn.Unique, s.Columns[0].Unique)
				},
			},
			{
				setup: func() *schema.Table {
					return &schema.Table{
						Name:        "t",
						Columns:     []schema.Column{existingID, {Name: "rid"}},
						ForeignKeys: []schema.ForeignKey{{Column: "rid"}},
					}
				},
				action: func(s *schema.Table) { removeColumn(s, "rid") },
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.Columns, 1)
					assert.Equal(t, existingID.Name, s.Columns[0].Name)
					assert.Equal(t, existingID.DataType, s.Columns[0].DataType)
					assert.Equal(t, existingID.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, existingID.Primary, s.Columns[0].Primary)
					assert.Equal(t, existingID.Unique, s.Columns[0].Unique)
					assert.Empty(t, s.ForeignKeys)
				},
			},
			{
				setup:  func() *schema.Table { return &schema.Table{Name: "t", Columns: []schema.Column{existingID}} },
				action: func(s *schema.Table) { removeColumn(s, "nonexist") },
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.Columns, 1)
					assert.Equal(t, existingID.Name, s.Columns[0].Name)
					assert.Equal(t, existingID.DataType, s.Columns[0].DataType)
					assert.Equal(t, existingID.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, existingID.Primary, s.Columns[0].Primary)
					assert.Equal(t, existingID.Unique, s.Columns[0].Unique)
				},
			},
		}

		for _, tt := range tests {
			// ===== Arrange ===== //
			s := tt.setup()

			// ===== Act ===== //
			tt.action(s)

			// ===== Assert ===== //
			tt.check(t, s)
		}
	})

	t.Run("foreignKeys", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			setup  func() *schema.Table
			action func(*schema.Table)
			check  func(t *testing.T, s *schema.Table)
		}
		newFK := schema.ForeignKey{Column: "rid"}
		tests := []tc{
			{
				setup:  func() *schema.Table { return &schema.Table{Name: "t"} },
				action: func(s *schema.Table) { appendForeignKeyIfMissing(s, newFK) },
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.ForeignKeys, 1)
					assert.Equal(t, newFK.Column, s.ForeignKeys[0].Column)
					assert.Equal(t, newFK.RefTable, s.ForeignKeys[0].RefTable)
					assert.Equal(t, newFK.RefColumn, s.ForeignKeys[0].RefColumn)
					assert.Equal(t, newFK.OnDelete, s.ForeignKeys[0].OnDelete)
				},
			},
			{
				setup: func() *schema.Table {
					return &schema.Table{Name: "t", ForeignKeys: []schema.ForeignKey{newFK}}
				},
				action: func(s *schema.Table) { appendForeignKeyIfMissing(s, newFK) },
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.ForeignKeys, 1)
					assert.Equal(t, newFK.Column, s.ForeignKeys[0].Column)
					assert.Equal(t, newFK.RefTable, s.ForeignKeys[0].RefTable)
					assert.Equal(t, newFK.RefColumn, s.ForeignKeys[0].RefColumn)
					assert.Equal(t, newFK.OnDelete, s.ForeignKeys[0].OnDelete)
				},
			},
			{
				setup: func() *schema.Table {
					return &schema.Table{Name: "t", ForeignKeys: []schema.ForeignKey{{Column: "rid", RefTable: "roles"}}}
				},
				action: func(s *schema.Table) { removeForeignKeyByConstraintName(s, "fk_t_rid") },
				check:  func(t *testing.T, s *schema.Table) { assert.Empty(t, s.ForeignKeys) },
			},
			{
				setup: func() *schema.Table {
					return &schema.Table{Name: "t", ForeignKeys: []schema.ForeignKey{newFK}}
				},
				action: func(s *schema.Table) { removeForeignKey(s, "rid") },
				check:  func(t *testing.T, s *schema.Table) { assert.Empty(t, s.ForeignKeys) },
			},
		}

		for _, tt := range tests {
			// ===== Arrange ===== //
			s := tt.setup()

			// ===== Act ===== //
			tt.action(s)

			// ===== Assert ===== //
			tt.check(t, s)
		}
	})

	t.Run("uniques", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			setup  func() *schema.Table
			action func(*schema.Table)
			check  func(t *testing.T, s *schema.Table)
		}
		tests := []tc{
			{
				setup: func() *schema.Table {
					return &schema.Table{Name: "t", Columns: []schema.Column{{Name: "e", Unique: true}}}
				},
				action: func(s *schema.Table) { removeUniqueByConstraintName(s, "uq_t_e") },
				check:  func(t *testing.T, s *schema.Table) { assert.False(t, s.Columns[0].Unique) },
			},
			{
				setup:  func() *schema.Table { return &schema.Table{Name: "t", Columns: []schema.Column{{Name: "e"}}} },
				action: func(s *schema.Table) { applyGeneratedUniqueConstraint(s, []string{"e"}) },
				check:  func(t *testing.T, s *schema.Table) { assert.True(t, s.Columns[0].Unique) },
			},
		}

		for _, tt := range tests {
			// ===== Arrange ===== //
			s := tt.setup()

			// ===== Act ===== //
			tt.action(s)

			// ===== Assert ===== //
			tt.check(t, s)
		}
	})

	t.Run("update", func(t *testing.T) {
		// ===== Arrange ===== //
		s := &schema.Table{Name: "t", Columns: []schema.Column{{Name: "e", DataType: "TEXT"}}}

		// ===== Act ===== //
		updateColumn(s, "e", func(c *schema.Column) { c.DataType = "VARCHAR(20)" })

		// ===== Assert ===== //
		assert.Equal(t, "VARCHAR(20)", s.Columns[0].DataType)
	})
}
