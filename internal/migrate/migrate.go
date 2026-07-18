// Package migrate applies migration files using the golang-migrate CLI.
package migrate

import (
	"fmt"
	"strconv"

	"github.com/wiszel/wigrate/internal"
	"github.com/wiszel/wigrate/internal/database"
	"github.com/wiszel/wigrate/internal/discover"
)

// Up runs all pending migrations.
func Up(moduleNames ...string) error {
	return migrateModules("up", "", moduleNames...)
}

// Status shows the current migration version.
func Status(moduleNames ...string) error {
	return migrateModules("version", "", moduleNames...)
}

// Down rolls back the given number of migrations.
func Down(steps int, moduleNames ...string) error {
	if steps <= 0 {
		return fmt.Errorf("down steps must be greater than zero")
	}

	return migrateModules("down", strconv.Itoa(steps), moduleNames...)
}

func migrateModules(direction string, steps string, moduleNames ...string) error {
	// Finding the project root
	root, err := config.FindRoot()
	if err != nil {
		return err
	}

	// Loading the database config
	dbConfig, err := database.Load(root)
	if err != nil {
		return err
	}

	// Discovering the modules
	modules, err := discover.FindModules()
	if err != nil {
		return err
	}

	// Filtering the modules
	modules, err = discover.FilterModules(modules, moduleNames...)
	if err != nil {
		return err
	}

	for _, module := range modules {
		// Running the migration
		if ok, err := hasMigrationFiles(module); err != nil {
			return err
		} else if !ok {
			fmt.Printf("No migration files found for module %s.\n", module.Name)
			continue
		}

		if err := runMigrateCommand(module, dbConfig, direction, steps); err != nil {
			return err
		}
	}

	return nil
}
