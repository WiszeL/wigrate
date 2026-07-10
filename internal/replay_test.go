package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayColumn(t *testing.T) {
	t.Run("Parse", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			want columnSchema
			ok   bool
		}{
			{"id UUID", columnSchema{name: "id", dataType: "UUID"}, true},
			{"id UUID PRIMARY KEY", columnSchema{name: "id", dataType: "UUID", primary: true}, true},
			{"email TEXT NOT NULL", columnSchema{name: "email", dataType: "TEXT", notNull: true}, true},
			{"email VARCHAR(20) NOT NULL UNIQUE", columnSchema{name: "email", dataType: "VARCHAR(20)", notNull: true, unique: true}, true},
			{"score DOUBLE PRECISION", columnSchema{name: "score", dataType: "DOUBLE PRECISION"}, true},
			{"FOREIGN KEY (u) REFERENCES r(id)", columnSchema{}, false},
			{"id", columnSchema{}, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedColumn(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})
}

func TestReplayFK(t *testing.T) {
	t.Run("parseFK", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			want foreignKeySchema
			ok   bool
		}{
			{"FOREIGN KEY (u) REFERENCES r(id)", foreignKeySchema{column: "u", refTable: "r", refColumn: "id"}, true},
			{"FOREIGN KEY (u) REFERENCES r(id) ON DELETE CASCADE", foreignKeySchema{column: "u", refTable: "r", refColumn: "id", onDelete: "CASCADE"}, true},
			{"FOREIGN KEY (u)", foreignKeySchema{}, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedForeignKey(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})

	t.Run("parseAlterFK", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			want foreignKeySchema
			ok   bool
		}{
			{"ADD CONSTRAINT x FOREIGN KEY (r) REFERENCES roles(id)", foreignKeySchema{column: "r", refTable: "roles", refColumn: "id"}, true},
			{"ADD CONSTRAINT x UNIQUE (e)", foreignKeySchema{}, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedAlterForeignKey(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})
}

func TestReplayUniqueConstraint(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			cols []string
			ok   bool
		}{
			{"ADD CONSTRAINT uq UNIQUE (email)", []string{"email"}, true},
			{"ADD CONSTRAINT uq UNIQUE (a, b)", []string{"a", "b"}, true},
			{"ADD CONSTRAINT uq UNIQUE ()", nil, false},
			{"bad", nil, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedUniqueConstraint(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.cols, got)
			}
		}
	})
}

func TestReplayColumnAlter(t *testing.T) {
	t.Run("apply", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			name   string
			schema *tableSchema
			sql    string
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{
				name:   "type change",
				schema: &tableSchema{name: "t", columns: []columnSchema{{name: "e", dataType: "TEXT", notNull: true}}},
				sql:    "e TYPE VARCHAR(20)",
				check:  func(t *testing.T, s *tableSchema) { assert.Equal(t, "VARCHAR(20)", s.columns[0].dataType) },
			},
			{
				name:   "set not null",
				schema: &tableSchema{name: "t", columns: []columnSchema{{name: "a", dataType: "INTEGER"}}},
				sql:    "a SET NOT NULL",
				check:  func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].notNull) },
			},
			{
				name:   "drop not null",
				schema: &tableSchema{name: "t", columns: []columnSchema{{name: "a", dataType: "INTEGER", notNull: true}}},
				sql:    "a DROP NOT NULL",
				check:  func(t *testing.T, s *tableSchema) { assert.False(t, s.columns[0].notNull) },
			},
			{
				name:   "no-op unknown column",
				schema: &tableSchema{name: "t", columns: []columnSchema{{name: "e", dataType: "TEXT"}}},
				sql:    "nonexist TYPE VARCHAR(20)",
				check:  func(t *testing.T, s *tableSchema) { assert.Equal(t, "TEXT", s.columns[0].dataType) },
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
		s := &tableSchema{name: "users"}

		// ===== Act ===== //
		applyGeneratedSQL(s, `CREATE TABLE users (
				id UUID PRIMARY KEY,
				email VARCHAR(20) NOT NULL UNIQUE,
				user_id UUID NOT NULL,
				CONSTRAINT fk_users_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		`)

		// ===== Assert ===== //
		assert.Len(t, s.columns, 3)
		assert.Equal(t, columnSchema{name: "id", dataType: "UUID", primary: true}, s.columns[0])
		assert.Equal(t, columnSchema{name: "email", dataType: "VARCHAR(20)", notNull: true, unique: true}, s.columns[1])
		assert.Equal(t, columnSchema{name: "user_id", dataType: "UUID", notNull: true}, s.columns[2])
		assert.Len(t, s.foreignKeys, 1)
		assert.Equal(t, foreignKeySchema{column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"}, s.foreignKeys[0])
	})

	t.Run("alterTable", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			name  string
			setup func() *tableSchema
			sql   string
			check func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{
				name:  "add column",
				setup: func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "id"}}} },
				sql:   "ALTER TABLE users\n    ADD COLUMN email VARCHAR(20) NOT NULL;\n",
				check: func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 2) },
			},
			{
				name: "drop column + FK cleanup",
				setup: func() *tableSchema {
					return &tableSchema{
						name:        "users",
						columns:     []columnSchema{{name: "id"}, {name: "r"}},
						foreignKeys: []foreignKeySchema{{column: "r"}},
					}
				},
				sql:   "ALTER TABLE users\n    DROP COLUMN IF EXISTS r;\n",
				check: func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1); assert.Empty(t, s.foreignKeys) },
			},
			{
				name: "alter column type",
				setup: func() *tableSchema {
					return &tableSchema{name: "users", columns: []columnSchema{{name: "e", dataType: "TEXT"}}}
				},
				sql:   "ALTER TABLE users\n    ALTER COLUMN e TYPE VARCHAR(20);\n",
				check: func(t *testing.T, s *tableSchema) { assert.Equal(t, "VARCHAR(20)", s.columns[0].dataType) },
			},
			{
				name:  "set not null",
				setup: func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "e"}}} },
				sql:   "ALTER TABLE users\n    ALTER COLUMN e SET NOT NULL;\n",
				check: func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].notNull) },
			},
			{
				name:  "add unique",
				setup: func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "e"}}} },
				sql:   "ALTER TABLE users\n    ADD CONSTRAINT uq_users_e UNIQUE (e);\n",
				check: func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].unique) },
			},
			{
				name: "drop unique",
				setup: func() *tableSchema {
					return &tableSchema{name: "users", columns: []columnSchema{{name: "e", unique: true}}}
				},
				sql:   "ALTER TABLE users\n    DROP CONSTRAINT IF EXISTS uq_users_e;\n",
				check: func(t *testing.T, s *tableSchema) { assert.False(t, s.columns[0].unique) },
			},
			{
				name:  "add FK",
				setup: func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "rid"}}} },
				sql:   "ALTER TABLE users\n    ADD CONSTRAINT fk_users_rid FOREIGN KEY (rid) REFERENCES roles(id) ON DELETE CASCADE;\n",
				check: func(t *testing.T, s *tableSchema) {
					assert.Len(t, s.foreignKeys, 1)
					assert.Equal(t, "CASCADE", s.foreignKeys[0].onDelete)
				},
			},
			{
				name: "drop FK",
				setup: func() *tableSchema {
					return &tableSchema{name: "users", foreignKeys: []foreignKeySchema{{column: "rid", refTable: "roles"}}}
				},
				sql:   "ALTER TABLE users\n    DROP CONSTRAINT IF EXISTS fk_users_rid;\n",
				check: func(t *testing.T, s *tableSchema) { assert.Empty(t, s.foreignKeys) },
			},
			{
				name:  "unrecognized line",
				setup: func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "id"}}} },
				sql:   "ALTER TABLE users\n    BOGUS;\n",
				check: func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) },
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
			module := migrationModule{migrationDir: dir}
			entries, err := os.ReadDir(module.migrationDir)
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
			before   *migrationFile
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
				before:   &migrationFile{baseName: "000002_alter_name_user", direction: "up"},
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
			module := migrationModule{migrationDir: dir}
			entries, err := os.ReadDir(module.migrationDir)
			require.NoError(t, err)
			s, err := readGeneratedSchema(module, entries, "user", tt.before)

			// ===== Assert ===== //
			assert.NoError(t, err)
			assert.Len(t, s.columns, tt.colCount)
		}
	})
}

func TestReplayHelpers(t *testing.T) {
	t.Run("columns", func(t *testing.T) {
		// ===== Arrange ===== //
		type tc struct {
			setup  func() *tableSchema
			action func(*tableSchema)
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{
				setup:  func() *tableSchema { return &tableSchema{name: "t"} },
				action: func(s *tableSchema) { appendColumnIfMissing(s, columnSchema{name: "e"}) },
				check:  func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) },
			},
			{
				setup:  func() *tableSchema { return &tableSchema{name: "t", columns: []columnSchema{{name: "e"}}} },
				action: func(s *tableSchema) { appendColumnIfMissing(s, columnSchema{name: "e"}) },
				check:  func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) },
			},
			{
				setup: func() *tableSchema {
					return &tableSchema{
						name:        "t",
						columns:     []columnSchema{{name: "id"}, {name: "rid"}},
						foreignKeys: []foreignKeySchema{{column: "rid"}},
					}
				},
				action: func(s *tableSchema) { removeColumn(s, "rid") },
				check:  func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1); assert.Empty(t, s.foreignKeys) },
			},
			{
				setup:  func() *tableSchema { return &tableSchema{name: "t", columns: []columnSchema{{name: "id"}}} },
				action: func(s *tableSchema) { removeColumn(s, "nonexist") },
				check:  func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) },
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
			setup  func() *tableSchema
			action func(*tableSchema)
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{
				setup:  func() *tableSchema { return &tableSchema{name: "t"} },
				action: func(s *tableSchema) { appendForeignKeyIfMissing(s, foreignKeySchema{column: "rid"}) },
				check:  func(t *testing.T, s *tableSchema) { assert.Len(t, s.foreignKeys, 1) },
			},
			{
				setup:  func() *tableSchema { return &tableSchema{name: "t", foreignKeys: []foreignKeySchema{{column: "rid"}}} },
				action: func(s *tableSchema) { appendForeignKeyIfMissing(s, foreignKeySchema{column: "rid"}) },
				check:  func(t *testing.T, s *tableSchema) { assert.Len(t, s.foreignKeys, 1) },
			},
			{
				setup: func() *tableSchema {
					return &tableSchema{name: "t", foreignKeys: []foreignKeySchema{{column: "rid", refTable: "roles"}}}
				},
				action: func(s *tableSchema) { removeForeignKeyByConstraintName(s, "fk_t_rid") },
				check:  func(t *testing.T, s *tableSchema) { assert.Empty(t, s.foreignKeys) },
			},
			{
				setup:  func() *tableSchema { return &tableSchema{name: "t", foreignKeys: []foreignKeySchema{{column: "rid"}}} },
				action: func(s *tableSchema) { removeForeignKey(s, "rid") },
				check:  func(t *testing.T, s *tableSchema) { assert.Empty(t, s.foreignKeys) },
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
			setup  func() *tableSchema
			action func(*tableSchema)
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{
				setup: func() *tableSchema {
					return &tableSchema{name: "t", columns: []columnSchema{{name: "e", unique: true}}}
				},
				action: func(s *tableSchema) { removeUniqueByConstraintName(s, "uq_t_e") },
				check:  func(t *testing.T, s *tableSchema) { assert.False(t, s.columns[0].unique) },
			},
			{
				setup:  func() *tableSchema { return &tableSchema{name: "t", columns: []columnSchema{{name: "e"}}} },
				action: func(s *tableSchema) { applyGeneratedUniqueConstraint(s, []string{"e"}) },
				check:  func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].unique) },
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
		s := &tableSchema{name: "t", columns: []columnSchema{{name: "e", dataType: "TEXT"}}}

		// ===== Act ===== //
		updateColumn(s, "e", func(c *columnSchema) { c.dataType = "VARCHAR(20)" })

		// ===== Assert ===== //
		assert.Equal(t, "VARCHAR(20)", s.columns[0].dataType)
	})
}

// TestReplayRoundTrip proves the replay parser correctly round-trips every SQL form
// that generate.go emits. These tests catch the class of bug where generation and
// replay diverge silently, causing phantom ALTER migrations on every run.
func TestReplayRoundTrip(t *testing.T) {
	t.Run("B1: inline FK in CREATE TABLE is replayed", func(t *testing.T) {
		// buildCreateTableSQL emits CONSTRAINT fk_<table>_<col> FOREIGN KEY (...) inline.
		// Replay must parse this and recover the FK; otherwise the next alter re-emits it.
		// ===== Arrange ===== //
		s := &tableSchema{name: "orders"}

		// ===== Act ===== //
		applyGeneratedSQL(s, "CREATE TABLE orders (\n"+
			"    id UUID PRIMARY KEY,\n"+
			"    user_id UUID NOT NULL,\n"+
			"    CONSTRAINT fk_orders_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE\n"+
			");\n")

		// ===== Assert ===== //
		assert.Len(t, s.columns, 2)
		assert.Len(t, s.foreignKeys, 1)
		assert.Equal(t, foreignKeySchema{column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"}, s.foreignKeys[0])
	})

	t.Run("B2: nullable UNIQUE column type is not corrupted", func(t *testing.T) {
		// buildColumnDefinition for a nullable unique column emits "email TEXT UNIQUE".
		// The type-accumulation loop must stop at UNIQUE, not include it in dataType.
		// ===== Arrange ===== //
		s := &tableSchema{name: "users"}

		// ===== Act ===== //
		applyGeneratedSQL(s, "CREATE TABLE users (\n    email TEXT UNIQUE\n);\n")

		// ===== Assert ===== //
		assert.Len(t, s.columns, 1)
		assert.Equal(t, "TEXT", s.columns[0].dataType)
		assert.True(t, s.columns[0].unique)
	})

	t.Run("B3: two FKs to same table get distinct constraint names", func(t *testing.T) {
		// fk_<table>_<column> — column-keyed — must be unique even when refTable is the same.
		// ===== Arrange ===== //
		s := &tableSchema{name: "things"}

		// ===== Act ===== //
		applyGeneratedSQL(s, "CREATE TABLE things (\n"+
			"    created_by_id UUID NOT NULL,\n"+
			"    updated_by_id UUID NOT NULL,\n"+
			"    CONSTRAINT fk_things_created_by_id FOREIGN KEY (created_by_id) REFERENCES users(id),\n"+
			"    CONSTRAINT fk_things_updated_by_id FOREIGN KEY (updated_by_id) REFERENCES users(id)\n"+
			");\n")

		// ===== Assert ===== //
		assert.Len(t, s.foreignKeys, 2)
		assert.Equal(t, "created_by_id", s.foreignKeys[0].column)
		assert.Equal(t, "updated_by_id", s.foreignKeys[1].column)

		// Removing by constraint name must target the correct FK.
		removeForeignKeyByConstraintName(s, "fk_things_created_by_id")
		assert.Len(t, s.foreignKeys, 1)
		assert.Equal(t, "updated_by_id", s.foreignKeys[0].column)
	})

	t.Run("B4: composite PRIMARY KEY and composite UNIQUE in CREATE TABLE are replayed", func(t *testing.T) {
		// buildCreateTableSQL emits table-level "PRIMARY KEY (a, b)" and
		// "CONSTRAINT uq_... UNIQUE (a, b)" for composites. Replay must recover both
		// without setting the per-column primary/unique bools.
		// ===== Arrange ===== //
		s := &tableSchema{name: "memberships"}

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
		assert.Equal(t, []string{"team", "user"}, s.primaryKey)
		assert.Equal(t, [][]string{{"role", "label"}}, s.uniques)
		for _, column := range s.columns {
			assert.False(t, column.primary)
			assert.False(t, column.unique)
		}
	})

	t.Run("B5: DROP CONSTRAINT removes a composite unique group", func(t *testing.T) {
		// The alter-generated DROP CONSTRAINT IF EXISTS must also match composite
		// unique constraints, not just single-column ones.
		// ===== Arrange ===== //
		s := &tableSchema{name: "memberships", uniques: [][]string{{"role", "label"}}}

		// ===== Act ===== //
		applyGeneratedSQL(s, "ALTER TABLE memberships\n    DROP CONSTRAINT IF EXISTS uq_memberships_role_label;\n")

		// ===== Assert ===== //
		assert.Empty(t, s.uniques)
	})
}
