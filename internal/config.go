package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CLI flags set in cmd/wigrate/main.go and used throughout
var (
	ModulesDir = "module"
	DryRun     = false
)

// Finding project root by looking for go.mod
func FindRoot() (string, error) {
	// Walking up from current directory to find go.mod
	if wd, err := os.Getwd(); err == nil {
		dir, err := filepath.Abs(wd)
		if err == nil {
			for {
				if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
					return dir, nil
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		}
	}

	// Using executable directory if go.mod not found
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable), nil
	}

	return "", errors.New("root not found")
}

// Executing external commands (prints when dry run)
var RunCommandFunc = func(cmd string, args ...string) error {
	if DryRun {
		fmt.Printf("[dry-run] %s %s\n", cmd, strings.Join(args, " "))
		return nil
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("%s not found in PATH — install golang-migrate: https://github.com/golang-migrate/migrate", cmd)
	}
	command := exec.Command(cmd, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	return command.Run()
}
