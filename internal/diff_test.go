package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Migration_DiffSchema(t *testing.T) {
	t.Run("rejects primary key changes", func(t *testing.T) {
		// ===== Arrange =====
		previous := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", primary: true},
			},
		}
		desired := tableSchema{
			name: "users",
			columns: []columnSchema{
				{name: "id", dataType: "UUID", notNull: true},
			},
		}

		// ===== Act =====
		_, err := diffSchema(previous, desired)

		// ===== Assert =====
		require.Error(t, err)
		assert.Contains(t, err.Error(), "primary key change")
	})
}
