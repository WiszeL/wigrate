package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func Test_LoadIgnoreSet(t *testing.T) {
	t.Run("returns nil when .wigrateignore is missing", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()

		// ===== Act ===== //
		set, err := loadIgnoreSet(dir)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Nil(t, set)
	})

	t.Run("parses entity names, skipping blanks and comments", func(t *testing.T) {
		// ===== Arrange ===== //
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".wigrateignore"), []byte("session\n\n# not migrated\ncache_entry\n"), 0644))

		// ===== Act ===== //
		set, err := loadIgnoreSet(dir)

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"session": {}, "cache_entry": {}}, set)
	})
}

func Test_ModuleTableSet(t *testing.T) {
	t.Run("includes real entities, excludes ignored and struct-less support files", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID uuid.UUID
}
`)
		require.NoError(t, os.WriteFile(filepath.Join(module.EntityDir, "role.go"), []byte(`package entity

import "github.com/google/uuid"

type Role struct {
	ID uuid.UUID
}
`), 0644))
		// session is a real struct but ignored via .wigrateignore (e.g. Redis-only).
		require.NoError(t, os.WriteFile(filepath.Join(module.EntityDir, "session.go"), []byte(`package entity

import "github.com/google/uuid"

type Session struct {
	ID uuid.UUID
}
`), 0644))
		// permission_level.go declares no struct — a support file (enum), not an entity.
		require.NoError(t, os.WriteFile(filepath.Join(module.EntityDir, "permission_level.go"), []byte(`package entity

type PermissionLevel int

const (
	PermissionRead PermissionLevel = iota
	PermissionWrite
)
`), 0644))

		entries, err := os.ReadDir(module.EntityDir)
		require.NoError(t, err)
		ignore := map[string]struct{}{"session": {}}

		// ===== Act ===== //
		set, err := moduleTableSet(module, entries, ignore)

		// ===== Assert ===== //
		require.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"users": {}, "roles": {}}, set)
	})
}

func Test_MakePerModule_EnumSupportFile(t *testing.T) {
	t.Run("skips a sibling enum-definition file with no matching struct, without needing .wigrateignore", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "billing", "payment.go", `package entity

import "github.com/google/uuid"

type Payment struct {
	ID     uuid.UUID
	Status PaymentStatus
}
`)
		// payment_status.go declares no struct — it's a support file for the
		// enum, not an entity of its own, and must not need .wigrateignore.
		require.NoError(t, os.WriteFile(filepath.Join(module.EntityDir, "payment_status.go"), []byte(`package entity

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentPaid    PaymentStatus = "paid"
)
`), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			upPath := filepath.Join(module.MigrationDir, "000001_init_payment.up.sql")
			downPath := filepath.Join(module.MigrationDir, "000001_init_payment.down.sql")
			require.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			require.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makePerModule(module, false)

		// ===== Assert ===== //
		assert.NoError(t, err, "a struct-less sibling file must be skipped, not treated as a broken entity")

		upSQL, readErr := os.ReadFile(filepath.Join(module.MigrationDir, "000001_init_payment.up.sql"))
		assert.NoError(t, readErr)
		assert.Contains(t, string(upSQL), "CREATE TABLE payments")
	})
}

func Test_MakePerModule_ValueObjectSupportFile(t *testing.T) {
	t.Run("flattens a sibling value-object struct into the entity table and generates no table of its own", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "billing", "payment.go", `package entity

import "github.com/google/uuid"

type Payment struct {
	ID   uuid.UUID
	Cust Customer
}
`)
		// customer.go has no primary key — a value object, flattened into
		// payments, never gets a customers table of its own.
		require.NoError(t, os.WriteFile(filepath.Join(module.EntityDir, "customer.go"), []byte(`package entity

type Customer struct {
	Name  string
	Email string
}
`), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			upPath := filepath.Join(module.MigrationDir, "000001_init_payment.up.sql")
			downPath := filepath.Join(module.MigrationDir, "000001_init_payment.down.sql")
			require.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			require.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makePerModule(module, false)

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, readErr := os.ReadFile(filepath.Join(module.MigrationDir, "000001_init_payment.up.sql"))
		assert.NoError(t, readErr)
		assert.Contains(t, string(upSQL), "CREATE TABLE payments")
		assert.Contains(t, string(upSQL), "cust_name")
		assert.Contains(t, string(upSQL), "cust_email")
		assert.NotContains(t, string(upSQL), "CREATE TABLE customers")

		_, statErr := os.Stat(filepath.Join(module.MigrationDir, "000001_init_customer.up.sql"))
		assert.True(t, os.IsNotExist(statErr), "value object must not get its own migration file")
	})
}

func Test_MakePerModule_WigrateIgnore(t *testing.T) {
	t.Run("skips entities listed in .wigrateignore", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID uuid.UUID
}
`)
		// Session is Redis-only: plain (non-DSL) inline comment would otherwise abort generation.
		require.NoError(t, os.WriteFile(filepath.Join(module.EntityDir, "session.go"), []byte(`package entity

type Session struct {
	ID          string
	Thumbprint string // DPoP key thumbprint bound at login
}
`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(module.MigrationDir, ".wigrateignore"), []byte("session\n"), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			upPath := filepath.Join(module.MigrationDir, "000001_init_user.up.sql")
			downPath := filepath.Join(module.MigrationDir, "000001_init_user.down.sql")
			require.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			require.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makePerModule(module, false)

		// ===== Assert ===== //
		assert.NoError(t, err, "session.go's invalid DSL comment must not abort generation once ignored")

		upSQL, readErr := os.ReadFile(filepath.Join(module.MigrationDir, "000001_init_user.up.sql"))
		assert.NoError(t, readErr)
		assert.Contains(t, string(upSQL), "CREATE TABLE users")
	})
}
