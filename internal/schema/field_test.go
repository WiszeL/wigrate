package schema

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, fieldComment{nullable: true}, comment)
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
		assert.Equal(t, fieldComment{unique: true}, comment)
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
		assert.Equal(t, fieldComment{index: true}, comment)
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
		assert.Equal(t, fieldComment{indexGroup: "lookup"}, comment)
	})

	t.Run("parses trgm token", func(t *testing.T) {
		// ===== Act ===== //
		comment, err := parseFieldComment(&ast.Field{
			Comment: &ast.CommentGroup{
				List: []*ast.Comment{{Text: "// trgm"}},
			},
		})
		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, fieldComment{trgm: true}, comment)
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
		assert.Equal(t, fieldComment{length: 50}, comment)
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
		assert.Equal(t, fieldComment{refTable: "roles"}, comment)
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
		assert.Equal(t, fieldComment{deleteRule: "CASCADE"}, comment)
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
