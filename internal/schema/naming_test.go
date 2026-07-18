package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
