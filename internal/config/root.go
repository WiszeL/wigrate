package config

import (
	"errors"
	"os"
	"path/filepath"
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
