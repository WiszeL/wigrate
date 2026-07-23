package discover

import (
	"os"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindInit  Kind = "init"
	KindAlter Kind = "alter"
)

type File struct {
	Path      string
	BaseName  string
	Kind      Kind
	Direction string
}

func LatestMigrationFile(module Module, entries []os.DirEntry, entityName string) *File {
	var latest *File
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		file, ok := ParseMigrationFile(filepath.Join(module.MigrationDir, entry.Name()), entityName)
		if !ok {
			continue
		}

		if latest == nil || file.BaseName > latest.BaseName {
			latest = file
			continue
		}

		if file.BaseName == latest.BaseName && file.Direction == "up" {
			latest = file
		}
	}
	return latest
}

func ParseMigrationFile(path string, entityName string) (*File, bool) {
	fileName := filepath.Base(path)

	var direction string
	var baseName string
	switch {
	case strings.HasSuffix(fileName, ".up.sql"):
		direction = "up"
		baseName = strings.TrimSuffix(fileName, ".up.sql")
	case strings.HasSuffix(fileName, ".down.sql"):
		direction = "down"
		baseName = strings.TrimSuffix(fileName, ".down.sql")
	default:
		return nil, false
	}

	// Splitting sequence prefix from descriptor
	parts := strings.SplitN(baseName, "_", 2)
	if len(parts) != 2 {
		return nil, false
	}

	descriptor := parts[1]
	if !strings.HasSuffix(descriptor, "_"+entityName) {
		return nil, false
	}

	// Classifying migration kind
	switch {
	case descriptor == "init_"+entityName:
		return &File{
			Path:      path,
			BaseName:  baseName,
			Kind:      KindInit,
			Direction: direction,
		}, true
	case strings.HasPrefix(descriptor, "alter_"):
		return &File{
			Path:      path,
			BaseName:  baseName,
			Kind:      KindAlter,
			Direction: direction,
		}, true
	default:
		return nil, false
	}
}

func MigrationFilePair(file File) (string, string) {
	dir := filepath.Dir(file.Path)

	return filepath.Join(dir, file.BaseName+".up.sql"), filepath.Join(dir, file.BaseName+".down.sql")
}
