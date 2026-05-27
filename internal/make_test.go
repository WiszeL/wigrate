package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Migration_FilterModules(t *testing.T) {
	t.Run("returns all modules when no module is selected", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []migrationModule{
			{name: "iam"},
			{name: "billing"},
		}

		// ===== Act ===== //
		filtered, err := filterModules(modules, "")

		// ===== Assert ===== //
		assert.NoError(t, err)

		assert.Equal(t, modules, filtered)
	})

	t.Run("returns selected module", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []migrationModule{
			{name: "iam"},
			{name: "billing"},
		}

		// ===== Act ===== //
		filtered, err := filterModules(modules, "iam")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []migrationModule{{name: "iam"}}, filtered)
	})

	t.Run("returns error when module is missing", func(t *testing.T) {
		// ===== Arrange ===== //
		modules := []migrationModule{
			{name: "iam"},
			{name: "billing"},
		}

		// ===== Act ===== //
		_, err := filterModules(modules, "catalog")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "module not found: catalog")
	})
}
