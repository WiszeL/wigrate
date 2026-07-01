package internal

import (
	"errors"
	"os"
	"path/filepath"
)

func FindRoot() (string, error) {
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

	// go.mod not found means in production environment, use executable dir instead
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable), nil
	}

	return "", errors.New("root not found")
}
