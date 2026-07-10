package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"

	"github.com/wiszel/wigrate/internal"
)

type makeMigrationFunc func(bool, ...string) error
type migrateUpFunc func(...string) error
type migrateDownFunc func(int, ...string) error
type migrateStatusFunc func(...string) error

type cliDependencies struct {
	makeMigration makeMigrationFunc
	migrateUp     migrateUpFunc
	migrateDown   migrateDownFunc
	migrateStatus migrateStatusFunc
}

func main() {
	deps := cliDependencies{
		makeMigration: internal.MakeMigration,
		migrateUp:     internal.MigrateUp,
		migrateDown:   internal.MigrateDown,
		migrateStatus: internal.MigrateStatus,
	}

	if err := run(os.Args[1:], deps); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, deps cliDependencies) error {
	if len(args) == 0 {
		printUsage()
		return fmt.Errorf("missing command")
	}

	switch args[0] {
	case "gen":
		return runGen(args[1:], deps.makeMigration)
	case "up":
		return runUp(args[1:], deps.migrateUp)
	case "down":
		return runDown(args[1:], deps.migrateDown)
	case "status":
		return runStatus(args[1:], deps.migrateStatus)
	case "-v", "--version", "version":
		printVersion()
		return nil
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// moduleFlags returns a flagset pre-loaded with the common -m/--module and --modules-dir flags.
func moduleFlags(name string) (*flag.FlagSet, *string, *string) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	moduleName := flags.String("module", "", "module name")
	flags.StringVar(moduleName, "m", "", "module name")
	modulesDir := flags.String("modules-dir", "module", "modules directory")
	return flags, moduleName, modulesDir
}

func runGen(args []string, makeMigration makeMigrationFunc) error {
	flags, moduleName, modulesDir := moduleFlags("gen")
	overwrite := flags.Bool("overwrite", false, "overwrite latest migration")
	flags.BoolVar(overwrite, "o", false, "overwrite latest migration")
	dryRun := flags.Bool("dry-run", false, "print what would be generated without writing")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	internal.ModulesDir = *modulesDir
	internal.DryRun = *dryRun
	return makeMigration(*overwrite, *moduleName)
}

func runUp(args []string, migrateUp migrateUpFunc) error {
	flags, moduleName, modulesDir := moduleFlags("up")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	internal.ModulesDir = *modulesDir
	return migrateUp(*moduleName)
}

func runDown(args []string, migrateDown migrateDownFunc) error {
	// Contract: wigrate down <steps> [flags]
	if len(args) == 0 {
		return fmt.Errorf("down steps is required")
	}
	steps, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("down steps must be a number")
	}
	if steps <= 0 {
		return fmt.Errorf("down steps must be greater than zero")
	}

	flags, moduleName, modulesDir := moduleFlags("down")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	internal.ModulesDir = *modulesDir
	return migrateDown(steps, *moduleName)
}

func runStatus(args []string, migrateStatus migrateStatusFunc) error {
	flags, moduleName, modulesDir := moduleFlags("status")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	internal.ModulesDir = *modulesDir
	return migrateStatus(*moduleName)
}

func printVersion() {
	bi, ok := debug.ReadBuildInfo()
	if ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		fmt.Printf("wigrate %s\n", bi.Main.Version)
	} else {
		fmt.Println("wigrate (dev)")
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `wigrate — schema migration generator for Go entity structs (PostgreSQL only)

Reads Go entity structs via go/ast, diffs them against replayed migration history,
generates SQL migration files, and delegates execution to the golang-migrate CLI.
Each module owns its schema under module/<name>/migration/ with its own
golang-migrate tracking table (schema_migrations_<name>).

Usage:
  wigrate gen [-o|--overwrite] [-m=<module>|--module=<module>] [--modules-dir=<dir>] [--dry-run]
  wigrate up [-m=<module>|--module=<module>] [--modules-dir=<dir>]
  wigrate down <steps> [-m=<module>|--module=<module>] [--modules-dir=<dir>]
  wigrate status [-m=<module>|--module=<module>] [--modules-dir=<dir>]
  wigrate version

Commands:
  gen       Discover modules, parse entity structs, diff vs migration history, write SQL.
  up        Apply pending migrations via golang-migrate.
  down      Roll back <steps> migrations (step count required, no implicit "1").
  status    Print current migration version and dirty state per module.
  version   Print wigrate build version.

Flags:
  -o, --overwrite         Overwrite the latest migration instead of creating a new alter (gen only)
  -m, --module=<name>     Restrict to one module (empty/omitted = all modules)
      --modules-dir=<dir> Base directory for modules, absolute or relative to project root (default "module")
      --dry-run           Print generated SQL without writing files or invoking migrate (gen only)

Module layout (required):
  module/<name>/internal/domain/entity/<entity>.go   — Go struct, file name = struct name in snake_case
  module/<name>/migration/                           — generated *.up.sql / *.down.sql + tracking table
  module/<name>/migration/.wigrateignore              — optional: entity names (one per line, # comments ok)
                                                         to exclude from migration (e.g. a Redis-only entity)

Entity field comment DSL (inline, trailing comment only):
  <number>        string length -> VARCHAR(n); no number -> TEXT
  null            column is nullable (pointer types are nullable by default, no annotation needed)
  unique          add UNIQUE constraint
  pk              mark PRIMARY KEY (default: field named ID)
  ref:<table>     foreign key target table (default: derived from "<Name>ID" field -> snake_case, pluralized)
  del:<rule>      ON DELETE rule for a foreign key: cascade | setnull | restrict | noaction
  A human-readable description does NOT go inline (every inline token must be valid DSL above and
  will error otherwise) — put it in the comment ABOVE the field instead, which is never parsed:
    // DPoP key thumbprint bound at login
    Thumbprint string // 100 unique

Naming conventions:
  Struct/field PascalCase -> table/column snake_case, table names pluralized.
  FK column: fk_<table>_<refTable>. Unique constraint: uq_<table>_<column>.

Supported Go types: string, int, int32, int64, bool, float32, float64, time.Time, uuid.UUID.
Limitations: no default-value DSL; PK changes are blocked in alter migrations (v1).

Run "wigrate <command> --help" for command-specific flags.
`)
}
