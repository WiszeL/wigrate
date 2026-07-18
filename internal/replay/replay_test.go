package replay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiszel/wigrate/internal/discover"
	"github.com/wiszel/wigrate/internal/schema"
)

func TestReplayLineSkip(t *testing.T) {
	t.Run("shouldSkip", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			skip bool
		}{
			{"", true},
			{"CREATE TABLE x (", true},
			{"ALTER TABLE x", true},
			{")", true},
			{"id UUID PRIMARY KEY", false},
			{"FOREIGN KEY (u) REFERENCES r(id)", false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got := shouldSkipGeneratedSQLLine(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.skip, got)
		}
	})

	t.Run("clean", func(t *testing.T) {
		// ===== Arrange ===== //
		dirty := "  id UUID NOT NULL,"

		// ===== Act ===== //
		cleaned := cleanGeneratedSQLLine(dirty)

		// ===== Assert ===== //
		assert.Equal(t, "id UUID NOT NULL", cleaned)
	})
}

func TestReplayApplySQL(t *testing.T) {
	t.Run("createTable", func(t *testing.T) {
		// ===== Arrange ===== //
		s := &schema.Table{Name: "users"}

		// ===== Act ===== //
		applyGeneratedSQL(s, `CREATE TABLE users (
				id UUID PRIMARY KEY,
				email VARCHAR(20) NOT NULL UNIQUE,
				user_id UUID NOT NULL,
				CONSTRAINT fk_users_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		`)

		// ===== Assert ===== //
		assert.Len(t, s.Columns, 3)
		assert.Equal(t, schema.Column{Name: "id", DataType: "UUID", Primary: true}, s.Columns[0])
		assert.Equal(t, schema.Column{Name: "email", DataType: "VARCHAR(20)", NotNull: true, Unique: true}, s.Columns[1])
		assert.Equal(t, schema.Column{Name: "user_id", DataType: "UUID", NotNull: true}, s.Columns[2])
		assert.Len(t, s.ForeignKeys, 1)
		assert.Equal(t, schema.ForeignKey{Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"}, s.ForeignKeys[0])
	})

	t.Run("alterTable", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			name  string
			setup func() *schema.Table
			sql   string
			check func(t *testing.T, s *schema.Table)
		}
		idColumn := schema.Column{Name: "id"}
		tests := []tc{
			{
				name:  "add column",
				setup: func() *schema.Table { return &schema.Table{Name: "users", Columns: []schema.Column{idColumn}} },
				sql:   "ALTER TABLE users\n    ADD COLUMN email VARCHAR(20) NOT NULL;\n",
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.Columns, 2)
					assert.Equal(t, idColumn.Name, s.Columns[0].Name)
					assert.Equal(t, idColumn.DataType, s.Columns[0].DataType)
					assert.Equal(t, idColumn.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, idColumn.Primary, s.Columns[0].Primary)
					assert.Equal(t, idColumn.Unique, s.Columns[0].Unique)
					expectedEmail := schema.Column{Name: "email", DataType: "VARCHAR(20)", NotNull: true}
					assert.Equal(t, expectedEmail.Name, s.Columns[1].Name)
					assert.Equal(t, expectedEmail.DataType, s.Columns[1].DataType)
					assert.Equal(t, expectedEmail.NotNull, s.Columns[1].NotNull)
					assert.Equal(t, expectedEmail.Primary, s.Columns[1].Primary)
					assert.Equal(t, expectedEmail.Unique, s.Columns[1].Unique)
				},
			},
			{
				name: "drop column + FK cleanup",
				setup: func() *schema.Table {
					return &schema.Table{
						Name:        "users",
						Columns:     []schema.Column{idColumn, {Name: "r"}},
						ForeignKeys: []schema.ForeignKey{{Column: "r"}},
					}
				},
				sql: "ALTER TABLE users\n    DROP COLUMN IF EXISTS r;\n",
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.Columns, 1)
					assert.Equal(t, idColumn.Name, s.Columns[0].Name)
					assert.Equal(t, idColumn.DataType, s.Columns[0].DataType)
					assert.Equal(t, idColumn.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, idColumn.Primary, s.Columns[0].Primary)
					assert.Equal(t, idColumn.Unique, s.Columns[0].Unique)
					assert.Empty(t, s.ForeignKeys)
				},
			},
			{
				name: "alter column type",
				setup: func() *schema.Table {
					return &schema.Table{Name: "users", Columns: []schema.Column{{Name: "e", DataType: "TEXT"}}}
				},
				sql: "ALTER TABLE users\n    ALTER COLUMN e TYPE VARCHAR(20);\n",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "e", DataType: "VARCHAR(20)"}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
			{
				name:  "set not null",
				setup: func() *schema.Table { return &schema.Table{Name: "users", Columns: []schema.Column{{Name: "e"}}} },
				sql:   "ALTER TABLE users\n    ALTER COLUMN e SET NOT NULL;\n",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "e", NotNull: true}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
			{
				name:  "add unique",
				setup: func() *schema.Table { return &schema.Table{Name: "users", Columns: []schema.Column{{Name: "e"}}} },
				sql:   "ALTER TABLE users\n    ADD CONSTRAINT uq_users_e UNIQUE (e);\n",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "e", Unique: true}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
			{
				name: "drop unique",
				setup: func() *schema.Table {
					return &schema.Table{Name: "users", Columns: []schema.Column{{Name: "e", Unique: true}}}
				},
				sql: "ALTER TABLE users\n    DROP CONSTRAINT IF EXISTS uq_users_e;\n",
				check: func(t *testing.T, s *schema.Table) {
					expected := schema.Column{Name: "e"}
					assert.Equal(t, expected.Name, s.Columns[0].Name)
					assert.Equal(t, expected.DataType, s.Columns[0].DataType)
					assert.Equal(t, expected.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, expected.Primary, s.Columns[0].Primary)
					assert.Equal(t, expected.Unique, s.Columns[0].Unique)
				},
			},
			{
				name:  "add FK",
				setup: func() *schema.Table { return &schema.Table{Name: "users", Columns: []schema.Column{{Name: "rid"}}} },
				sql:   "ALTER TABLE users\n    ADD CONSTRAINT fk_users_rid FOREIGN KEY (rid) REFERENCES roles(id) ON DELETE CASCADE;\n",
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.ForeignKeys, 1)
					expected := schema.ForeignKey{Column: "rid", RefTable: "roles", RefColumn: "id", OnDelete: "CASCADE"}
					assert.Equal(t, expected.Column, s.ForeignKeys[0].Column)
					assert.Equal(t, expected.RefTable, s.ForeignKeys[0].RefTable)
					assert.Equal(t, expected.RefColumn, s.ForeignKeys[0].RefColumn)
					assert.Equal(t, expected.OnDelete, s.ForeignKeys[0].OnDelete)
				},
			},
			{
				name: "drop FK",
				setup: func() *schema.Table {
					return &schema.Table{Name: "users", ForeignKeys: []schema.ForeignKey{{Column: "rid", RefTable: "roles"}}}
				},
				sql:   "ALTER TABLE users\n    DROP CONSTRAINT IF EXISTS fk_users_rid;\n",
				check: func(t *testing.T, s *schema.Table) { assert.Empty(t, s.ForeignKeys) },
			},
			{
				name:  "unrecognized line",
				setup: func() *schema.Table { return &schema.Table{Name: "users", Columns: []schema.Column{idColumn}} },
				sql:   "ALTER TABLE users\n    BOGUS;\n",
				check: func(t *testing.T, s *schema.Table) {
					require.Len(t, s.Columns, 1)
					assert.Equal(t, idColumn.Name, s.Columns[0].Name)
					assert.Equal(t, idColumn.DataType, s.Columns[0].DataType)
					assert.Equal(t, idColumn.NotNull, s.Columns[0].NotNull)
					assert.Equal(t, idColumn.Primary, s.Columns[0].Primary)
					assert.Equal(t, idColumn.Unique, s.Columns[0].Unique)
				},
			},
		}

		for _, tt := range tests {
			// ===== Arrange ===== //
			s := tt.setup()

			// ===== Act ===== //
			applyGeneratedSQL(s, tt.sql)

			// ===== Assert ===== //
			tt.check(t, s)
		}
	})
}

func TestReplayFindFiles(t *testing.T) {
	t.Run("cases", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			setup func(t *testing.T) string
			count int
		}{
			{
				setup: func(t *testing.T) string {
					dir := t.TempDir()
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_u.up.sql"), []byte(""), 0644))
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_u.down.sql"), []byte(""), 0644))
					return dir
				},
				count: 2,
			},
			{
				setup: func(t *testing.T) string {
					dir := t.TempDir()
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "random.txt"), []byte(""), 0644))
					return dir
				},
				count: 0,
			},
		}

		for _, tt := range tests {
			// ===== Arrange ===== //
			dir := tt.setup(t)

			// ===== Act ===== //
			module := discover.Module{MigrationDir: dir}
			entries, err := os.ReadDir(module.MigrationDir)
			require.NoError(t, err)
			files := findEntityMigrationFiles(module, entries, "u")

			// ===== Assert ===== //
			assert.Len(t, files, tt.count)
		}
	})
}

func TestReplayReadSchema(t *testing.T) {
	t.Run("cases", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			name     string
			before   *discover.File
			colCount int
			setup    func(t *testing.T) string
		}
		tests := []tc{
			{
				name:     "no history",
				before:   nil,
				colCount: 0,
				setup:    func(t *testing.T) string { return t.TempDir() },
			},
			{
				name:     "init migration",
				before:   nil,
				colCount: 1,
				setup: func(t *testing.T) string {
					dir := t.TempDir()
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte("CREATE TABLE users (\n    id UUID PRIMARY KEY\n);\n"), 0644))
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.down.sql"), []byte(""), 0644))
					return dir
				},
			},
			{
				name:     "init + alter",
				before:   nil,
				colCount: 2,
				setup: func(t *testing.T) string {
					dir := t.TempDir()
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte("CREATE TABLE users (\n    id UUID PRIMARY KEY\n);\n"), 0644))
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000002_alter_name_user.up.sql"), []byte("ALTER TABLE users\n    ADD COLUMN name VARCHAR(50) NOT NULL;\n"), 0644))
					return dir
				},
			},
			{
				name:     "stops before marker",
				before:   &discover.File{BaseName: "000002_alter_name_user", Direction: "up"},
				colCount: 1,
				setup: func(t *testing.T) string {
					dir := t.TempDir()
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte("CREATE TABLE users (\n    id UUID PRIMARY KEY\n);\n"), 0644))
					assert.NoError(t, os.WriteFile(filepath.Join(dir, "000002_alter_name_user.up.sql"), []byte("ALTER TABLE users\n    ADD COLUMN name VARCHAR(50) NOT NULL;\n"), 0644))
					return dir
				},
			},
		}

		for _, tt := range tests {
			// ===== Arrange ===== //
			dir := tt.setup(t)

			// ===== Act ===== //
			module := discover.Module{MigrationDir: dir}
			entries, err := os.ReadDir(module.MigrationDir)
			require.NoError(t, err)
			s, err := Read(module, entries, "user", tt.before)

			// ===== Assert ===== //
			assert.NoError(t, err)
			assert.Len(t, s.Columns, tt.colCount)
		}
	})
}

// TestReplayRoundTrip proves the replay parser correctly round-trips every SQL form
// that sqlgen emits. These tests catch the class of bug where generation and
// replay diverge silently, causing phantom ALTER migrations on every run.
func TestReplayRoundTrip(t *testing.T) {
	t.Run("B1: inline FK in CREATE TABLE is replayed", func(t *testing.T) {
		// sqlgen.CreateTableSQL emits CONSTRAINT fk_<table>_<col> FOREIGN KEY (...) inline.
		// Replay must parse this and recover the FK; otherwise the next alter re-emits it.
		// ===== Arrange ===== //
		s := &schema.Table{Name: "orders"}

		// ===== Act ===== //
		applyGeneratedSQL(s, "CREATE TABLE orders (\n"+
			"    id UUID PRIMARY KEY,\n"+
			"    user_id UUID NOT NULL,\n"+
			"    CONSTRAINT fk_orders_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE\n"+
			");\n")

		// ===== Assert ===== //
		assert.Len(t, s.Columns, 2)
		assert.Len(t, s.ForeignKeys, 1)
		assert.Equal(t, schema.ForeignKey{Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"}, s.ForeignKeys[0])
	})

	t.Run("B2: nullable UNIQUE column type is not corrupted", func(t *testing.T) {
		// buildColumnDefinition for a nullable unique column emits "email TEXT UNIQUE".
		// The type-accumulation loop must stop at UNIQUE, not include it in DataType.
		// ===== Arrange ===== //
		s := &schema.Table{Name: "users"}

		// ===== Act ===== //
		applyGeneratedSQL(s, "CREATE TABLE users (\n    email TEXT UNIQUE\n);\n")

		// ===== Assert ===== //
		assert.Len(t, s.Columns, 1)
		assert.Equal(t, "TEXT", s.Columns[0].DataType)
		assert.True(t, s.Columns[0].Unique)
	})

	t.Run("B3: two FKs to same table get distinct constraint names", func(t *testing.T) {
		// fk_<table>_<column> — column-keyed — must be unique even when refTable is the same.
		// ===== Arrange ===== //
		s := &schema.Table{Name: "things"}

		// ===== Act ===== //
		applyGeneratedSQL(s, "CREATE TABLE things (\n"+
			"    created_by_id UUID NOT NULL,\n"+
			"    updated_by_id UUID NOT NULL,\n"+
			"    CONSTRAINT fk_things_created_by_id FOREIGN KEY (created_by_id) REFERENCES users(id),\n"+
			"    CONSTRAINT fk_things_updated_by_id FOREIGN KEY (updated_by_id) REFERENCES users(id)\n"+
			");\n")

		// ===== Assert ===== //
		assert.Len(t, s.ForeignKeys, 2)
		assert.Equal(t, "created_by_id", s.ForeignKeys[0].Column)
		assert.Equal(t, "updated_by_id", s.ForeignKeys[1].Column)

		// Removing by constraint name must target the correct FK.
		removeForeignKeyByConstraintName(s, "fk_things_created_by_id")
		assert.Len(t, s.ForeignKeys, 1)
		assert.Equal(t, "updated_by_id", s.ForeignKeys[0].Column)
	})

	t.Run("B4: composite PRIMARY KEY and composite UNIQUE in CREATE TABLE are replayed", func(t *testing.T) {
		// sqlgen.CreateTableSQL emits table-level "PRIMARY KEY (a, b)" and
		// "CONSTRAINT uq_... UNIQUE (a, b)" for composites. Replay must recover both
		// without setting the per-column Primary/Unique bools.
		// ===== Arrange ===== //
		s := &schema.Table{Name: "memberships"}

		// ===== Act ===== //
		applyGeneratedSQL(s, "CREATE TABLE memberships (\n"+
			"    team UUID NOT NULL,\n"+
			"    user UUID NOT NULL,\n"+
			"    role TEXT NOT NULL,\n"+
			"    label TEXT NOT NULL,\n"+
			"    PRIMARY KEY (team, user),\n"+
			"    CONSTRAINT uq_memberships_role_label UNIQUE (role, label)\n"+
			");\n")

		// ===== Assert ===== //
		assert.Equal(t, []string{"team", "user"}, s.PrimaryKey)
		assert.Equal(t, [][]string{{"role", "label"}}, s.Uniques)
		for _, column := range s.Columns {
			assert.False(t, column.Primary)
			assert.False(t, column.Unique)
		}
	})

	t.Run("B5: DROP CONSTRAINT removes a composite unique group", func(t *testing.T) {
		// The alter-generated DROP CONSTRAINT IF EXISTS must also match composite
		// unique constraints, not just single-column ones.
		// ===== Arrange ===== //
		s := &schema.Table{Name: "memberships", Uniques: [][]string{{"role", "label"}}}

		// ===== Act ===== //
		applyGeneratedSQL(s, "ALTER TABLE memberships\n    DROP CONSTRAINT IF EXISTS uq_memberships_role_label;\n")

		// ===== Assert ===== //
		assert.Empty(t, s.Uniques)
	})
}
