package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiszel/wigrate/internal/discover"
)

func Test_Migration_ParseEntitySchema(t *testing.T) {
	t.Run("maps struct fields and inline comments to table schema", func(t *testing.T) {
		// ===== Arrange ===== //
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

		// ===== Act ===== //
		schema, err := Parse(module, "user_profile")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "user_profiles", schema.Name)
		assert.Equal(t, []Column{
			{Name: "id", DataType: "UUID", Primary: true},
			{Name: "user_id", DataType: "UUID", NotNull: true},
			{Name: "category_id", DataType: "UUID"},
			{Name: "email", DataType: "VARCHAR(20)", NotNull: true, Unique: true},
			{Name: "bio", DataType: "TEXT"},
			{Name: "created_at", DataType: "TIMESTAMPTZ", NotNull: true},
		}, schema.Columns)
		assert.Equal(t, []ForeignKey{
			{Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"},
			{Column: "category_id", RefTable: "categories", RefColumn: "id", OnDelete: "SET NULL"},
		}, schema.ForeignKeys)
	})

	t.Run("requires nullable column for set null delete rule", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "post.go", `package entity

import "github.com/google/uuid"

type Post struct {
	ID     uuid.UUID
	UserID uuid.UUID // del:setnull
}
`)

		// ===== Act ===== //
		_, err := Parse(module, "post")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "del:setnull requires null")
	})

	t.Run("honors pk annotation for non-ID field", func(t *testing.T) {
		// ===== Arrange ===== //
		// ID is always primary; an explicit `pk` on another field now folds
		// both into a composite primary key rather than two inline PK columns.
		module := makeTestMigrationModule(t, "shop", "custom.go", `package entity

import "github.com/google/uuid"

type Custom struct {
	ID   uuid.UUID
	Code string // 20 pk
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "custom")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, schema.Columns, 2)
		expectedID := Column{Name: "id", DataType: "UUID", NotNull: true}
		assert.Equal(t, expectedID.Name, schema.Columns[0].Name)
		assert.Equal(t, expectedID.DataType, schema.Columns[0].DataType)
		assert.Equal(t, expectedID.NotNull, schema.Columns[0].NotNull)
		expectedCode := Column{Name: "code", DataType: "VARCHAR(20)", NotNull: true}
		assert.Equal(t, expectedCode.Name, schema.Columns[1].Name)
		assert.Equal(t, expectedCode.DataType, schema.Columns[1].DataType)
		assert.Equal(t, expectedCode.NotNull, schema.Columns[1].NotNull)
		assert.Equal(t, []string{"id", "code"}, schema.PrimaryKey)
	})

	t.Run("folds unique:<group> into a composite unique constraint", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "membership.go", `package entity

import "github.com/google/uuid"

type Membership struct {
	ID     uuid.UUID
	TeamID uuid.UUID // unique:member
	UserID uuid.UUID // unique:member
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "membership")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.False(t, schema.Columns[1].Unique)
		assert.False(t, schema.Columns[2].Unique)
		assert.Equal(t, [][]string{{"team_id", "user_id"}}, schema.Uniques)
	})

	t.Run("single-member unique group degrades to inline unique", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "solo.go", `package entity

import "github.com/google/uuid"

type Solo struct {
	ID   uuid.UUID
	Code string // unique:only
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "solo")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, schema.Columns[1].Unique)
		assert.Empty(t, schema.Uniques)
	})

	t.Run("parses bare index into a single-column index", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "article.go", `package entity

import "github.com/google/uuid"

type Article struct {
	ID    uuid.UUID
	Title string // 100 index
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "article")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"title"}}, schema.Indexes)
	})

	t.Run("folds index:<group> into a composite index", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "event.go", `package entity

import (
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID       uuid.UUID
	TenantID uuid.UUID // index:lookup
	Happened time.Time // index:lookup
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "event")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"tenant_id", "happened"}}, schema.Indexes)
	})

	t.Run("rejects empty index group", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "bad.go", `package entity

import "github.com/google/uuid"

type Bad struct {
	ID   uuid.UUID
	Code string // index:
}
`)

		// ===== Act ===== //
		_, err := Parse(module, "bad")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty index group")
	})

	t.Run("parses trgm into a single-column trigram index", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "note.go", `package entity

import "github.com/google/uuid"

type Note struct {
	ID   uuid.UUID
	Body string // 200 unique trgm
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "note")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []string{"body"}, schema.TrgmIndexes)
		assert.True(t, schema.Columns[1].Unique, "trgm combines with unique on the same field")
	})

	t.Run("rejects trgm on a non-string field", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "count.go", `package entity

import "github.com/google/uuid"

type Count struct {
	ID    uuid.UUID
	Total int // trgm
}
`)

		// ===== Act ===== //
		_, err := Parse(module, "count")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trgm requires a string field")
	})

	t.Run("makes pointer field nullable by default", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "item.go", `package entity

import "github.com/google/uuid"

type Item struct {
	ID    uuid.UUID
	Name  *string
	Price *int
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "item")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, schema.Columns, 3)
		assert.False(t, schema.Columns[1].NotNull, "pointer string should be nullable")
		assert.False(t, schema.Columns[2].NotNull, "pointer int should be nullable")
	})

	t.Run("explicit null on pointer field is still nullable", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "product.go", `package entity

import "github.com/google/uuid"

type Product struct {
	ID   uuid.UUID
	Desc *string // 50 null
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "product")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, schema.Columns, 2)
		assert.False(t, schema.Columns[1].NotNull)
		assert.Equal(t, "VARCHAR(50)", schema.Columns[1].DataType)
	})
}

func makeTestMigrationModule(t *testing.T, moduleName string, entityFile string, entitySource string) discover.Module {
	t.Helper()

	root := t.TempDir()
	entityDir := filepath.Join(root, "module", moduleName, "internal", "domain", "entity")
	migrationDir := filepath.Join(root, "module", moduleName, "migration")
	assert.NoError(t, os.MkdirAll(entityDir, 0755))
	assert.NoError(t, os.MkdirAll(migrationDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(entityDir, entityFile), []byte(entitySource), 0644))

	return discover.Module{
		Name:         moduleName,
		EntityDir:    entityDir,
		MigrationDir: migrationDir,
	}
}

// TestSchemaParserRobustness covers the hardening fixes (B5-B8).
func TestSchemaParserRobustness(t *testing.T) {
	t.Run("B5: acronym struct name (APIKey) found by snake_case match", func(t *testing.T) {
		// pascalCase("api_key") = "ApiKey" ≠ "APIKey"; findStruct must match by snakeCase.
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "api_key.go", `package entity

import "github.com/google/uuid"

type APIKey struct {
	ID    uuid.UUID
	Token string
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "api_key")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "api_keys", schema.Name)
		assert.Len(t, schema.Columns, 2)
	})

	t.Run("B6: UUID-suffixed field does not produce a foreign key", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID        uuid.UUID
	OwnerUUID uuid.UUID
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Empty(t, schema.ForeignKeys)
	})

	t.Run("B7: ref: annotation on non-ID field returns error", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

type User struct {
	ID    int
	Owner string // ref:users
}
`)

		// ===== Act ===== //
		_, err := Parse(module, "user")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ref:/del:")
	})

	t.Run("B8: embedded struct returns error", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type Base struct{ ID uuid.UUID }

type User struct {
	Base
	Name string
}
`)

		// ===== Act ===== //
		_, err := Parse(module, "user")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "embedded")
	})
}
