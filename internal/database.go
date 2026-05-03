package internal

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type envLookupFunc func(string) (string, bool)

type databaseConfig struct {
	host     string
	port     string
	name     string
	user     string
	password string
	sslMode  string
}

func loadDatabaseConfig(root string) (databaseConfig, error) {
	dotEnv, err := readDotEnv(filepath.Join(root, ".env"))
	if err != nil {
		return databaseConfig{}, err
	}

	return databaseConfigFromEnv(os.LookupEnv, dotEnv)
}

func databaseConfigFromEnv(lookup envLookupFunc, dotEnv map[string]string) (databaseConfig, error) {
	config := databaseConfig{
		host:     envValue("DB_HOST", lookup, dotEnv),
		port:     envValue("DB_PORT", lookup, dotEnv),
		name:     envValue("DB_NAME", lookup, dotEnv),
		user:     envValue("DB_USER", lookup, dotEnv),
		password: envValue("DB_PASSWORD", lookup, dotEnv),
		sslMode:  envValue("DB_SSLMODE", lookup, dotEnv),
	}
	if config.sslMode == "" {
		config.sslMode = "disable"
	}

	for _, required := range []struct {
		key   string
		value string
	}{
		{key: "DB_HOST", value: config.host},
		{key: "DB_PORT", value: config.port},
		{key: "DB_NAME", value: config.name},
		{key: "DB_USER", value: config.user},
		{key: "DB_PASSWORD", value: config.password},
	} {
		if strings.TrimSpace(required.value) == "" {
			return databaseConfig{}, fmt.Errorf("%s is required", required.key)
		}
	}

	return config, nil
}

func envValue(key string, lookup envLookupFunc, dotEnv map[string]string) string {
	if value, ok := lookup(key); ok {
		return value
	}
	return dotEnv[key]
}

func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseDotEnvLine(scanner.Text())
		if ok {
			values[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return values, nil
}

func parseDotEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")

	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}

	return key, trimDotEnvValue(value), true
}

func trimDotEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return value
	}

	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}

	return value
}

func (config databaseConfig) urlForModule(module migrationModule) string {
	postgresURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.user, config.password),
		Host:   net.JoinHostPort(config.host, config.port),
		Path:   "/" + config.name,
	}

	query := postgresURL.Query()
	query.Set("sslmode", config.sslMode)
	query.Set("x-migrations-table", migrationTableName(module.name))
	postgresURL.RawQuery = query.Encode()

	return postgresURL.String()
}

func migrationTableName(moduleName string) string {
	return "schema_migrations_" + moduleName
}
