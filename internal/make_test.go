package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsGoEntityFile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "entity file", input: "user.go", expected: true},
		{name: "entity file nested", input: "role.go", expected: true},
		{name: "test file", input: "user_test.go", expected: false},
		{name: "non-go file", input: "README.md", expected: false},
		{name: "go test with extra dots", input: "user_service_test.go", expected: false},
		{name: "go file with dots", input: "user.service.go", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGoEntityFile(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

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
