package discover

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wiszel/wigrate/internal"
)

type Module struct {
	Name         string
	MigrationDir string
	EntityDir    string
}

func FindModules() ([]Module, error) {
	var modules []Module

	// Finding project root
	root, err := config.FindRoot()
	if err != nil {
		return nil, err
	}

	// Resolving module path
	var modulesPath string
	if filepath.IsAbs(config.ModulesDir) {
		modulesPath = config.ModulesDir
	} else {
		modulesPath = filepath.Join(root, config.ModulesDir)
	}
	if _, err := os.Stat(modulesPath); errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("module path is not found")
	}

	// Reading module directories
	entries, err := os.ReadDir(modulesPath)
	if err != nil {
		return nil, fmt.Errorf("read modules: %w", err)
	}
	for _, entry := range entries {
		// Skipping files
		if !entry.IsDir() {
			continue
		}

		modulePath := filepath.Join(modulesPath, entry.Name())
		module := Module{
			Name:         entry.Name(),
			MigrationDir: filepath.Join(modulePath, "migration"),
			EntityDir:    filepath.Join(modulePath, "internal", "domain", "entity"),
		}
		modules = append(modules, module)
	}

	return modules, nil
}

// Filtering modules to the given names
func FilterModules(modules []Module, moduleNames ...string) ([]Module, error) {
	wanted := make(map[string]struct{})
	for _, moduleName := range moduleNames {
		moduleName = strings.TrimSpace(moduleName)
		if moduleName == "" {
			continue
		}
		wanted[moduleName] = struct{}{}
	}
	if len(wanted) == 0 {
		return modules, nil
	}

	var filtered []Module
	for _, module := range modules {
		if _, ok := wanted[module.Name]; ok {
			filtered = append(filtered, module)
			delete(wanted, module.Name)
		}
	}
	// Reporting modules not found
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for moduleName := range wanted {
			missing = append(missing, moduleName)
		}

		return nil, fmt.Errorf("module not found: %s", strings.Join(missing, ", "))
	}

	return filtered, nil
}
