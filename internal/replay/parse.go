package replay

import (
	"strings"

	"github.com/wiszel/wigrate/internal/schema"
)

func parseGeneratedColumn(line string) (schema.Column, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return schema.Column{}, false
	}
	if parts[0] == "FOREIGN" || parts[0] == "CONSTRAINT" {
		return schema.Column{}, false
	}

	// Extracting column name
	column := schema.Column{Name: parts[0]}

	// Extracting data type (may contain spaces like DOUBLE PRECISION)
	var dataTypeParts []string
	for i := 1; i < len(parts); i++ {
		if parts[i] == "PRIMARY" || parts[i] == "NOT" || parts[i] == "UNIQUE" {
			break
		}
		dataTypeParts = append(dataTypeParts, parts[i])
	}
	if len(dataTypeParts) == 0 {
		return schema.Column{}, false
	}
	column.DataType = strings.Join(dataTypeParts, " ")

	// Parsing column constraints (PRIMARY, NOT NULL, UNIQUE)
	for i := 1 + len(dataTypeParts); i < len(parts); i++ {
		switch parts[i] {
		case "PRIMARY":
			column.Primary = true
		case "NOT":
			if i+1 < len(parts) && parts[i+1] == "NULL" {
				column.NotNull = true
			}
		case "UNIQUE":
			column.Unique = true
		}
	}

	return column, true
}

// Extracting column list from a UNIQUE constraint line.
func parseGeneratedUniqueConstraint(line string) ([]string, bool) {
	_, after, ok := strings.Cut(line, " UNIQUE ")
	if !ok {
		return nil, false
	}

	return parseGeneratedColumnList(after)
}

// Extracting parenthesized column list (e.g., "(a, b)").
func parseGeneratedColumnList(text string) ([]string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "(") || !strings.HasSuffix(text, ")") {
		return nil, false
	}

	var columns []string
	for part := range strings.SplitSeq(text[1:len(text)-1], ",") {
		column := strings.TrimSpace(part)
		if column == "" {
			return nil, false
		}
		columns = append(columns, column)
	}

	return columns, len(columns) > 0
}

// Extracting column list from a CREATE INDEX line.
func parseGeneratedIndex(line string) ([]string, bool) {
	parenStart := strings.Index(line, "(")
	if parenStart == -1 {
		return nil, false
	}

	return parseGeneratedColumnList(line[parenStart:])
}

// Extracting column from a GIN trigram index line.
func parseGeneratedTrgmIndex(line string) (string, bool) {
	_, after, ok := strings.Cut(line, "(")
	if !ok {
		return "", false
	}
	inner := strings.TrimSuffix(strings.TrimSpace(after), ")")
	col, _, ok := strings.Cut(inner, " ")
	if !ok || col == "" {
		return "", false
	}

	return col, true
}

// Extracting column name + canonical value list from a CHECK constraint line,
// e.g. "CONSTRAINT chk_t_c CHECK (c IN ('a','b'))" (CREATE TABLE) or
// "ADD CONSTRAINT chk_t_c CHECK (c IN ('a','b'))" (ALTER).
func parseGeneratedCheckConstraint(line string) (columnName string, checkBody string, ok bool) {
	_, after, found := strings.Cut(line, " CHECK (")
	if !found || !strings.HasSuffix(after, ")") {
		return "", "", false
	}
	inner := strings.TrimSuffix(after, ")")

	column, list, found := strings.Cut(inner, " IN (")
	if !found || !strings.HasSuffix(list, ")") {
		return "", "", false
	}
	body := strings.TrimSuffix(list, ")")
	column = strings.TrimSpace(column)
	if column == "" || body == "" {
		return "", "", false
	}

	return column, body, true
}

func parseGeneratedForeignKey(line string) (schema.ForeignKey, bool) {
	// Finding referencing column in parentheses
	columnStart := strings.Index(line, "(")
	columnEnd := strings.Index(line, ")")
	_, after, ok := strings.Cut(line, " REFERENCES ")
	if columnStart == -1 || columnEnd == -1 || !ok || columnEnd <= columnStart {
		return schema.ForeignKey{}, false
	}

	// Finding referenced table and column
	reference := after
	refTableEnd := strings.Index(reference, "(")
	refColumnEnd := strings.Index(reference, ")")
	if refTableEnd == -1 || refColumnEnd == -1 || refColumnEnd <= refTableEnd {
		return schema.ForeignKey{}, false
	}

	foreignKey := schema.ForeignKey{
		Column:    strings.TrimSpace(line[columnStart+1 : columnEnd]),
		RefTable:  strings.TrimSpace(reference[:refTableEnd]),
		RefColumn: strings.TrimSpace(reference[refTableEnd+1 : refColumnEnd]),
	}

	// Checking for ON DELETE clause
	if _, after, ok := strings.Cut(line, " ON DELETE "); ok {
		foreignKey.OnDelete = strings.TrimSpace(after)
	}

	return foreignKey, true
}
