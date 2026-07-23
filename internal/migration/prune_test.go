package migration

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wiszel/wigrate/internal/schema"
)

// captureStderr redirects os.Stderr for the duration of fn and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stderr
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	os.Stderr = w

	fn()

	assert.NoError(t, w.Close())
	os.Stderr = original

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	assert.NoError(t, err)
	return buf.String()
}

func Test_PruneForeignKeys(t *testing.T) {
	t.Run("keeps a foreign key whose ref table exists in the module", func(t *testing.T) {
		// ===== Arrange ===== //
		fks := []schema.ForeignKey{{Column: "role_id", RefTable: "roles", RefColumn: "id"}}
		moduleTables := map[string]struct{}{"roles": {}, "users": {}}

		// ===== Act ===== //
		kept := pruneForeignKeys(fks, moduleTables, "iam")

		// ===== Assert ===== //
		assert.Equal(t, fks, kept)
	})

	t.Run("drops a foreign key whose ref table is missing from the module, and warns", func(t *testing.T) {
		// ===== Arrange ===== //
		fks := []schema.ForeignKey{{Column: "user_id", RefTable: "users", RefColumn: "id"}}
		moduleTables := map[string]struct{}{"roles": {}}

		// ===== Act ===== //
		var kept []schema.ForeignKey
		stderr := captureStderr(t, func() {
			kept = pruneForeignKeys(fks, moduleTables, "billing")
		})

		// ===== Assert ===== //
		assert.Empty(t, kept)
		assert.Contains(t, stderr, `"user_id"`)
		assert.Contains(t, stderr, `"users"`)
		assert.Contains(t, stderr, "billing")
		assert.Contains(t, stderr, "Is this intended?")
	})
}
