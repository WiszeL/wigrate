package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_CLI_Run(t *testing.T) {
	t.Run("passes overwrite and module flags to generator", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()
		deps.makeMigration = func(overwriteLatest bool, moduleNames ...string) error {
			assert.True(t, overwriteLatest)
			assert.Equal(t, []string{"iam"}, moduleNames)
			return nil
		}

		// ===== Act =====
		err := run([]string{"gen", "-o", "-m=iam"}, deps)

		// ===== Assert =====
		require.NoError(t, err)
	})

	t.Run("passes empty module when module flag is omitted", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()
		deps.makeMigration = func(overwriteLatest bool, moduleNames ...string) error {
			assert.False(t, overwriteLatest)
			assert.Equal(t, []string{""}, moduleNames)
			return nil
		}

		// ===== Act =====
		err := run([]string{"gen"}, deps)

		// ===== Assert =====
		require.NoError(t, err)
	})

	t.Run("passes empty module filter to up", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()
		deps.migrateUp = func(moduleNames ...string) error {
			assert.Equal(t, []string{""}, moduleNames)
			return nil
		}

		// ===== Act =====
		err := run([]string{"up"}, deps)

		// ===== Assert =====
		require.NoError(t, err)
	})

	t.Run("passes selected module to up", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()
		deps.migrateUp = func(moduleNames ...string) error {
			assert.Equal(t, []string{"iam"}, moduleNames)
			return nil
		}

		// ===== Act =====
		err := run([]string{"up", "--module=iam"}, deps)

		// ===== Assert =====
		require.NoError(t, err)
	})

	t.Run("passes steps to down", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()
		deps.migrateDown = func(steps int, moduleNames ...string) error {
			assert.Equal(t, 1, steps)
			assert.Equal(t, []string{""}, moduleNames)
			return nil
		}

		// ===== Act =====
		err := run([]string{"down", "1"}, deps)

		// ===== Assert =====
		require.NoError(t, err)
	})

	t.Run("passes steps and selected module to down", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()
		deps.migrateDown = func(steps int, moduleNames ...string) error {
			assert.Equal(t, 1, steps)
			assert.Equal(t, []string{"iam"}, moduleNames)
			return nil
		}

		// ===== Act =====
		err := run([]string{"down", "1", "-m=iam"}, deps)

		// ===== Assert =====
		require.NoError(t, err)
	})

	t.Run("passes selected module before steps to down", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()
		deps.migrateDown = func(steps int, moduleNames ...string) error {
			assert.Equal(t, 1, steps)
			assert.Equal(t, []string{"iam"}, moduleNames)
			return nil
		}

		// ===== Act =====
		err := run([]string{"down", "-m=iam", "1"}, deps)

		// ===== Assert =====
		require.NoError(t, err)
	})

	t.Run("rejects down without steps", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()

		// ===== Act =====
		err := run([]string{"down"}, deps)

		// ===== Assert =====
		require.Error(t, err)
		assert.Contains(t, err.Error(), "down steps is required")
	})

	t.Run("rejects down with non numeric steps", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()

		// ===== Act =====
		err := run([]string{"down", "abc"}, deps)

		// ===== Assert =====
		require.Error(t, err)
		assert.Contains(t, err.Error(), "down steps must be a number")
	})

	t.Run("rejects down with zero steps", func(t *testing.T) {
		// ===== Arrange =====
		deps := stubDependencies()

		// ===== Act =====
		err := run([]string{"down", "0"}, deps)

		// ===== Assert =====
		require.Error(t, err)
		assert.Contains(t, err.Error(), "down steps must be greater than zero")
	})
}

func stubDependencies() cliDependencies {
	return cliDependencies{
		makeMigration: func(bool, ...string) error {
			return nil
		},
		migrateUp: func(...string) error {
			return nil
		},
		migrateDown: func(int, ...string) error {
			return nil
		},
	}
}
