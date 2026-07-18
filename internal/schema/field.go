package schema

import (
	"fmt"
	"go/ast"
	"go/token"
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

func mapStructFieldToSchema(structName string, field *ast.Field) ([]Column, []ForeignKey, []string, []fieldIndex, error) {
	if len(field.Names) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("%s: embedded fields are not supported", structName)
	}

	// Parsing field comment
	comment, err := parseFieldComment(field)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%s: %w", structName, err)
	}

	// Mapping exported fields
	var columns []Column
	var foreignKeys []ForeignKey
	var groups []string
	var indexes []fieldIndex
	for _, name := range field.Names {
		if !name.IsExported() {
			continue
		}

		column, err := mapFieldToColumn(name.Name, field.Type, comment)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%s.%s: %w", structName, name.Name, err)
		}
		columns = append(columns, column)
		groups = append(groups, comment.uniqueGroup)
		indexes = append(indexes, fieldIndex{on: comment.index || comment.indexGroup != "", group: comment.indexGroup, trgm: comment.trgm})

		foreignKey, ok, err := mapFieldToForeignKey(name.Name, column, comment)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%s.%s: %w", structName, name.Name, err)
		}
		if ok {
			foreignKeys = append(foreignKeys, foreignKey)
		}
	}

	return columns, foreignKeys, groups, indexes, nil
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

func mapFieldToColumn(fieldName string, expr ast.Expr, comment fieldComment) (Column, error) {
	// Converting type to SQL
	dataType, err := goTypeToSQLType(expr, comment)
	if err != nil {
		return Column{}, err
	}

	// Building column
	_, isPointer := expr.(*ast.StarExpr)

	column := Column{
		Name:     snakeCase(fieldName),
		DataType: dataType,
		NotNull:  !comment.nullable && !isPointer,
		Primary:  fieldName == "ID" || comment.primary,
		Unique:   comment.unique,
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
