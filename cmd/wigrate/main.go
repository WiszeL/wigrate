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
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  wigrate gen [-o|--overwrite] [-m=<module>|--module=<module>] [--modules-dir=<dir>] [--dry-run]")
	fmt.Fprintln(os.Stderr, "  wigrate up [-m=<module>|--module=<module>] [--modules-dir=<dir>]")
	fmt.Fprintln(os.Stderr, "  wigrate down <steps> [-m=<module>|--module=<module>] [--modules-dir=<dir>]")
	fmt.Fprintln(os.Stderr, "  wigrate status [-m=<module>|--module=<module>] [--modules-dir=<dir>]")
	fmt.Fprintln(os.Stderr, "  wigrate version")
}
