package internal

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Migration_MakeInitMigration(t *testing.T) {
	t.Run("creates init migration files from entity schema", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID    uuid.UUID
	Email string // 20 unique
}
`)

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			assert.Equal(t, "migrate", cmd)
			assert.Equal(t, []string{"create", "-ext", "sql", "-dir", module.migrationDir, "-seq", "init_user"}, args)

			upPath := filepath.Join(module.migrationDir, "000001_init_user.up.sql")
			downPath := filepath.Join(module.migrationDir, "000001_init_user.down.sql")
			assert.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			assert.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makeInitMigration(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000001_init_user.up.sql"))
		assert.NoError(t, err)
		downSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000001_init_user.down.sql"))
		assert.NoError(t, err)

		assert.Equal(t, `CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(20) NOT NULL UNIQUE
);
`, string(upSQL))
		assert.Equal(t, "DROP TABLE IF EXISTS users;\n", string(downSQL))
	})
}

func Test_Migration_MakeAlterMigration(t *testing.T) {
	t.Run("creates alter migration for added columns and foreign keys", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID     uuid.UUID
	Email  string // 20
	Name   string // 50
	RoleID uuid.UUID // ref:roles del:cascade
}
`)
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.up.sql"), []byte(`CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(20) NOT NULL
);
`), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.down.sql"), []byte("DROP TABLE IF EXISTS users;\n"), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			assert.Equal(t, "migrate", cmd)
			assert.Equal(t, []string{"create", "-ext", "sql", "-dir", module.migrationDir, "-seq", "alter_name_role_id_user"}, args)

			upPath := filepath.Join(module.migrationDir, "000002_alter_name_role_id_user.up.sql")
			downPath := filepath.Join(module.migrationDir, "000002_alter_name_role_id_user.down.sql")
			assert.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			assert.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makeAlterMigration(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000002_alter_name_role_id_user.up.sql"))
		assert.NoError(t, err)
		downSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000002_alter_name_role_id_user.down.sql"))
		assert.NoError(t, err)

		assert.Equal(t, `ALTER TABLE users
    ADD COLUMN name VARCHAR(50) NOT NULL,
    ADD COLUMN role_id UUID NOT NULL,
    ADD CONSTRAINT fk_users_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
`, string(upSQL))
		assert.Equal(t, `ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_role_id,
    DROP COLUMN IF EXISTS role_id,
    DROP COLUMN IF EXISTS name;
`, string(downSQL))
	})
}

func Test_Migration_MakeAlterMigrationFullDiff(t *testing.T) {
	t.Run("creates alter migration for mixed schema changes", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID       uuid.UUID
	Email    string // 20
	Age      int // null unique
	Nickname string
	RoleID   uuid.UUID // ref:teams del:restrict
	NewCode  string // 10
}
`)
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.up.sql"), []byte(`CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL,
    age INTEGER NOT NULL,
    nickname TEXT NOT NULL,
    role_id UUID NOT NULL,
    obsolete TEXT NOT NULL,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);
`), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.down.sql"), []byte("DROP TABLE IF EXISTS users;\n"), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			assert.Equal(t, "migrate", cmd)
			assert.Equal(t, []string{"create", "-ext", "sql", "-dir", module.migrationDir, "-seq", "alter_new_code_obsolete_etc_user"}, args)

			upPath := filepath.Join(module.migrationDir, "000002_alter_new_code_obsolete_etc_user.up.sql")
			downPath := filepath.Join(module.migrationDir, "000002_alter_new_code_obsolete_etc_user.down.sql")
			assert.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			assert.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makeAlterMigration(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000002_alter_new_code_obsolete_etc_user.up.sql"))
		assert.NoError(t, err)
		downSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000002_alter_new_code_obsolete_etc_user.down.sql"))
		assert.NoError(t, err)

		assert.Equal(t, `ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_role_id,
    DROP COLUMN IF EXISTS obsolete,
    ALTER COLUMN email TYPE VARCHAR(20),
    ALTER COLUMN age DROP NOT NULL,
    ADD CONSTRAINT uq_users_age UNIQUE (age),
    ADD COLUMN new_code VARCHAR(10) NOT NULL,
    ADD CONSTRAINT fk_users_role_id FOREIGN KEY (role_id) REFERENCES teams(id) ON DELETE RESTRICT;
`, string(upSQL))
		assert.Equal(t, `ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_role_id,
    DROP COLUMN IF EXISTS new_code,
    DROP CONSTRAINT IF EXISTS uq_users_age,
    ALTER COLUMN age SET NOT NULL,
    ALTER COLUMN email TYPE TEXT,
    ADD COLUMN obsolete TEXT NOT NULL,
    ADD CONSTRAINT fk_users_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
`, string(downSQL))
	})
}

func Test_Migration_MakeAlterMigrationNoChanges(t *testing.T) {
	t.Run("does not create migration when schema is unchanged", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID    uuid.UUID
	Email string // 20 unique
}
`)
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.up.sql"), []byte(`CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(20) NOT NULL UNIQUE
);
`), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.down.sql"), []byte("DROP TABLE IF EXISTS users;\n"), 0644))

		called := false
		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			called = true
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makeAlterMigration(module, "user")

		// ===== Assert ===== //
		assert.NoError(t, err)
		assert.False(t, called)
	})
}

func Test_Migration_OverwriteLatestMigration(t *testing.T) {
	t.Run("overwrites init migration pair", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID    uuid.UUID
	Email string // 20 unique
}
`)
		upPath := filepath.Join(module.migrationDir, "000001_init_user.up.sql")
		downPath := filepath.Join(module.migrationDir, "000001_init_user.down.sql")
		assert.NoError(t, os.WriteFile(upPath, []byte("-- old up\n"), 0644))
		assert.NoError(t, os.WriteFile(downPath, []byte("-- old down\n"), 0644))

		latest := migrationFile{
			path:      upPath,
			baseName:  "000001_init_user",
			kind:      migrationKindInit,
			direction: "up",
		}

		// ===== Act ===== //
		err := overwriteLatestMigration(module, "user", latest)

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(upPath)
		assert.NoError(t, err)
		downSQL, err := os.ReadFile(downPath)
		assert.NoError(t, err)

		assert.Equal(t, `CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(20) NOT NULL UNIQUE
);
`, string(upSQL))
		assert.Equal(t, "DROP TABLE IF EXISTS users;\n", string(downSQL))
	})

	t.Run("overwrites alter migration pair", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID     uuid.UUID
	Email  string // 20
	Name   string // 50
	RoleID uuid.UUID // ref:roles del:cascade
}
`)
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.up.sql"), []byte(`CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(20) NOT NULL
);
`), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.down.sql"), []byte("DROP TABLE IF EXISTS users;\n"), 0644))
		upPath := filepath.Join(module.migrationDir, "000002_alter_name_user.up.sql")
		downPath := filepath.Join(module.migrationDir, "000002_alter_name_user.down.sql")
		assert.NoError(t, os.WriteFile(upPath, []byte("-- old up\n"), 0644))
		assert.NoError(t, os.WriteFile(downPath, []byte("-- old down\n"), 0644))

		latest := migrationFile{
			path:      upPath,
			baseName:  "000002_alter_name_user",
			kind:      migrationKindAlter,
			direction: "up",
		}

		// ===== Act ===== //
		err := overwriteLatestMigration(module, "user", latest)

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(upPath)
		assert.NoError(t, err)
		downSQL, err := os.ReadFile(downPath)
		assert.NoError(t, err)

		assert.Equal(t, `ALTER TABLE users
    ADD COLUMN name VARCHAR(50) NOT NULL,
    ADD COLUMN role_id UUID NOT NULL,
    ADD CONSTRAINT fk_users_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
`, string(upSQL))
		assert.Equal(t, `ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_role_id,
    DROP COLUMN IF EXISTS role_id,
    DROP COLUMN IF EXISTS name;
`, string(downSQL))
	})

	t.Run("skips unchanged alter overwrite", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID    uuid.UUID
	Email string // 20 unique
	Name  string // 50
}
`)
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.up.sql"), []byte(`CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(20) NOT NULL UNIQUE
);
`), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_user.down.sql"), []byte("DROP TABLE IF EXISTS users;\n"), 0644))
		upPath := filepath.Join(module.migrationDir, "000002_alter_name_user.up.sql")
		downPath := filepath.Join(module.migrationDir, "000002_alter_name_user.down.sql")
		originalUp := `ALTER TABLE users
    ADD COLUMN name VARCHAR(50) NOT NULL;
`
		originalDown := `ALTER TABLE users
    DROP COLUMN IF EXISTS name;
`
		assert.NoError(t, os.WriteFile(upPath, []byte(originalUp), 0644))
		assert.NoError(t, os.WriteFile(downPath, []byte(originalDown), 0644))

		latest := migrationFile{
			path:      upPath,
			baseName:  "000002_alter_name_user",
			kind:      migrationKindAlter,
			direction: "up",
		}

		// ===== Act ===== //
		err := overwriteLatestMigration(module, "user", latest)

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(upPath)
		assert.NoError(t, err)
		downSQL, err := os.ReadFile(downPath)
		assert.NoError(t, err)
		assert.Equal(t, originalUp, string(upSQL))
		assert.Equal(t, originalDown, string(downSQL))
	})
}

func stubRunCommand(t *testing.T, stub func(cmd string, args ...string) error) func() {
	t.Helper()

	original := runCommandFunc
	runCommandFunc = func(cmd string, args ...string) error {
		return stub(cmd, slices.Clone(args)...)
	}

	return func() {
		runCommandFunc = original
	}
}
