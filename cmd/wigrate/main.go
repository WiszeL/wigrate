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

type cliDependencies struct {
	makeMigration makeMigrationFunc
	migrateUp     migrateUpFunc
	migrateDown   migrateDownFunc
}

func main() {
	deps := cliDependencies{
		makeMigration: internal.MakeMigration,
		migrateUp:     internal.MigrateUp,
		migrateDown:   internal.MigrateDown,
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
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runGen(args []string, makeMigration makeMigrationFunc) error {
	flags := flag.NewFlagSet("gen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	overwrite := flags.Bool("overwrite", false, "overwrite latest migration")
	flags.BoolVar(overwrite, "o", false, "overwrite latest migration")
	moduleName := flags.String("module", "", "module to generate")
	flags.StringVar(moduleName, "m", "", "module to generate")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	return makeMigration(*overwrite, *moduleName)
}

func runUp(args []string, migrateUp migrateUpFunc) error {
	flags := flag.NewFlagSet("up", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	moduleName := flags.String("module", "", "module to migrate")
	flags.StringVar(moduleName, "m", "", "module to migrate")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	return migrateUp(*moduleName)
}

func runDown(args []string, migrateDown migrateDownFunc) error {
	stepArg, flagArgs, err := splitDownArgs(args)
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("down", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	moduleName := flags.String("module", "", "module to migrate")
	flags.StringVar(moduleName, "m", "", "module to migrate")
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

	return migrateDown(steps, *moduleName)
}

func splitDownArgs(args []string) (string, []string, error) {
	var stepArg string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-m" || arg == "--module":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("flag needs an argument: %s", arg)
			}
			flagArgs = append(flagArgs, arg, args[i+1])
			i++
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
	if stepArg == "" {
		return "", nil, fmt.Errorf("down steps is required")
	}

	return stepArg, flagArgs, nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  wigrate gen [-o|--overwrite] [-m=<module>|--module=<module>]")
	fmt.Fprintln(os.Stderr, "  wigrate up [-m=<module>|--module=<module>]")
	fmt.Fprintln(os.Stderr, "  wigrate down <steps> [-m=<module>|--module=<module>]")
}
