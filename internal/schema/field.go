package schema

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

type fieldComment struct {
	length      int
	nullable    bool
	unique      bool
	primary     bool
	refTable    string
	deleteRule  string
	uniqueGroup string
	index       bool
	indexGroup  string
	trgm        bool
}

// fieldIndex holds the index directive for one column (no index if on=false, bare if group=="").
type fieldIndex struct {
	on    bool
	group string
	trgm  bool
}

// mapStructFieldsToSchema walks every field of structType, mapping each to a
// column (or, for a same-dir value-object struct field, recursively
// flattening it into columns prefixed by the field path — e.g. Cust.Name ->
// cust_name, Buyer.Address.City -> buyer_address_city). Nested VO fields keep
// full DSL semantics (FK, enum, unique/index groups) as if inlined into the
// parent, just prefixed and, for unique/index groups, scoped to the prefix so
// two VO instances never merge into one constraint.
//
// allowPrimaryKey is true only for the top-level entity struct; recursing
// into a value object flips it false — a VO owns no identity, so a bare ID
// field or pk annotation found there is an error, not a nonsensical
// PRIMARY KEY on the parent table. visited tracks the chain of value-object
// type names currently being expanded, to reject a cycle.
func mapStructFieldsToSchema(structType *ast.StructType, typeName, originPath, columnPrefix string, allowPrimaryKey bool, structs map[string]*ast.StructType, enums map[string]enumDef, visited []string) ([]Column, []ForeignKey, []string, []fieldIndex, []string, error) {
	var columns []Column
	var foreignKeys []ForeignKey
	var groups []string
	var indexes []fieldIndex
	var origins []string

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			return nil, nil, nil, nil, nil, fmt.Errorf("%s: embedded fields are not supported", typeName)
		}

		comment, err := parseFieldComment(field)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("%s: %w", typeName, err)
		}

		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			if !allowPrimaryKey && (name.Name == "ID" || comment.primary) {
				return nil, nil, nil, nil, nil, fmt.Errorf("%s.%s: a value object cannot declare a primary key", typeName, name.Name)
			}

			origin := originPath + "." + name.Name

			// A same-dir named struct type flattens into prefixed columns
			// instead of mapping to a single one.
			if voStruct, voTypeName, isVO := resolveValueObjectType(field.Type, structs); isVO {
				if comment != (fieldComment{}) {
					return nil, nil, nil, nil, nil, fmt.Errorf("%s.%s: DSL annotations are not allowed on a value-object field", typeName, name.Name)
				}
				if slices.Contains(visited, voTypeName) {
					return nil, nil, nil, nil, nil, fmt.Errorf("cyclic value-object reference: %s -> %s", strings.Join(visited, " -> "), voTypeName)
				}
				childPrefix := prefixed(columnPrefix, "_", snakeCase(name.Name))
				cols, fks, grps, idxs, forigins, err := mapStructFieldsToSchema(voStruct, voTypeName, origin, childPrefix, false, structs, enums, append(slices.Clone(visited), voTypeName))
				if err != nil {
					return nil, nil, nil, nil, nil, err
				}
				columns = append(columns, cols...)
				foreignKeys = append(foreignKeys, fks...)
				groups = append(groups, grps...)
				indexes = append(indexes, idxs...)
				origins = append(origins, forigins...)
				continue
			}

			column, err := mapFieldToColumn(name.Name, field.Type, comment, enums)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("%s.%s: %w", typeName, name.Name, err)
			}
			column.Name = prefixed(columnPrefix, "_", column.Name)
			columns = append(columns, column)

			// Unique/index group labels are scoped to the value-object's
			// column prefix so the same label in two different VO instances
			// (or at the parent level) never silently merges.
			group := prefixed(columnPrefix, ".", comment.uniqueGroup)
			groups = append(groups, group)

			indexGroup := prefixed(columnPrefix, ".", comment.indexGroup)
			indexes = append(indexes, fieldIndex{on: comment.index || comment.indexGroup != "", group: indexGroup, trgm: comment.trgm})
			origins = append(origins, origin)

			foreignKey, ok, err := mapFieldToForeignKey(name.Name, column, comment)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("%s.%s: %w", typeName, name.Name, err)
			}
			if ok {
				foreignKeys = append(foreignKeys, foreignKey)
			}
		}
	}

	return columns, foreignKeys, groups, indexes, origins, nil
}

// prefixed joins prefix and s with sep, unless either is empty (in which case
// s is returned unchanged).
func prefixed(prefix, sep, s string) string {
	if prefix == "" || s == "" {
		return s
	}
	return prefix + sep + s
}

// resolveValueObjectType reports whether expr is a bare (non-pointer) local
// named struct type — a value object eligible for flattening. Pointers and
// package selectors deliberately don't match: they fall through to the normal
// scalar path and error as "unsupported field type", matching FK/enum's
// same-dir-only resolution scope.
func resolveValueObjectType(expr ast.Expr, structs map[string]*ast.StructType) (*ast.StructType, string, bool) {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return nil, "", false
	}
	structType, ok := structs[ident.Name]
	if !ok {
		return nil, "", false
	}
	return structType, ident.Name, true
}

// findStruct finds a struct whose snake_case name matches entityName.
func findStruct(file *ast.File, entityName string) (*ast.StructType, string) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if snakeCase(typeSpec.Name.Name) != entityName {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if ok {
				return structType, typeSpec.Name.Name
			}
		}
	}

	return nil, ""
}

func parseFieldComment(field *ast.Field) (fieldComment, error) {
	if field.Comment == nil {
		return fieldComment{}, nil
	}

	comment := fieldComment{}

	// Parsing comment tokens (DSL: "20 null unique ref:table unique:group index:group del:cascade trgm")
	for token := range strings.FieldsSeq(field.Comment.Text()) {
		switch {
		case token == "null":
			comment.nullable = true
		case token == "unique":
			comment.unique = true
		case token == "pk":
			comment.primary = true
		case token == "index":
			comment.index = true
		case token == "trgm":
			comment.trgm = true
		case strings.HasPrefix(token, "ref:"):
			comment.refTable = strings.TrimPrefix(token, "ref:")
			if comment.refTable == "" {
				return fieldComment{}, fmt.Errorf("empty ref table")
			}
		case strings.HasPrefix(token, "unique:"):
			comment.uniqueGroup = strings.TrimPrefix(token, "unique:")
			if comment.uniqueGroup == "" {
				return fieldComment{}, fmt.Errorf("empty unique group")
			}
		case strings.HasPrefix(token, "index:"):
			comment.indexGroup = strings.TrimPrefix(token, "index:")
			if comment.indexGroup == "" {
				return fieldComment{}, fmt.Errorf("empty index group")
			}
		case strings.HasPrefix(token, "del:"):
			rule, err := normalizeDeleteRule(strings.TrimPrefix(token, "del:"))
			if err != nil {
				return fieldComment{}, err
			}
			comment.deleteRule = rule
		default:
			length, err := strconv.Atoi(token)
			if err != nil {
				return fieldComment{}, fmt.Errorf("unknown comment token %q", token)
			}
			if length <= 0 {
				return fieldComment{}, fmt.Errorf("varchar length must be greater than zero")
			}
			comment.length = length
		}
	}

	return comment, nil
}

func normalizeDeleteRule(token string) (string, error) {
	switch token {
	case "cascade":
		return "CASCADE", nil
	case "setnull":
		return "SET NULL", nil
	case "restrict":
		return "RESTRICT", nil
	case "noaction":
		return "NO ACTION", nil
	default:
		return "", fmt.Errorf("unsupported delete rule %q", token)
	}
}

func mapFieldToColumn(fieldName string, expr ast.Expr, comment fieldComment, enums map[string]enumDef) (Column, error) {
	// Converting type to SQL — enum types (local named string/int with consts)
	// bypass the normal builtin mapping and carry a CHECK constraint instead
	var dataType, check string
	if typeName, ok := enumTypeName(expr); ok {
		if def, isEnum := enums[typeName]; isEnum {
			dataType = enumColumnType(def)
			check = strings.Join(def.values, ",")
		}
	}
	if dataType == "" {
		var err error
		dataType, err = goTypeToSQLType(expr, comment)
		if err != nil {
			return Column{}, err
		}
	}

	// Building column
	_, isPointer := expr.(*ast.StarExpr)

	column := Column{
		Name:     snakeCase(fieldName),
		DataType: dataType,
		NotNull:  !comment.nullable && !isPointer,
		Primary:  fieldName == "ID" || comment.primary,
		Unique:   comment.unique,
		Check:    check,
	}
	if column.Primary {
		column.NotNull = false
		column.Unique = false
	}
	if comment.trgm && !isStringSQLType(column.DataType) {
		return Column{}, fmt.Errorf("trgm requires a string field")
	}

	return column, nil
}

func isStringSQLType(dataType string) bool {
	return dataType == "TEXT" || strings.HasPrefix(dataType, "VARCHAR")
}

func mapFieldToForeignKey(fieldName string, column Column, comment fieldComment) (ForeignKey, bool, error) {
	// Checking field name ends with ID
	isFK := fieldName != "ID" && strings.HasSuffix(fieldName, "ID") && !strings.HasSuffix(fieldName, "UUID")
	if !isFK {
		if comment.refTable != "" || comment.deleteRule != "" {
			return ForeignKey{}, false, fmt.Errorf("ref:/del: annotations require a field name ending in ID (not UUID)")
		}
		return ForeignKey{}, false, nil
	}

	// Validating delete rule
	if comment.deleteRule == "SET NULL" && column.NotNull {
		return ForeignKey{}, false, fmt.Errorf("del:setnull requires null")
	}

	// Finding referenced table
	refEntity := strings.TrimSuffix(fieldName, "ID")
	refTable := TableNameFromEntity(snakeCase(refEntity))
	if comment.refTable != "" {
		refTable = comment.refTable
	}

	return ForeignKey{
		Column:    column.Name,
		RefTable:  refTable,
		RefColumn: "id",
		OnDelete:  comment.deleteRule,
	}, true, nil
}

func goTypeToSQLType(expr ast.Expr, comment fieldComment) (string, error) {
	switch value := expr.(type) {
	case *ast.StarExpr:
		return goTypeToSQLType(value.X, comment)
	case *ast.Ident:
		return identToSQLType(value.Name, comment)
	case *ast.SelectorExpr:
		pkg, ok := value.X.(*ast.Ident)
		if !ok {
			return "", fmt.Errorf("unsupported selector type")
		}
		return identToSQLType(pkg.Name+"."+value.Sel.Name, comment)
	default:
		return "", fmt.Errorf("unsupported field type")
	}
}

func identToSQLType(typeName string, comment fieldComment) (string, error) {
	switch typeName {
	case "string":
		if comment.length > 0 {
			return fmt.Sprintf("VARCHAR(%d)", comment.length), nil
		}
		return "TEXT", nil
	case "int", "int32":
		return "INTEGER", nil
	case "int64":
		return "BIGINT", nil
	case "bool":
		return "BOOLEAN", nil
	case "float32":
		return "REAL", nil
	case "float64":
		return "DOUBLE PRECISION", nil
	case "time.Time":
		return "TIMESTAMPTZ", nil
	case "uuid.UUID":
		return "UUID", nil
	default:
		return "", fmt.Errorf("unsupported field type %s", typeName)
	}
}
