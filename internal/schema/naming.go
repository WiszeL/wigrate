package schema

import (
	"strings"
	"unicode"
)

// TableNameFromEntity derives the plural snake_case table name for an entity name.
func TableNameFromEntity(entityName string) string {
	return pluralizeSnakeCase(snakeCase(entityName))
}

func snakeCase(value string) string {
	if value == "" {
		return ""
	}

	runes := []rune(value)
	var builder strings.Builder
	for i, r := range runes {
		if r == '_' {
			builder.WriteRune(r)
			continue
		}

		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prev != '_' && (unicode.IsLower(prev) || unicode.IsDigit(prev) || nextIsLower) {
				builder.WriteRune('_')
			}
		}

		builder.WriteRune(unicode.ToLower(r))
	}

	return builder.String()
}

func pluralizeSnakeCase(value string) string {
	if value == "" {
		return value
	}

	if strings.HasSuffix(value, "y") && len(value) > 1 && !isVowel(rune(value[len(value)-2])) {
		return strings.TrimSuffix(value, "y") + "ies"
	}

	for _, suffix := range []string{"s", "x", "z", "ch", "sh"} {
		if strings.HasSuffix(value, suffix) {
			return value + "es"
		}
	}

	return value + "s"
}

func isVowel(r rune) bool {
	switch unicode.ToLower(r) {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	default:
		return false
	}
}

// SQL identifier naming: lives here (not sqlgen) so diff and replay can compute names independently.

func ForeignKeyConstraintName(tableName string, column string) string {
	return "fk_" + tableName + "_" + column
}

func UniqueConstraintName(tableName string, columns ...string) string {
	return "uq_" + tableName + "_" + strings.Join(columns, "_")
}

func IndexName(tableName string, columns []string) string {
	return "idx_" + tableName + "_" + strings.Join(columns, "_")
}

func TrgmIndexName(tableName string, column string) string {
	return "idx_" + tableName + "_" + column + "_trgm"
}
