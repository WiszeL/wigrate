package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
