package internal

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
		schema, err := parseEntitySchema(module, "user_profile")

		// ===== Assert ===== //
		assert.NoError(t, err)
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
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "post.go", `package entity

import "github.com/google/uuid"

type Post struct {
	ID     uuid.UUID
	UserID uuid.UUID // del:setnull
}
`)

		// ===== Act ===== //
		_, err := parseEntitySchema(module, "post")

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
		schema, err := parseEntitySchema(module, "custom")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, schema.columns, 2)
		assert.False(t, schema.columns[0].primary)
		assert.False(t, schema.columns[1].primary)
		assert.Equal(t, []string{"id", "code"}, schema.primaryKey)
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
		schema, err := parseEntitySchema(module, "membership")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.False(t, schema.columns[1].unique)
		assert.False(t, schema.columns[2].unique)
		assert.Equal(t, [][]string{{"team_id", "user_id"}}, schema.uniques)
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
		schema, err := parseEntitySchema(module, "solo")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, schema.columns[1].unique)
		assert.Empty(t, schema.uniques)
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
		schema, err := parseEntitySchema(module, "article")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"title"}}, schema.indexes)
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
		schema, err := parseEntitySchema(module, "event")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"tenant_id", "happened"}}, schema.indexes)
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
		_, err := parseEntitySchema(module, "bad")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty index group")
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
		schema, err := parseEntitySchema(module, "item")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, schema.columns, 3)
		assert.False(t, schema.columns[1].notNull, "pointer string should be nullable")
		assert.False(t, schema.columns[2].notNull, "pointer int should be nullable")
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
		schema, err := parseEntitySchema(module, "product")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, schema.columns, 2)
		assert.False(t, schema.columns[1].notNull)
		assert.Equal(t, "VARCHAR(50)", schema.columns[1].dataType)
	})
}

func Test_Migration_BuildCreateTableSQL(t *testing.T) {
	t.Run("builds create table sql", func(t *testing.T) {
		// ===== Arrange ===== //
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

		// ===== Act ===== //
		sql := buildCreateTableSQL(schema)

		// ===== Assert ===== //
		assert.Equal(t, `CREATE TABLE posts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    title VARCHAR(100) NOT NULL UNIQUE,
    CONSTRAINT fk_posts_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
`, sql)
	})
}

func Test_Migration_BuildDropTableSQL(t *testing.T) {
	t.Run("builds drop table sql", func(t *testing.T) {
		// ===== Arrange ===== //
		schema := tableSchema{name: "posts"}

		// ===== Act ===== //
		sql := buildDropTableSQL(schema)

		// ===== Assert ===== //
		assert.Equal(t, "DROP TABLE IF EXISTS posts;\n", sql)
	})
}

func Test_Schema_SnakeCase(t *testing.T) {
	t.Run("converts camel case", func(t *testing.T) {
		// ===== Act ===== //
		result := snakeCase("UserID")
		// ===== Assert ===== //
		assert.Equal(t, "user_id", result)
	})

	t.Run("converts ID to id", func(t *testing.T) {
		// ===== Act ===== //
		result := snakeCase("ID")
		// ===== Assert ===== //
		assert.Equal(t, "id", result)
	})

	t.Run("handles HTML in acronym", func(t *testing.T) {
		// ===== Act ===== //
		result := snakeCase("HTMLParser")
		// ===== Assert ===== //
		assert.Equal(t, "html_parser", result)
	})

	t.Run("preserves existing underscores", func(t *testing.T) {
		// ===== Act ===== //
		result := snakeCase("already_snake")
		// ===== Assert ===== //
		assert.Equal(t, "already_snake", result)
	})

	t.Run("handles single character", func(t *testing.T) {
		// ===== Act ===== //
		result := snakeCase("A")
		// ===== Assert ===== //
		assert.Equal(t, "a", result)
	})

	t.Run("handles empty string", func(t *testing.T) {
		// ===== Act ===== //
		result := snakeCase("")
		// ===== Assert ===== //
		assert.Equal(t, "", result)
	})
}

func Test_Schema_PascalCase(t *testing.T) {
	t.Run("converts snake case", func(t *testing.T) {
		// ===== Act ===== //
		result := pascalCase("user")
		// ===== Assert ===== //
		assert.Equal(t, "User", result)
	})

	t.Run("converts multi-word snake case", func(t *testing.T) {
		// ===== Act ===== //
		result := pascalCase("user_profile")
		// ===== Assert ===== //
		assert.Equal(t, "UserProfile", result)
	})

	t.Run("handles empty string", func(t *testing.T) {
		// ===== Act ===== //
		result := pascalCase("")
		// ===== Assert ===== //
		assert.Equal(t, "", result)
	})

	t.Run("handles single word", func(t *testing.T) {
		// ===== Act ===== //
		result := pascalCase("user")
		// ===== Assert ===== //
		assert.Equal(t, "User", result)
	})
}

func Test_Schema_PluralizeSnakeCase(t *testing.T) {
	t.Run("appends s for regular words", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("user")
		// ===== Assert ===== //
		assert.Equal(t, "users", result)
	})

	t.Run("changes y to ies after consonant", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("category")
		// ===== Assert ===== //
		assert.Equal(t, "categories", result)
	})

	t.Run("keeps y after vowel", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("toy")
		// ===== Assert ===== //
		assert.Equal(t, "toys", result)
	})

	t.Run("appends es for s ending", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("status")
		// ===== Assert ===== //
		assert.Equal(t, "statuses", result)
	})

	t.Run("appends es for x ending", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("box")
		// ===== Assert ===== //
		assert.Equal(t, "boxes", result)
	})

	t.Run("appends es for z ending", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("quiz")
		// ===== Assert ===== //
		assert.Equal(t, "quizes", result)
	})

	t.Run("appends es for ch ending", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("match")
		// ===== Assert ===== //
		assert.Equal(t, "matches", result)
	})

	t.Run("appends es for sh ending", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("dish")
		// ===== Assert ===== //
		assert.Equal(t, "dishes", result)
	})

	t.Run("handles empty string", func(t *testing.T) {
		// ===== Act ===== //
		result := pluralizeSnakeCase("")
		// ===== Assert ===== //
		assert.Equal(t, "", result)
	})
}

func Test_Schema_IdentToSQLType(t *testing.T) {
	t.Run("maps string to TEXT", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("string", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "TEXT", typ)
	})

	t.Run("maps string with length to VARCHAR", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("string", fieldComment{length: 50})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "VARCHAR(50)", typ)
	})

	t.Run("maps int to INTEGER", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("int", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "INTEGER", typ)
	})

	t.Run("maps int32 to INTEGER", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("int32", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "INTEGER", typ)
	})

	t.Run("maps int64 to BIGINT", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("int64", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "BIGINT", typ)
	})

	t.Run("maps bool to BOOLEAN", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("bool", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "BOOLEAN", typ)
	})

	t.Run("maps float32 to REAL", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("float32", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "REAL", typ)
	})

	t.Run("maps float64 to DOUBLE PRECISION", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("float64", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "DOUBLE PRECISION", typ)
	})

	t.Run("maps time.Time to TIMESTAMPTZ", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("time.Time", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "TIMESTAMPTZ", typ)
	})

	t.Run("maps uuid.UUID to UUID", func(t *testing.T) {
		// ===== Act ===== //
		typ, err := identToSQLType("uuid.UUID", fieldComment{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "UUID", typ)
	})

	t.Run("returns error for unsupported type", func(t *testing.T) {
		// ===== Act ===== //
		_, err := identToSQLType("unsupported.Type", fieldComment{})
		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported field type")
	})
}

func Test_Schema_ParseFieldComment(t *testing.T) {
	t.Run("returns empty comment for no comment", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, fieldComment{}, comment)
	})

	t.Run("parses null token", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// null"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, comment.nullable)
	})

	t.Run("parses unique token", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// unique"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, comment.unique)
	})

	t.Run("parses index token", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// index"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, comment.index)
	})

	t.Run("parses index group", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// index:lookup"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "lookup", comment.indexGroup)
	})

	t.Run("parses length token", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// 50"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, 50, comment.length)
	})

	t.Run("parses ref table", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// ref:roles"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "roles", comment.refTable)
	})

	t.Run("parses delete rule", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// del:cascade"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "CASCADE", comment.deleteRule)
	})

	t.Run("parses multiple tokens", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// 20 null unique ref:roles del:cascade"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, 20, comment.length)
		assert.True(t, comment.nullable)
		assert.True(t, comment.unique)
		assert.Equal(t, "roles", comment.refTable)
		assert.Equal(t, "CASCADE", comment.deleteRule)
	})

	t.Run("returns error for empty ref table", func(t *testing.T) {
		// ===== Act ===== //
		_, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// ref:"}},
			},
		})
		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty ref table")
	})

	t.Run("returns error for invalid delete rule", func(t *testing.T) {
		// ===== Act ===== //
		_, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// del:invalid"}},
			},
		})
		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported delete rule")
	})

	t.Run("returns error for invalid token", func(t *testing.T) {
		// ===== Act ===== //
		_, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// unknown_token"}},
			},
		})
		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown comment token")
	})

	t.Run("returns error for zero length", func(t *testing.T) {
		// ===== Act ===== //
		_, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// 0"}},
			},
		})
		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "varchar length must be greater than zero")
	})

	t.Run("returns error for negative length", func(t *testing.T) {
		// ===== Act ===== //
		_, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// -5"}},
			},
		})
		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "varchar length must be greater than zero")
	})
}

func Test_Schema_NormalizeDeleteRule(t *testing.T) {
	t.Run("normalizes cascade", func(t *testing.T) {
		// ===== Act ===== //
		result, err := normalizeDeleteRule("cascade")
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "CASCADE", result)
	})

	t.Run("normalizes setnull", func(t *testing.T) {
		// ===== Act ===== //
		result, err := normalizeDeleteRule("setnull")
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "SET NULL", result)
	})

	t.Run("normalizes restrict", func(t *testing.T) {
		// ===== Act ===== //
		result, err := normalizeDeleteRule("restrict")
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "RESTRICT", result)
	})

	t.Run("normalizes noaction", func(t *testing.T) {
		// ===== Act ===== //
		result, err := normalizeDeleteRule("noaction")
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "NO ACTION", result)
	})

	t.Run("returns error for unsupported rule", func(t *testing.T) {
		// ===== Act ===== //
		_, err := normalizeDeleteRule("invalid")
		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported delete rule")
	})
}

func makeTestMigrationModule(t *testing.T, moduleName string, entityFile string, entitySource string) migrationModule {
	t.Helper()

	root := t.TempDir()
	entityDir := filepath.Join(root, "module", moduleName, "internal", "domain", "entity")
	migrationDir := filepath.Join(root, "module", moduleName, "migration")
	assert.NoError(t, os.MkdirAll(entityDir, 0755))
	assert.NoError(t, os.MkdirAll(migrationDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(entityDir, entityFile), []byte(entitySource), 0644))

	return migrationModule{
		name:         moduleName,
		entityDir:    entityDir,
		migrationDir: migrationDir,
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
		schema, err := parseEntitySchema(module, "api_key")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "api_keys", schema.name)
		assert.Len(t, schema.columns, 2)
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
		schema, err := parseEntitySchema(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Empty(t, schema.foreignKeys)
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
		_, err := parseEntitySchema(module, "user")

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
		_, err := parseEntitySchema(module, "user")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "embedded")
	})
}
