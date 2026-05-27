package internal

import (
	"errors"
	"os"
	"path/filepath"
)

func FindRoot() (string, error) {
	// Try to find with `go.mod` first
	if wd, err := os.Getwd(); err == nil {
		if root, ok := findParentWithAnchor(wd, "go.mod"); ok {
			return root, nil
		}
	}

	// go.mod not found means in production environment, use executable dir instead
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable), nil
	}

	// Root not found
	return "", errors.New("root not found")
}

func findParentWithAnchor(start string, anchor string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, anchor)); err == nil {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}

		dir = parent
	}
}
