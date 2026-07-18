package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wiszel/wigrate/internal/schema"
)

func TestReplayColumn(t *testing.T) {
	t.Run("Parse", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			want schema.Column
			ok   bool
		}{
			{"id UUID", schema.Column{Name: "id", DataType: "UUID"}, true},
			{"id UUID PRIMARY KEY", schema.Column{Name: "id", DataType: "UUID", Primary: true}, true},
			{"email TEXT NOT NULL", schema.Column{Name: "email", DataType: "TEXT", NotNull: true}, true},
			{"email VARCHAR(20) NOT NULL UNIQUE", schema.Column{Name: "email", DataType: "VARCHAR(20)", NotNull: true, Unique: true}, true},
			{"score DOUBLE PRECISION", schema.Column{Name: "score", DataType: "DOUBLE PRECISION"}, true},
			{"FOREIGN KEY (u) REFERENCES r(id)", schema.Column{}, false},
			{"id", schema.Column{}, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedColumn(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})
}

func TestReplayFK(t *testing.T) {
	t.Run("parseFK", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			want schema.ForeignKey
			ok   bool
		}{
			{"FOREIGN KEY (u) REFERENCES r(id)", schema.ForeignKey{Column: "u", RefTable: "r", RefColumn: "id"}, true},
			{"FOREIGN KEY (u) REFERENCES r(id) ON DELETE CASCADE", schema.ForeignKey{Column: "u", RefTable: "r", RefColumn: "id", OnDelete: "CASCADE"}, true},
			{"FOREIGN KEY (u)", schema.ForeignKey{}, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedForeignKey(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})

	t.Run("parseAlterFK", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			want schema.ForeignKey
			ok   bool
		}{
			{"ADD CONSTRAINT x FOREIGN KEY (r) REFERENCES roles(id)", schema.ForeignKey{Column: "r", RefTable: "roles", RefColumn: "id"}, true},
			{"ADD CONSTRAINT x UNIQUE (e)", schema.ForeignKey{}, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedAlterForeignKey(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		}
	})
}

func TestReplayUniqueConstraint(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			cols []string
			ok   bool
		}{
			{"ADD CONSTRAINT uq UNIQUE (email)", []string{"email"}, true},
			{"ADD CONSTRAINT uq UNIQUE (a, b)", []string{"a", "b"}, true},
			{"ADD CONSTRAINT uq UNIQUE ()", nil, false},
			{"bad", nil, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedUniqueConstraint(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.cols, got)
			}
		}
	})
}

func TestReplayIndex(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in   string
			cols []string
			ok   bool
		}{
			{"CREATE INDEX idx_users_email ON users (email)", []string{"email"}, true},
			{"CREATE INDEX idx_events_tenant_id_happened ON events (tenant_id, happened)", []string{"tenant_id", "happened"}, true},
			{"CREATE INDEX idx_users_email ON users ()", nil, false},
			{"bad", nil, false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedIndex(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.cols, got)
			}
		}
	})

	t.Run("round-trips CREATE INDEX and DROP INDEX IF EXISTS through applyGeneratedSQL", func(t *testing.T) {
		// ===== Arrange ===== //
		table := &schema.Table{Name: "users"}

		// ===== Act ===== //
		applyGeneratedSQL(table, "CREATE INDEX idx_users_email ON users (email);\n")

		// ===== Assert ===== //
		assert.Equal(t, [][]string{{"email"}}, table.Indexes)

		// ===== Act ===== //
		applyGeneratedSQL(table, "DROP INDEX IF EXISTS idx_users_email;\n")

		// ===== Assert ===== //
		assert.Empty(t, table.Indexes)
	})
}

func TestReplayTrgmIndex(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		// ===== Arrange ===== //
		tests := []struct {
			in  string
			col string
			ok  bool
		}{
			{"CREATE INDEX idx_notes_body_trgm ON notes USING GIN (body gin_trgm_ops)", "body", true},
			{"bad", "", false},
		}

		for _, tt := range tests {
			// ===== Act ===== //
			got, ok := parseGeneratedTrgmIndex(tt.in)

			// ===== Assert ===== //
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.col, got)
			}
		}
	})

	t.Run("round-trips CREATE INDEX ... USING GIN and DROP INDEX IF EXISTS through applyGeneratedSQL", func(t *testing.T) {
		// ===== Arrange ===== //
		table := &schema.Table{Name: "notes"}

		// ===== Act ===== //
		applyGeneratedSQL(table, "CREATE EXTENSION IF NOT EXISTS pg_trgm;\nCREATE INDEX idx_notes_body_trgm ON notes USING GIN (body gin_trgm_ops);\n")

		// ===== Assert ===== //
		assert.Equal(t, []string{"body"}, table.TrgmIndexes)
		assert.Empty(t, table.Columns, "CREATE EXTENSION line must not be parsed as a column")

		// ===== Act ===== //
		applyGeneratedSQL(table, "DROP INDEX IF EXISTS idx_notes_body_trgm;\n")

		// ===== Assert ===== //
		assert.Empty(t, table.TrgmIndexes)
	})

	t.Run("plain index and trgm index on the same column stay independent", func(t *testing.T) {
		// ===== Arrange ===== //
		table := &schema.Table{Name: "notes"}

		// ===== Act ===== //
		applyGeneratedSQL(table, "CREATE INDEX idx_notes_body ON notes (body);\nCREATE INDEX idx_notes_body_trgm ON notes USING GIN (body gin_trgm_ops);\n")

		// ===== Assert ===== //
		assert.Equal(t, [][]string{{"body"}}, table.Indexes)
		assert.Equal(t, []string{"body"}, table.TrgmIndexes)

		// ===== Act ===== //
		applyGeneratedSQL(table, "DROP INDEX IF EXISTS idx_notes_body_trgm;\n")

		// ===== Assert ===== //
		assert.Equal(t, [][]string{{"body"}}, table.Indexes, "dropping the trgm index must not remove the plain index")
		assert.Empty(t, table.TrgmIndexes)
	})
}
