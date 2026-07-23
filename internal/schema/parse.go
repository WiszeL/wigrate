package schema

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"

	"github.com/wiszel/wigrate/internal/discover"
)

type Table struct {
	Name        string
	Columns     []Column
	ForeignKeys []ForeignKey
	PrimaryKey  []string   // composite PK column names, in order (empty => single/no PK, stays inline column bool)
	Uniques     [][]string // composite unique groups, each >=2 cols, in order
	Indexes     [][]string // plain (non-unique) indexes, each 1+ cols, in order; always standalone CREATE INDEX
	TrgmIndexes []string   // columns with a GIN trigram index (single-column only), in order
}

type Column struct {
	Name     string
	DataType string
	NotNull  bool
	Primary  bool
	Unique   bool
	Check    string // canonical CHECK IN (...) body for enum fields; "" if not an enum
}

type ForeignKey struct {
	Column    string
	RefTable  string
	RefColumn string
	OnDelete  string
}

// IsEntityFile reports whether entityName's file declares a matching struct —
// i.e. whether it's a real entity, as opposed to a support file sitting in the
// same entity dir (e.g. an enum's `type X string` + const block, referenced by
// another entity's field but with no table of its own).
func IsEntityFile(module discover.Module, entityName string) (bool, error) {
	entityPath := filepath.Join(module.EntityDir, entityName+".go")

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, entityPath, nil, 0)
	if err != nil {
		return false, err
	}

	structType, _ := findStruct(file, entityName)
	return structType != nil, nil
}

func Parse(module discover.Module, entityName string) (Table, error) {
	// Reading entity file
	entityPath := filepath.Join(module.EntityDir, entityName+".go")

	// Parsing Go file
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, entityPath, nil, parser.ParseComments)
	if err != nil {
		return Table{}, err
	}

	// Finding struct matching entity name
	structType, structName := findStruct(file, entityName)
	if structType == nil {
		return Table{}, fmt.Errorf("entity struct for %s not found in %s", entityName, entityPath)
	}

	// Scanning sibling files in the entity dir for local enum types (named
	// string/int types with a const block)
	enums, err := scanEnumDefs(module.EntityDir)
	if err != nil {
		return Table{}, err
	}

	table := Table{Name: TableNameFromEntity(entityName)}

	// Mapping fields to table schema
	var uniqueGroups []string // parallel to table.Columns; "" = no group
	var indexDirectives []fieldIndex
	for _, field := range structType.Fields.List {
		columns, foreignKeys, groups, indexes, err := mapStructFieldToSchema(structName, field, enums)
		if err != nil {
			return Table{}, err
		}
		table.Columns = append(table.Columns, columns...)
		table.ForeignKeys = append(table.ForeignKeys, foreignKeys...)
		uniqueGroups = append(uniqueGroups, groups...)
		indexDirectives = append(indexDirectives, indexes...)
	}

	if len(table.Columns) == 0 {
		return Table{}, fmt.Errorf("entity struct %s has no exported fields", structName)
	}

	// Folding composite constraints
	foldCompositePrimaryKey(&table)
	foldCompositeUniques(&table, uniqueGroups)
	foldIndexes(&table, indexDirectives)

	return table, nil
}

// foldCompositePrimaryKey moves 2+ primary key columns to table-level PK (single stays inline).
func foldCompositePrimaryKey(table *Table) {
	// Finding marked columns
	var members []int
	for i, column := range table.Columns {
		if column.Primary {
			members = append(members, i)
		}
	}
	if len(members) < 2 {
		return
	}

	// Moving to table-level
	for _, i := range members {
		table.PrimaryKey = append(table.PrimaryKey, table.Columns[i].Name)
		table.Columns[i].Primary = false
		table.Columns[i].NotNull = true
	}
}

// foldCompositeUniques groups labeled columns into table-level UNIQUE (size 1 stays inline).
func foldCompositeUniques(table *Table, groups []string) {
	// Building group map
	order := make([]string, 0)
	indexByGroup := make(map[string][]int)
	for i, group := range groups {
		if group == "" {
			continue
		}
		if _, ok := indexByGroup[group]; !ok {
			order = append(order, group)
		}
		indexByGroup[group] = append(indexByGroup[group], i)
	}

	// Adding to table or inline
	for _, group := range order {
		members := indexByGroup[group]
		if len(members) == 1 {
			table.Columns[members[0]].Unique = true
			continue
		}

		var cols []string
		for _, i := range members {
			cols = append(cols, table.Columns[i].Name)
		}
		table.Uniques = append(table.Uniques, cols)
	}
}

// foldIndexes groups labeled columns into table-level indexes (bare labels become single-column).
func foldIndexes(table *Table, directives []fieldIndex) {
	// Collecting trigram and grouped indexes
	order := make([]string, 0)
	indexByGroup := make(map[string][]int)
	for i, directive := range directives {
		if directive.trgm {
			table.TrgmIndexes = append(table.TrgmIndexes, table.Columns[i].Name)
		}
		if !directive.on {
			continue
		}
		if directive.group == "" {
			table.Indexes = append(table.Indexes, []string{table.Columns[i].Name})
			continue
		}
		if _, ok := indexByGroup[directive.group]; !ok {
			order = append(order, directive.group)
		}
		indexByGroup[directive.group] = append(indexByGroup[directive.group], i)
	}

	// Adding grouped indexes
	for _, group := range order {
		var cols []string
		for _, i := range indexByGroup[group] {
			cols = append(cols, table.Columns[i].Name)
		}
		table.Indexes = append(table.Indexes, cols)
	}
}
