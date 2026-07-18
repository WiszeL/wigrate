package sqlgen

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
