package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiszel/wigrate/internal/discover"
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
		schema, err := Parse(module, "user_profile")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "user_profiles", schema.Name)
		assert.Equal(t, []Column{
			{Name: "id", DataType: "UUID", Primary: true},
			{Name: "user_id", DataType: "UUID", NotNull: true},
			{Name: "category_id", DataType: "UUID"},
			{Name: "email", DataType: "VARCHAR(20)", NotNull: true, Unique: true},
			{Name: "bio", DataType: "TEXT"},
			{Name: "created_at", DataType: "TIMESTAMPTZ", NotNull: true},
		}, schema.Columns)
		assert.Equal(t, []ForeignKey{
			{Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"},
			{Column: "category_id", RefTable: "categories", RefColumn: "id", OnDelete: "SET NULL"},
		}, schema.ForeignKeys)
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
		_, err := Parse(module, "post")

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
		schema, err := Parse(module, "custom")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, schema.Columns, 2)
		expectedID := Column{Name: "id", DataType: "UUID", NotNull: true}
		assert.Equal(t, expectedID.Name, schema.Columns[0].Name)
		assert.Equal(t, expectedID.DataType, schema.Columns[0].DataType)
		assert.Equal(t, expectedID.NotNull, schema.Columns[0].NotNull)
		expectedCode := Column{Name: "code", DataType: "VARCHAR(20)", NotNull: true}
		assert.Equal(t, expectedCode.Name, schema.Columns[1].Name)
		assert.Equal(t, expectedCode.DataType, schema.Columns[1].DataType)
		assert.Equal(t, expectedCode.NotNull, schema.Columns[1].NotNull)
		assert.Equal(t, []string{"id", "code"}, schema.PrimaryKey)
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
		schema, err := Parse(module, "membership")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.False(t, schema.Columns[1].Unique)
		assert.False(t, schema.Columns[2].Unique)
		assert.Equal(t, [][]string{{"team_id", "user_id"}}, schema.Uniques)
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
		schema, err := Parse(module, "solo")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, schema.Columns[1].Unique)
		assert.Empty(t, schema.Uniques)
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
		schema, err := Parse(module, "article")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"title"}}, schema.Indexes)
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
		schema, err := Parse(module, "event")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{{"tenant_id", "happened"}}, schema.Indexes)
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
		_, err := Parse(module, "bad")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty index group")
	})

	t.Run("parses trgm into a single-column trigram index", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "note.go", `package entity

import "github.com/google/uuid"

type Note struct {
	ID   uuid.UUID
	Body string // 200 unique trgm
}
`)

		// ===== Act ===== //
		schema, err := Parse(module, "note")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []string{"body"}, schema.TrgmIndexes)
		assert.True(t, schema.Columns[1].Unique, "trgm combines with unique on the same field")
	})

	t.Run("rejects trgm on a non-string field", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "count.go", `package entity

import "github.com/google/uuid"

type Count struct {
	ID    uuid.UUID
	Total int // trgm
}
`)

		// ===== Act ===== //
		_, err := Parse(module, "count")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trgm requires a string field")
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
		schema, err := Parse(module, "item")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, schema.Columns, 3)
		assert.False(t, schema.Columns[1].NotNull, "pointer string should be nullable")
		assert.False(t, schema.Columns[2].NotNull, "pointer int should be nullable")
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
		schema, err := Parse(module, "product")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Len(t, schema.Columns, 2)
		assert.False(t, schema.Columns[1].NotNull)
		assert.Equal(t, "VARCHAR(50)", schema.Columns[1].DataType)
	})

	t.Run("recognizes local named string type with consts as an enum field", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID     uuid.UUID
	Status PaymentStatus
}
`,
			"payment_status.go": `package entity

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentPaid    PaymentStatus = "paid"
	PaymentFailed  PaymentStatus = "failed"
)
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, schema.Columns, 2)
		status := schema.Columns[1]
		assert.Equal(t, "status", status.Name)
		assert.Equal(t, "VARCHAR(7)", status.DataType, "sized to longest label 'pending'")
		assert.Equal(t, "'failed','paid','pending'", status.Check, "canonical: sorted lexically regardless of const order")
		assert.True(t, status.NotNull)
	})

	t.Run("recognizes local named int type with iota consts as an enum field", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"invoice.go": `package entity

import "github.com/google/uuid"

type Invoice struct {
	ID       uuid.UUID
	Priority InvoicePriority
}
`,
			"invoice_priority.go": `package entity

type InvoicePriority int

const (
	PriorityLow InvoicePriority = iota
	PriorityMedium
	PriorityHigh
)
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "invoice")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, schema.Columns, 2)
		priority := schema.Columns[1]
		assert.Equal(t, "priority", priority.Name)
		assert.Equal(t, "INTEGER", priority.DataType)
		assert.Equal(t, "0,1,2", priority.Check)
	})

	t.Run("recognizes local named int64 type with explicit consts as an enum field", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"ticket.go": `package entity

import "github.com/google/uuid"

type Ticket struct {
	ID       uuid.UUID
	Severity TicketSeverity
}
`,
			"ticket_severity.go": `package entity

type TicketSeverity int64

const (
	SeverityLow    TicketSeverity = 1
	SeverityMedium TicketSeverity = 5
	SeverityHigh   TicketSeverity = 9
)
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "ticket")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, schema.Columns, 2)
		severity := schema.Columns[1]
		assert.Equal(t, "severity", severity.Name)
		assert.Equal(t, "BIGINT", severity.DataType)
		assert.Equal(t, "1,5,9", severity.Check, "explicit non-contiguous values preserved, sorted numerically")
	})

	t.Run("named type with no const block is still an unsupported field type", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"widget.go": `package entity

import "github.com/google/uuid"

type Widget struct {
	ID   uuid.UUID
	Kind WidgetKind
}
`,
			"widget_kind.go": `package entity

type WidgetKind string
`,
		})

		// ===== Act ===== //
		_, err := Parse(module, "widget")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported field type WidgetKind")
	})

	t.Run("rejects enum const expressions beyond bare iota and literals", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"flag.go": `package entity

import "github.com/google/uuid"

type Flag struct {
	ID   uuid.UUID
	Bits FlagBits
}
`,
			"flag_bits.go": `package entity

type FlagBits int

const (
	FlagNone FlagBits = 1 << iota
	FlagSome
)
`,
		})

		// ===== Act ===== //
		_, err := Parse(module, "flag")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported enum const expression")
	})

	t.Run("flattens a same-dir value-object struct field into prefixed columns", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID   uuid.UUID
	Cust Customer
}
`,
			"customer.go": `package entity

type Customer struct {
	Name  string
	Email string
}
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []Column{
			{Name: "id", DataType: "UUID", Primary: true},
			{Name: "cust_name", DataType: "TEXT", NotNull: true},
			{Name: "cust_email", DataType: "TEXT", NotNull: true},
		}, schema.Columns)
	})

	t.Run("field name prefixes two value-object fields of the same type without collision", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID     uuid.UUID
	Buyer  Customer
	Seller Customer
}
`,
			"customer.go": `package entity

type Customer struct {
	Name string
}
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []Column{
			{Name: "id", DataType: "UUID", Primary: true},
			{Name: "buyer_name", DataType: "TEXT", NotNull: true},
			{Name: "seller_name", DataType: "TEXT", NotNull: true},
		}, schema.Columns)
	})

	t.Run("recursively flattens a nested value object to any depth", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID    uuid.UUID
	Buyer Customer
}
`,
			"customer.go": `package entity

type Customer struct {
	Address Address
}
`,
			"address.go": `package entity

type Address struct {
	City string
}
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, []Column{
			{Name: "id", DataType: "UUID", Primary: true},
			{Name: "buyer_address_city", DataType: "TEXT", NotNull: true},
		}, schema.Columns)
	})

	t.Run("errors on a cyclic value-object reference", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID    uuid.UUID
	Buyer Customer
}
`,
			"customer.go": `package entity

type Customer struct {
	Self Customer
}
`,
		})

		// ===== Act ===== //
		_, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cyclic value-object reference")
	})

	t.Run("errors when a value-object field references a struct that has its own primary key", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID   uuid.UUID
	Cust Customer
}
`,
			"customer.go": `package entity

import "github.com/google/uuid"

type Customer struct {
	ID   uuid.UUID
	Name string
}
`,
		})

		// ===== Act ===== //
		_, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "value object cannot declare a primary key")
	})

	t.Run("errors when a value-object field carries a pk annotation instead of bare ID", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID   uuid.UUID
	Cust Customer
}
`,
			"customer.go": `package entity

type Customer struct {
	Code string // pk
}
`,
		})

		// ===== Act ===== //
		_, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "value object cannot declare a primary key")
	})

	t.Run("rejects inline DSL on a value-object field", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID   uuid.UUID
	Cust Customer // null
}
`,
			"customer.go": `package entity

type Customer struct {
	Name string
}
`,
		})

		// ===== Act ===== //
		_, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DSL annotations are not allowed on a value-object field")
	})

	t.Run("errors when a flattened value-object column collides with an existing column name", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID        uuid.UUID
	CustName  string
	Cust      Customer
}
`,
			"customer.go": `package entity

type Customer struct {
	Name string
}
`,
		})

		// ===== Act ===== //
		_, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `column "cust_name"`)
	})

	t.Run("keeps FK and enum semantics for fields nested inside a value object", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID   uuid.UUID
	Cust Customer
}
`,
			"customer.go": `package entity

import "github.com/google/uuid"

type Customer struct {
	RoleID uuid.UUID
	Status PaymentStatus
}
`,
			"payment_status.go": `package entity

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentPaid    PaymentStatus = "paid"
)
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.NoError(t, err)
		require.Len(t, schema.Columns, 3)
		assert.Equal(t, "cust_role_id", schema.Columns[1].Name)
		assert.Equal(t, "cust_status", schema.Columns[2].Name)
		assert.Equal(t, "VARCHAR(7)", schema.Columns[2].DataType)
		assert.Equal(t, []ForeignKey{
			{Column: "cust_role_id", RefTable: "roles", RefColumn: "id", OnDelete: ""},
		}, schema.ForeignKeys)
	})

	t.Run("scopes composite group labels inside a value object by field path", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID     uuid.UUID
	Buyer  Address
	Seller Address
}
`,
			"address.go": `package entity

type Address struct {
	City   string // unique:loc
	Street string // unique:loc
}
`,
		})

		// ===== Act ===== //
		schema, err := Parse(module, "payment")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, [][]string{
			{"buyer_city", "buyer_street"},
			{"seller_city", "seller_street"},
		}, schema.Uniques)
	})

	t.Run("reordering enum consts produces the same canonical Check", func(t *testing.T) {
		// ===== Arrange ===== //
		moduleA := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID     uuid.UUID
	Status PaymentStatus
}
`,
			"payment_status.go": `package entity

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentPaid    PaymentStatus = "paid"
	PaymentFailed  PaymentStatus = "failed"
)
`,
		})
		moduleB := makeTestMigrationModuleFiles(t, "billing", map[string]string{
			"payment.go": `package entity

import "github.com/google/uuid"

type Payment struct {
	ID     uuid.UUID
	Status PaymentStatus
}
`,
			"payment_status.go": `package entity

type PaymentStatus string

const (
	PaymentFailed  PaymentStatus = "failed"
	PaymentPaid    PaymentStatus = "paid"
	PaymentPending PaymentStatus = "pending"
)
`,
		})

		// ===== Act ===== //
		schemaA, errA := Parse(moduleA, "payment")
		schemaB, errB := Parse(moduleB, "payment")

		// ===== Assert ===== //
		assert.NoError(t, errA)
		assert.NoError(t, errB)
		assert.Equal(t, schemaA.Columns, schemaB.Columns, "const declaration order must not affect generated SQL")
	})
}

func Test_Migration_IsEntityFile(t *testing.T) {
	t.Run("a struct with a bare ID field is an entity", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "billing", "payment.go", `package entity

import "github.com/google/uuid"

type Payment struct {
	ID uuid.UUID
}
`)

		// ===== Act ===== //
		isEntity, err := IsEntityFile(module, "payment")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, isEntity)
	})

	t.Run("a struct with a composite pk (no bare ID) is still an entity", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "billing", "membership.go", `package entity

import "github.com/google/uuid"

type Membership struct {
	TeamID uuid.UUID // pk
	UserID uuid.UUID // pk
}
`)

		// ===== Act ===== //
		isEntity, err := IsEntityFile(module, "membership")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.True(t, isEntity)
	})

	t.Run("a struct with no primary key is a value object, not an entity", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "billing", "customer.go", `package entity

type Customer struct {
	Name  string
	Email string
}
`)

		// ===== Act ===== //
		isEntity, err := IsEntityFile(module, "customer")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.False(t, isEntity)
	})

	t.Run("a support file declaring no struct at all is still not an entity", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "billing", "payment_status.go", `package entity

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
)
`)

		// ===== Act ===== //
		isEntity, err := IsEntityFile(module, "payment_status")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.False(t, isEntity)
	})
}

func makeTestMigrationModule(t *testing.T, moduleName string, entityFile string, entitySource string) discover.Module {
	t.Helper()

	root := t.TempDir()
	entityDir := filepath.Join(root, "module", moduleName, "internal", "domain", "entity")
	migrationDir := filepath.Join(root, "module", moduleName, "migration")
	assert.NoError(t, os.MkdirAll(entityDir, 0755))
	assert.NoError(t, os.MkdirAll(migrationDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(entityDir, entityFile), []byte(entitySource), 0644))

	return discover.Module{
		Name:         moduleName,
		EntityDir:    entityDir,
		MigrationDir: migrationDir,
	}
}

// makeTestMigrationModuleFiles is like makeTestMigrationModule but writes several
// entity-dir files at once (e.g. an entity struct plus a sibling enum-definition file).
func makeTestMigrationModuleFiles(t *testing.T, moduleName string, files map[string]string) discover.Module {
	t.Helper()

	root := t.TempDir()
	entityDir := filepath.Join(root, "module", moduleName, "internal", "domain", "entity")
	migrationDir := filepath.Join(root, "module", moduleName, "migration")
	assert.NoError(t, os.MkdirAll(entityDir, 0755))
	assert.NoError(t, os.MkdirAll(migrationDir, 0755))
	for name, source := range files {
		assert.NoError(t, os.WriteFile(filepath.Join(entityDir, name), []byte(source), 0644))
	}

	return discover.Module{
		Name:         moduleName,
		EntityDir:    entityDir,
		MigrationDir: migrationDir,
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
		schema, err := Parse(module, "api_key")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, "api_keys", schema.Name)
		assert.Len(t, schema.Columns, 2)
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
		schema, err := Parse(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Empty(t, schema.ForeignKeys)
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
		_, err := Parse(module, "user")

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
		_, err := Parse(module, "user")

		// ===== Assert ===== //
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "embedded")
	})
}
