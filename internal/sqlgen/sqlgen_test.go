package sqlgen

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wiszel/wigrate/internal/diff"
	"github.com/wiszel/wigrate/internal/schema"
)

func Test_Sqlgen_CreateTableSQL(t *testing.T) {
	t.Run("builds create table sql", func(t *testing.T) {
		// ===== Arrange ===== //
		table := schema.Table{
			Name: "posts",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "user_id", DataType: "UUID", NotNull: true},
				{Name: "title", DataType: "VARCHAR(100)", NotNull: true, Unique: true},
			},
			ForeignKeys: []schema.ForeignKey{
				{Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"},
			},
		}

		// ===== Act ===== //
		sql := CreateTableSQL(table)

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

func Test_Sqlgen_CreateTableSQL_Enum(t *testing.T) {
	t.Run("adds a CHECK constraint for an enum column", func(t *testing.T) {
		// ===== Arrange ===== //
		table := schema.Table{
			Name: "payments",
			Columns: []schema.Column{
				{Name: "id", DataType: "UUID", Primary: true},
				{Name: "status", DataType: "VARCHAR(7)", NotNull: true, Check: "'failed','paid','pending'"},
			},
		}

		// ===== Act ===== //
		sql := CreateTableSQL(table)

		// ===== Assert ===== //
		assert.Equal(t, `CREATE TABLE payments (
    id UUID PRIMARY KEY,
    status VARCHAR(7) NOT NULL,
    CONSTRAINT chk_payments_status CHECK (status IN ('failed','paid','pending'))
);
`, sql)
	})
}

func Test_Sqlgen_AlterTableSQL_Enum(t *testing.T) {
	t.Run("adds a CHECK constraint for a newly added enum column", func(t *testing.T) {
		// ===== Arrange ===== //
		d := diff.Result{
			TableName:    "payments",
			AddedColumns: []schema.Column{{Name: "status", DataType: "VARCHAR(7)", NotNull: true, Check: "'failed','paid','pending'"}},
		}

		// ===== Act ===== //
		sql := AlterTableSQL(d)

		// ===== Assert ===== //
		assert.Equal(t, `ALTER TABLE payments
    ADD COLUMN status VARCHAR(7) NOT NULL,
    ADD CONSTRAINT chk_payments_status CHECK (status IN ('failed','paid','pending'));
`, sql)
	})

	t.Run("drops and re-adds the CHECK constraint when enum values change", func(t *testing.T) {
		// ===== Arrange ===== //
		d := diff.Result{
			TableName: "payments",
			ChangedColumns: []diff.ColumnChange{{
				Before: schema.Column{Name: "status", DataType: "VARCHAR(7)", NotNull: true, Check: "'paid','pending'"},
				After:  schema.Column{Name: "status", DataType: "VARCHAR(9)", NotNull: true, Check: "'paid','pending','refunded'"},
			}},
		}

		// ===== Act ===== //
		sql := AlterTableSQL(d)

		// ===== Assert ===== //
		assert.Equal(t, `ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS chk_payments_status,
    ALTER COLUMN status TYPE VARCHAR(9),
    ADD CONSTRAINT chk_payments_status CHECK (status IN ('paid','pending','refunded'));
`, sql)
	})

	t.Run("reverts an enum value change back to the previous CHECK", func(t *testing.T) {
		// ===== Arrange ===== //
		d := diff.Result{
			TableName: "payments",
			ChangedColumns: []diff.ColumnChange{{
				Before: schema.Column{Name: "status", DataType: "VARCHAR(7)", NotNull: true, Check: "'paid','pending'"},
				After:  schema.Column{Name: "status", DataType: "VARCHAR(9)", NotNull: true, Check: "'paid','pending','refunded'"},
			}},
		}

		// ===== Act ===== //
		sql := RevertAlterTableSQL(d)

		// ===== Assert ===== //
		assert.Equal(t, `ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS chk_payments_status,
    ALTER COLUMN status TYPE VARCHAR(7),
    ADD CONSTRAINT chk_payments_status CHECK (status IN ('paid','pending'));
`, sql)
	})
}

func Test_Sqlgen_DropTableSQL(t *testing.T) {
	t.Run("builds drop table sql", func(t *testing.T) {
		// ===== Arrange ===== //
		table := schema.Table{Name: "posts"}

		// ===== Act ===== //
		sql := DropTableSQL(table)

		// ===== Assert ===== //
		assert.Equal(t, "DROP TABLE IF EXISTS posts;\n", sql)
	})
}
