package database

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// loadEnvFile reads KEY=VALUE pairs from a .env file into the environment.
func loadEnvFile(path string) error {
	// Open the file, treating a missing .env as a no-op
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	// Parse each KEY=VALUE line, skipping blanks/comments
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = unquote(val)

		// Real env wins over .env
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}

	return scanner.Err()
}

// unquote removes quotes and handles escaped characters like newlines and quotes.
func unquote(s string) string {
	// Check if value is long enough to have quotes
	if len(s) < 2 {
		return s
	}

	// Handle single quotes (no escape expansion)
	if s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}

	// Handle double quotes (with escape expansion)
	if s[0] == '"' && s[len(s)-1] == '"' {
		if v, err := strconv.Unquote(s); err == nil {
			return v
		}

		return s[1 : len(s)-1]
	}

	return s
}
