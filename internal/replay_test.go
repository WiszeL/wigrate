package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplayColumn(t *testing.T) {
	t.Run("Parse", func(t *testing.T) {
		// Arrange
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
			// Act
			got, ok := parseGeneratedColumn(tt.in)

			// Assert
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})
}

func TestReplayFK(t *testing.T) {
	t.Run("parseFK", func(t *testing.T) {
		// Arrange
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
			// Act
			got, ok := parseGeneratedForeignKey(tt.in)

			// Assert
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})

	t.Run("parseAlterFK", func(t *testing.T) {
		// Arrange
		tests := []struct {
			in   string
			want foreignKeySchema
			ok   bool
		}{
			{"ADD CONSTRAINT x FOREIGN KEY (r) REFERENCES roles(id)", foreignKeySchema{column: "r", refTable: "roles", refColumn: "id"}, true},
			{"ADD CONSTRAINT x UNIQUE (e)", foreignKeySchema{}, false},
		}

		for _, tt := range tests {
			// Act
			got, ok := parseGeneratedAlterForeignKey(tt.in)

			// Assert
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})
}

func TestReplayUniqueConstraint(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		// Arrange
		tests := []struct {
			in  string
			col string
			ok  bool
		}{
			{"ADD CONSTRAINT uq UNIQUE (email)", "email", true},
			{"ADD CONSTRAINT uq UNIQUE ()", "", false},
			{"bad", "", false},
		}

		for _, tt := range tests {
			// Act
			got, ok := parseGeneratedUniqueConstraint(tt.in)

			// Assert
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.col, got)
			}
		}
	})
}

func TestReplayColumnAlter(t *testing.T) {
	t.Run("apply", func(t *testing.T) {
		// Arrange
		type tc struct {
			name   string
			schema *tableSchema
			sql    string
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{"type change", &tableSchema{name: "t", columns: []columnSchema{{name: "e", dataType: "TEXT", notNull: true}}}, "e TYPE VARCHAR(20)", func(t *testing.T, s *tableSchema) { assert.Equal(t, "VARCHAR(20)", s.columns[0].dataType) }},
			{"set not null", &tableSchema{name: "t", columns: []columnSchema{{name: "a", dataType: "INTEGER"}}}, "a SET NOT NULL", func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].notNull) }},
			{"drop not null", &tableSchema{name: "t", columns: []columnSchema{{name: "a", dataType: "INTEGER", notNull: true}}}, "a DROP NOT NULL", func(t *testing.T, s *tableSchema) { assert.False(t, s.columns[0].notNull) }},
			{"no-op unknown column", &tableSchema{name: "t", columns: []columnSchema{{name: "e", dataType: "TEXT"}}}, "nonexist TYPE VARCHAR(20)", func(t *testing.T, s *tableSchema) { assert.Equal(t, "TEXT", s.columns[0].dataType) }},
		}

		for _, tt := range tests {
			// Act
			applyGeneratedColumnAlter(tt.schema, tt.sql)

			// Assert
			tt.check(t, tt.schema)
		}
	})
}

func TestReplayLineSkip(t *testing.T) {
	t.Run("shouldSkip", func(t *testing.T) {
		// Arrange
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
			// Act
			got := shouldSkipGeneratedSQLLine(tt.in)

			// Assert
			assert.Equal(t, tt.skip, got)
		}
	})

	t.Run("clean", func(t *testing.T) {
		// Arrange
		dirty := "  id UUID NOT NULL,"

		// Act
		cleaned := cleanGeneratedSQLLine(dirty)

		// Assert
		assert.Equal(t, "id UUID NOT NULL", cleaned)
	})
}

func TestReplayApplySQL(t *testing.T) {
	t.Run("createTable", func(t *testing.T) {
		// Arrange
		s := &tableSchema{name: "users"}

		// Act
		applyGeneratedSQL(s, `CREATE TABLE users (
				id UUID PRIMARY KEY,
				email VARCHAR(20) NOT NULL UNIQUE,
				user_id UUID NOT NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		`)

		// Assert
		assert.Len(t, s.columns, 3)
		assert.Equal(t, columnSchema{name: "id", dataType: "UUID", primary: true}, s.columns[0])
		assert.Equal(t, columnSchema{name: "email", dataType: "VARCHAR(20)", notNull: true, unique: true}, s.columns[1])
		assert.Equal(t, columnSchema{name: "user_id", dataType: "UUID", notNull: true}, s.columns[2])
		assert.Len(t, s.foreignKeys, 1)
		assert.Equal(t, foreignKeySchema{column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"}, s.foreignKeys[0])
	})

	t.Run("alterTable", func(t *testing.T) {
		// Arrange
		type tc struct {
			name  string
			setup func() *tableSchema
			sql   string
			check func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{"add column", func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "id"}}} }, "ALTER TABLE users\n    ADD COLUMN email VARCHAR(20) NOT NULL;\n", func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 2) }},
			{"drop column + FK cleanup", func() *tableSchema {
				return &tableSchema{name: "users", columns: []columnSchema{{name: "id"}, {name: "r"}}, foreignKeys: []foreignKeySchema{{column: "r"}}}
			}, "ALTER TABLE users\n    DROP COLUMN IF EXISTS r;\n", func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1); assert.Empty(t, s.foreignKeys) }},
			{"alter column type", func() *tableSchema {
				return &tableSchema{name: "users", columns: []columnSchema{{name: "e", dataType: "TEXT"}}}
			}, "ALTER TABLE users\n    ALTER COLUMN e TYPE VARCHAR(20);\n", func(t *testing.T, s *tableSchema) { assert.Equal(t, "VARCHAR(20)", s.columns[0].dataType) }},
			{"set not null", func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "e"}}} }, "ALTER TABLE users\n    ALTER COLUMN e SET NOT NULL;\n", func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].notNull) }},
			{"add unique", func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "e"}}} }, "ALTER TABLE users\n    ADD CONSTRAINT uq_users_e UNIQUE (e);\n", func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].unique) }},
			{"drop unique", func() *tableSchema {
				return &tableSchema{name: "users", columns: []columnSchema{{name: "e", unique: true}}}
			}, "ALTER TABLE users\n    DROP CONSTRAINT IF EXISTS uq_users_e;\n", func(t *testing.T, s *tableSchema) { assert.False(t, s.columns[0].unique) }},
			{"add FK", func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "rid"}}} }, "ALTER TABLE users\n    ADD CONSTRAINT fk_users_roles FOREIGN KEY (rid) REFERENCES roles(id) ON DELETE CASCADE;\n", func(t *testing.T, s *tableSchema) {
				assert.Len(t, s.foreignKeys, 1)
				assert.Equal(t, "CASCADE", s.foreignKeys[0].onDelete)
			}},
			{"drop FK", func() *tableSchema {
				return &tableSchema{name: "users", foreignKeys: []foreignKeySchema{{column: "rid", refTable: "roles"}}}
			}, "ALTER TABLE users\n    DROP CONSTRAINT IF EXISTS fk_users_roles;\n", func(t *testing.T, s *tableSchema) { assert.Empty(t, s.foreignKeys) }},
			{"unrecognized line", func() *tableSchema { return &tableSchema{name: "users", columns: []columnSchema{{name: "id"}}} }, "ALTER TABLE users\n    BOGUS;\n", func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) }},
		}

		for _, tt := range tests {
			// Arrange
			s := tt.setup()

			// Act
			applyGeneratedSQL(s, tt.sql)

			// Assert
			tt.check(t, s)
		}
	})
}

func TestReplayFindFiles(t *testing.T) {
	t.Run("cases", func(t *testing.T) {
		// Arrange
		tests := []struct {
			setup func(t *testing.T) string
			count int
		}{
			{func(t *testing.T) string {
				dir := t.TempDir()
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_u.up.sql"), []byte(""), 0644))
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_u.down.sql"), []byte(""), 0644))
				return dir
			}, 2},
			{func(t *testing.T) string {
				dir := t.TempDir()
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "random.txt"), []byte(""), 0644))
				return dir
			}, 0},
		}

		for _, tt := range tests {
			// Arrange
			dir := tt.setup(t)

			// Act
			files, err := findEntityMigrationFiles(migrationModule{migrationDir: dir}, "u")

			// Assert
			assert.NoError(t, err)
			assert.Len(t, files, tt.count)
		}
	})
}

func TestReplayReadSchema(t *testing.T) {
	t.Run("cases", func(t *testing.T) {
		// Arrange
		type tc struct {
			name     string
			before   *migrationFile
			colCount int
			setup    func(t *testing.T) string
		}
		tests := []tc{
			{"no history", nil, 0, func(t *testing.T) string { return t.TempDir() }},
			{"init migration", nil, 1, func(t *testing.T) string {
				dir := t.TempDir()
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte("CREATE TABLE users (\n    id UUID PRIMARY KEY\n);\n"), 0644))
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.down.sql"), []byte(""), 0644))
				return dir
			}},
			{"init + alter", nil, 2, func(t *testing.T) string {
				dir := t.TempDir()
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte("CREATE TABLE users (\n    id UUID PRIMARY KEY\n);\n"), 0644))
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000002_alter_name_user.up.sql"), []byte("ALTER TABLE users\n    ADD COLUMN name VARCHAR(50) NOT NULL;\n"), 0644))
				return dir
			}},
			{"stops before marker", &migrationFile{baseName: "000002_alter_name_user", direction: "up"}, 1, func(t *testing.T) string {
				dir := t.TempDir()
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000001_init_user.up.sql"), []byte("CREATE TABLE users (\n    id UUID PRIMARY KEY\n);\n"), 0644))
				assert.NoError(t, os.WriteFile(filepath.Join(dir, "000002_alter_name_user.up.sql"), []byte("ALTER TABLE users\n    ADD COLUMN name VARCHAR(50) NOT NULL;\n"), 0644))
				return dir
			}},
		}

		for _, tt := range tests {
			// Arrange
			dir := tt.setup(t)

			// Act
			s, err := readGeneratedSchema(migrationModule{migrationDir: dir}, "user", tt.before)

			// Assert
			assert.NoError(t, err)
			assert.Len(t, s.columns, tt.colCount)
		}
	})
}

func TestReplayHelpers(t *testing.T) {
	t.Run("columns", func(t *testing.T) {
		// Arrange
		type tc struct {
			setup  func() *tableSchema
			action func(*tableSchema)
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{func() *tableSchema { return &tableSchema{name: "t"} }, func(s *tableSchema) { appendColumnIfMissing(s, columnSchema{name: "e"}) }, func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) }},
			{func() *tableSchema { return &tableSchema{name: "t", columns: []columnSchema{{name: "e"}}} }, func(s *tableSchema) { appendColumnIfMissing(s, columnSchema{name: "e"}) }, func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) }},
			{func() *tableSchema {
				return &tableSchema{name: "t", columns: []columnSchema{{name: "id"}, {name: "rid"}}, foreignKeys: []foreignKeySchema{{column: "rid"}}}
			}, func(s *tableSchema) { removeColumn(s, "rid") }, func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1); assert.Empty(t, s.foreignKeys) }},
			{func() *tableSchema { return &tableSchema{name: "t", columns: []columnSchema{{name: "id"}}} }, func(s *tableSchema) { removeColumn(s, "nonexist") }, func(t *testing.T, s *tableSchema) { assert.Len(t, s.columns, 1) }},
		}

		for _, tt := range tests {
			// Arrange
			s := tt.setup()

			// Act
			tt.action(s)

			// Assert
			tt.check(t, s)
		}
	})

	t.Run("foreignKeys", func(t *testing.T) {
		// Arrange
		type tc struct {
			setup  func() *tableSchema
			action func(*tableSchema)
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{func() *tableSchema { return &tableSchema{name: "t"} }, func(s *tableSchema) { appendForeignKeyIfMissing(s, foreignKeySchema{column: "rid"}) }, func(t *testing.T, s *tableSchema) { assert.Len(t, s.foreignKeys, 1) }},
			{func() *tableSchema { return &tableSchema{name: "t", foreignKeys: []foreignKeySchema{{column: "rid"}}} }, func(s *tableSchema) { appendForeignKeyIfMissing(s, foreignKeySchema{column: "rid"}) }, func(t *testing.T, s *tableSchema) { assert.Len(t, s.foreignKeys, 1) }},
			{func() *tableSchema {
				return &tableSchema{name: "t", foreignKeys: []foreignKeySchema{{column: "rid", refTable: "roles"}}}
			}, func(s *tableSchema) { removeForeignKeyByConstraintName(s, "fk_t_roles") }, func(t *testing.T, s *tableSchema) { assert.Empty(t, s.foreignKeys) }},
			{func() *tableSchema { return &tableSchema{name: "t", foreignKeys: []foreignKeySchema{{column: "rid"}}} }, func(s *tableSchema) { removeForeignKey(s, "rid") }, func(t *testing.T, s *tableSchema) { assert.Empty(t, s.foreignKeys) }},
		}

		for _, tt := range tests {
			// Arrange
			s := tt.setup()

			// Act
			tt.action(s)

			// Assert
			tt.check(t, s)
		}
	})

	t.Run("uniques", func(t *testing.T) {
		// Arrange
		type tc struct {
			setup  func() *tableSchema
			action func(*tableSchema)
			check  func(t *testing.T, s *tableSchema)
		}
		tests := []tc{
			{func() *tableSchema {
				return &tableSchema{name: "t", columns: []columnSchema{{name: "e", unique: true}}}
			}, func(s *tableSchema) { removeUniqueByConstraintName(s, "uq_t_e") }, func(t *testing.T, s *tableSchema) { assert.False(t, s.columns[0].unique) }},
			{func() *tableSchema { return &tableSchema{name: "t", columns: []columnSchema{{name: "e"}}} }, func(s *tableSchema) { applyGeneratedUniqueConstraint(s, "e", true) }, func(t *testing.T, s *tableSchema) { assert.True(t, s.columns[0].unique) }},
			{func() *tableSchema {
				return &tableSchema{name: "t", columns: []columnSchema{{name: "e", unique: true}}}
			}, func(s *tableSchema) { applyGeneratedUniqueConstraint(s, "e", false) }, func(t *testing.T, s *tableSchema) { assert.False(t, s.columns[0].unique) }},
		}

		for _, tt := range tests {
			// Arrange
			s := tt.setup()

			// Act
			tt.action(s)

			// Assert
			tt.check(t, s)
		}
	})

	t.Run("lookups", func(t *testing.T) {
		// Arrange
		type tc struct {
			setup func() (int, bool)
			idx   int
			found bool
		}
		tests := []tc{
			{func() (int, bool) { return findColumnIndex([]columnSchema{{name: "id"}, {name: "e"}}, "e") }, 1, true},
			{func() (int, bool) { return findColumnIndex([]columnSchema{{name: "id"}}, "missing") }, 0, false},
			{func() (int, bool) {
				return findForeignKeyIndex([]foreignKeySchema{{column: "uid"}, {column: "rid"}}, "rid")
			}, 1, true},
			{func() (int, bool) { return findForeignKeyIndex([]foreignKeySchema{{column: "uid"}}, "missing") }, 0, false},
		}

		for _, tt := range tests {
			// Act
			idx, found := tt.setup()

			// Assert
			assert.Equal(t, tt.found, found)
			if found {
				assert.Equal(t, tt.idx, idx)
			}
		}
	})

	t.Run("update", func(t *testing.T) {
		// Arrange
		s := &tableSchema{name: "t", columns: []columnSchema{{name: "e", dataType: "TEXT"}}}

		// Act
		updateColumn(s, "e", func(c *columnSchema) { c.dataType = "VARCHAR(20)" })

		// Assert
		assert.Equal(t, "VARCHAR(20)", s.columns[0].dataType)
	})
}
