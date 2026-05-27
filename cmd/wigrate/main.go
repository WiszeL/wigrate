package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

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
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runGen(args []string, makeMigration makeMigrationFunc) error {
	// Getting the flags
	flags := flag.NewFlagSet("gen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	// Defining the flags
	overwrite := flags.Bool("overwrite", false, "overwrite latest migration")
	flags.BoolVar(overwrite, "o", false, "overwrite latest migration")

	moduleName := flags.String("module", "", "module to generate")
	flags.StringVar(moduleName, "m", "", "module to generate")

	modulesDir := flags.String("modules-dir", "module", "modules directory")
	dryRun := flags.Bool("dry-run", false, "print what would be generated without writing")

	// Parsing the flags
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	internal.ModulesDir = *modulesDir
	internal.DryRun = *dryRun

	// Run the migration generation
	return makeMigration(*overwrite, *moduleName)
}

func runUp(args []string, migrateUp migrateUpFunc) error {
	// Getting the flags
	flags := flag.NewFlagSet("up", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	// Defining the flags
	moduleName := flags.String("module", "", "module to migrate")
	flags.StringVar(moduleName, "m", "", "module to migrate")
	modulesDir := flags.String("modules-dir", "module", "modules directory")

	// Parsing the flags
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	internal.ModulesDir = *modulesDir

	// Run the migration up
	return migrateUp(*moduleName)
}

func runDown(args []string, migrateDown migrateDownFunc) error {
	// The down command has a required positional argument for the number of steps,
	// so we need to handle flag parsing manually
	stepArg, flagArgs, err := splitDownArgs(args)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("down", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	// Defining the flags
	moduleName := flags.String("module", "", "module to migrate")
	flags.StringVar(moduleName, "m", "", "module to migrate")
	modulesDir := flags.String("modules-dir", "module", "modules directory")

	// Parsing the flags
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}

	steps, err := strconv.Atoi(stepArg)
	if err != nil {
		return fmt.Errorf("down steps must be a number")
	}
	if steps <= 0 {
		return fmt.Errorf("down steps must be greater than zero")
	}
	internal.ModulesDir = *modulesDir

	// Run the migration down
	return migrateDown(steps, *moduleName)
}

func runStatus(args []string, migrateStatus migrateStatusFunc) error {
	// Getting the flags
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	// Defining the flags
	moduleName := flags.String("module", "", "module to show status")
	flags.StringVar(moduleName, "m", "", "module to show status")
	modulesDir := flags.String("modules-dir", "module", "modules directory")

	// Parsing the flags
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	internal.ModulesDir = *modulesDir

	// Run the migration status
	return migrateStatus(*moduleName)
}

func splitDownArgs(args []string) (string, []string, error) {
	// Getting the step count from positional args
	var stepArg string
	var flagArgs []string
	needValue := false

	// Separating positional and flag arguments
	for _, arg := range args {
		if needValue {
			flagArgs = append(flagArgs, arg)
			needValue = false
			continue
		}
		switch {
		case arg == "-m" || arg == "--module":
			flagArgs = append(flagArgs, arg)
			needValue = true
		case strings.HasPrefix(arg, "-m=") || strings.HasPrefix(arg, "--module="):
			flagArgs = append(flagArgs, arg)
		case strings.HasPrefix(arg, "-"):
			flagArgs = append(flagArgs, arg)
		case stepArg == "":
			stepArg = arg
		default:
			return "", nil, fmt.Errorf("unexpected argument %q", arg)
		}
	}

	// Validating the split result
	if needValue {
		return "", nil, fmt.Errorf("flag needs an argument: -m")
	}
	if stepArg == "" {
		return "", nil, fmt.Errorf("down steps is required")
	}

	return stepArg, flagArgs, nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  wigrate gen [-o|--overwrite] [-m=<module>|--module=<module>] [--modules-dir=<dir>] [--dry-run]")
	fmt.Fprintln(os.Stderr, "  wigrate up [-m=<module>|--module=<module>] [--modules-dir=<dir>]")
	fmt.Fprintln(os.Stderr, "  wigrate down <steps> [-m=<module>|--module=<module>] [--modules-dir=<dir>]")
	fmt.Fprintln(os.Stderr, "  wigrate status [-m=<module>|--module=<module>] [--modules-dir=<dir>]")
}
