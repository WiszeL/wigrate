package internal

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	t.Run("dry-run previews init migration without writing files", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "iam", "user.go", `package entity

import "github.com/google/uuid"

type User struct {
	ID    uuid.UUID
	Email string // 20 unique
}
`)

		restoreDryRun := stubDryRun(t, true)
		defer restoreDryRun()

		// migrate create is a no-op under dry-run; assert it's never called for real work.
		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			assert.Equal(t, "migrate", cmd)
			assert.Equal(t, []string{"create", "-ext", "sql", "-dir", module.migrationDir, "-seq", "init_user"}, args)
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makeInitMigration(module, "user")

		// ===== Assert ===== //
		require.NoError(t, err)

		entries, readErr := os.ReadDir(module.migrationDir)
		require.NoError(t, readErr)
		assert.Empty(t, entries, "dry-run must not write migration files to disk")
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
		entries, err := os.ReadDir(module.migrationDir)
		require.NoError(t, err)
		err = makeAlterMigration(module, entries, "user")

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
    CONSTRAINT fk_users_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
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
		entries, err := os.ReadDir(module.migrationDir)
		require.NoError(t, err)
		err = makeAlterMigration(module, entries, "user")

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
		entries, err := os.ReadDir(module.migrationDir)
		require.NoError(t, err)
		err = makeAlterMigration(module, entries, "user")

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
		entries, err := os.ReadDir(module.migrationDir)
		require.NoError(t, err)
		err = overwriteLatestMigration(module, entries, "user", latest)

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
		entries, err := os.ReadDir(module.migrationDir)
		require.NoError(t, err)
		err = overwriteLatestMigration(module, entries, "user", latest)

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
		entries, err := os.ReadDir(module.migrationDir)
		require.NoError(t, err)
		err = overwriteLatestMigration(module, entries, "user", latest)

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

func stubDryRun(t *testing.T, value bool) func() {
	t.Helper()

	original := DryRun
	DryRun = value

	return func() {
		DryRun = original
	}
}

func Test_Migration_MakeInitMigrationComposite(t *testing.T) {
	t.Run("creates init migration with composite PK and composite unique", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "membership.go", `package entity

import "github.com/google/uuid"

type Membership struct {
	Team   uuid.UUID // pk
	User   uuid.UUID // pk
	RoleID uuid.UUID // unique:role del:cascade
	Label  string    // unique:role
}
`)

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			upPath := filepath.Join(module.migrationDir, "000001_init_membership.up.sql")
			downPath := filepath.Join(module.migrationDir, "000001_init_membership.down.sql")
			assert.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			assert.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		err := makeInitMigration(module, "membership")

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000001_init_membership.up.sql"))
		assert.NoError(t, err)

		assert.Equal(t, `CREATE TABLE memberships (
    team UUID NOT NULL,
    user UUID NOT NULL,
    role_id UUID NOT NULL,
    label TEXT NOT NULL,
    PRIMARY KEY (team, user),
    CONSTRAINT uq_memberships_role_id_label UNIQUE (role_id, label),
    CONSTRAINT fk_memberships_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);
`, string(upSQL))
	})
}

func Test_Migration_MakeAlterMigrationCompositeUnique(t *testing.T) {
	t.Run("adds and removes a composite unique constraint via alter", func(t *testing.T) {
		// ===== Arrange ===== //
		module := makeTestMigrationModule(t, "shop", "membership.go", `package entity

import "github.com/google/uuid"

type Membership struct {
	ID   uuid.UUID
	Team uuid.UUID // unique:member
	User uuid.UUID // unique:member
}
`)
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_membership.up.sql"), []byte(`CREATE TABLE memberships (
    id UUID PRIMARY KEY,
    team UUID NOT NULL,
    user UUID NOT NULL
);
`), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(module.migrationDir, "000001_init_membership.down.sql"), []byte("DROP TABLE IF EXISTS memberships;\n"), 0644))

		restoreRunCommand := stubRunCommand(t, func(cmd string, args ...string) error {
			upPath := filepath.Join(module.migrationDir, "000002_alter_team_user_membership.up.sql")
			downPath := filepath.Join(module.migrationDir, "000002_alter_team_user_membership.down.sql")
			assert.NoError(t, os.WriteFile(upPath, []byte(""), 0644))
			assert.NoError(t, os.WriteFile(downPath, []byte(""), 0644))
			return nil
		})
		defer restoreRunCommand()

		// ===== Act ===== //
		entries, err := os.ReadDir(module.migrationDir)
		require.NoError(t, err)
		err = makeAlterMigration(module, entries, "membership")

		// ===== Assert ===== //
		assert.NoError(t, err)

		upSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000002_alter_team_user_membership.up.sql"))
		assert.NoError(t, err)
		downSQL, err := os.ReadFile(filepath.Join(module.migrationDir, "000002_alter_team_user_membership.down.sql"))
		assert.NoError(t, err)

		assert.Equal(t, `ALTER TABLE memberships
    ADD CONSTRAINT uq_memberships_team_user UNIQUE (team, user);
`, string(upSQL))
		assert.Equal(t, `ALTER TABLE memberships
    DROP CONSTRAINT IF EXISTS uq_memberships_team_user;
`, string(downSQL))
	})
}
