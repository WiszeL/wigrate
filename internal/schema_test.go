package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Migration_ParseEntitySchema(t *testing.T) {
	t.Run("maps struct fields and inline comments to table schema", func(t *testing.T) {
		// ===== Arrange =====
		module := makeTestMigrationModule(t, "shop", "user_profile.go", `package entity

import (
	"time"

	"github.com/google/uuid"
)

type UserProfile struct {
	ID         uuid.UUID
	UserID     uuid.UUID // del:cascade
	CategoryID uuid.UUID // null ref:categories del:setnull
	Email      string    // 20 unique
	Bio        string    // null
	CreatedAt  time.Time
	private    string
}
`)

		// ===== Act =====
		schema, err := parseEntitySchema(module, "user_profile")

		// ===== Assert =====
		require.NoError(t, err)

		assert.Equal(t, "user_profiles", schema.name)
		assert.Equal(t, []columnSchema{
			{name: "id", dataType: "UUID", primary: true},
			{name: "user_id", dataType: "UUID", notNull: true},
			{name: "category_id", dataType: "UUID"},
			{name: "email", dataType: "VARCHAR(20)", notNull: true, unique: true},
			{name: "bio", dataType: "TEXT"},
			{name: "created_at", dataType: "TIMESTAMPTZ", notNull: true},
		}, schema.columns)
		assert.Equal(t, []foreignKeySchema{
			{column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
			{column: "category_id", refTable: "categories", refColumn: "id", onDelete: "SET NULL"},
		}, schema.foreignKeys)
	})

	t.Run("requires nullable column for set null delete rule", func(t *testing.T) {
		// ===== Arrange =====
		module := makeTestMigrationModule(t, "shop", "post.go", `package entity

import "github.com/google/uuid"

type Post struct {
	ID     uuid.UUID
	UserID uuid.UUID // del:setnull
}
`)

		// ===== Act =====
		_, err := parseEntitySchema(module, "post")

		// ===== Assert =====
		require.Error(t, err)
		assert.Contains(t, err.Error(), "del:setnull requires null")
	})
}

func Test_Migration_BuildCreateTableSQL(t *testing.T) {
	t.Run("builds create table sql", func(t *testing.T) {
		// ===== Arrange =====
		schema := tableSchema{
			name: "posts",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
				{name: "user_id", dataType: "UUID", notNull: true},
				{name: "title", dataType: "VARCHAR(100)", notNull: true, unique: true},
			},
			foreignKeys: []foreignKeySchema{
				{column: "user_id", refTable: "users", refColumn: "id", onDelete: "CASCADE"},
			},
		}

		// ===== Act =====
		sql := buildCreateTableSQL(schema)

		// ===== Assert =====
		assert.Equal(t, `CREATE TABLE posts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    title VARCHAR(100) NOT NULL UNIQUE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
`, sql)
	})
}

func Test_Migration_BuildDropTableSQL(t *testing.T) {
	t.Run("builds drop table sql", func(t *testing.T) {
		// ===== Arrange =====
		schema := tableSchema{name: "posts"}

		// ===== Act =====
		sql := buildDropTableSQL(schema)

		// ===== Assert =====
		assert.Equal(t, "DROP TABLE IF EXISTS posts;\n", sql)
	})
}

func makeTestMigrationModule(t *testing.T, moduleName string, entityFile string, entitySource string) migrationModule {
	t.Helper()

	root := t.TempDir()
	entityDir := filepath.Join(root, "module", moduleName, "internal", "domain", "entity")
	migrationDir := filepath.Join(root, "module", moduleName, "migration")
	require.NoError(t, os.MkdirAll(entityDir, 0755))
	require.NoError(t, os.MkdirAll(migrationDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(entityDir, entityFile), []byte(entitySource), 0644))

	return migrationModule{
		name:         moduleName,
		entityDir:    entityDir,
		migrationDir: migrationDir,
	}
}
