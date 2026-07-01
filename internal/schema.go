package internal

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type tableSchema struct {
	name        string
	columns     []columnSchema
	foreignKeys []foreignKeySchema
}

type columnSchema struct {
	name     string
	dataType string
	notNull  bool
	primary  bool
	unique   bool
}

type foreignKeySchema struct {
	column    string
	refTable  string
	refColumn string
	onDelete  string
}

type fieldComment struct {
	length     int
	nullable   bool
	unique     bool
	primary    bool
	refTable   string
	deleteRule string
}

func parseEntitySchema(module migrationModule, entityName string) (tableSchema, error) {
	// Entity files are the source of truth for desired schema.
	entityPath := filepath.Join(module.entityDir, entityName+".go")

	// Parsing the entity file
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, entityPath, nil, parser.ParseComments)
	if err != nil {
		return tableSchema{}, err
	}

	// Finding the target struct
	structType, structName := findStruct(file, entityName)
	if structType == nil {
		return tableSchema{}, fmt.Errorf("entity struct for %s not found in %s", entityName, entityPath)
	}

	schema := tableSchema{name: tableNameFromEntity(entityName)}

	// Mapping fields to table schema
	for _, field := range structType.Fields.List {
		columns, foreignKeys, err := mapStructFieldToSchema(structName, field)
		if err != nil {
			return tableSchema{}, err
		}
		schema.columns = append(schema.columns, columns...)
		schema.foreignKeys = append(schema.foreignKeys, foreignKeys...)
	}

	if len(schema.columns) == 0 {
		return tableSchema{}, fmt.Errorf("entity struct %s has no exported fields", structName)
	}

	return schema, nil
}

func mapStructFieldToSchema(structName string, field *ast.Field) ([]columnSchema, []foreignKeySchema, error) {
	if len(field.Names) == 0 {
		return nil, nil, fmt.Errorf("%s: embedded fields are not supported", structName)
	}

	// Parsing the field comment
	comment, err := parseFieldComment(field)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", structName, err)
	}

	// Mapping exported fields to columns and FKs
	var columns []columnSchema
	var foreignKeys []foreignKeySchema
	for _, name := range field.Names {
		if !name.IsExported() {
			continue
		}

		column, err := mapFieldToColumn(name.Name, field.Type, comment)
		if err != nil {
			return nil, nil, fmt.Errorf("%s.%s: %w", structName, name.Name, err)
		}
		columns = append(columns, column)

		foreignKey, ok, err := mapFieldToForeignKey(name.Name, column, comment)
		if err != nil {
			return nil, nil, fmt.Errorf("%s.%s: %w", structName, name.Name, err)
		}
		if ok {
			foreignKeys = append(foreignKeys, foreignKey)
		}
	}

	return columns, foreignKeys, nil
}

// findStruct locates a struct whose snake_case name matches entityName.
// Returns the struct type and the actual declared name (for error messages).
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

	// Iterating over comment tokens
	for token := range strings.FieldsSeq(field.Comment.Text()) {
		// Inline comments form a small schema DSL, e.g. `20 null unique`.
		switch {
		case token == "null":
			comment.nullable = true
		case token == "unique":
			comment.unique = true
		case token == "pk":
			comment.primary = true
		case strings.HasPrefix(token, "ref:"):
			comment.refTable = strings.TrimPrefix(token, "ref:")
			if comment.refTable == "" {
				return fieldComment{}, fmt.Errorf("empty ref table")
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

func mapFieldToColumn(fieldName string, expr ast.Expr, comment fieldComment) (columnSchema, error) {
	// Mapping Go type to SQL type
	dataType, err := goTypeToSQLType(expr, comment)
	if err != nil {
		return columnSchema{}, err
	}

	_, isPointer := expr.(*ast.StarExpr)

	// Setting column constraints
	column := columnSchema{
		name:     snakeCase(fieldName),
		dataType: dataType,
		notNull:  !comment.nullable && !isPointer,
		primary:  fieldName == "ID" || comment.primary,
		unique:   comment.unique,
	}
	if column.primary {
		column.notNull = false
		column.unique = false
	}

	return column, nil
}

func mapFieldToForeignKey(fieldName string, column columnSchema, comment fieldComment) (foreignKeySchema, bool, error) {
	// Checking FK naming convention
	isFK := fieldName != "ID" && strings.HasSuffix(fieldName, "ID") && !strings.HasSuffix(fieldName, "UUID")
	if !isFK {
		if comment.refTable != "" || comment.deleteRule != "" {
			return foreignKeySchema{}, false, fmt.Errorf("ref:/del: annotations require a field name ending in ID (not UUID)")
		}
		return foreignKeySchema{}, false, nil
	}

	// Validating delete rule against nullability
	if comment.deleteRule == "SET NULL" && column.notNull {
		return foreignKeySchema{}, false, fmt.Errorf("del:setnull requires null")
	}

	// Resolving the referenced table
	refEntity := strings.TrimSuffix(fieldName, "ID")
	refTable := tableNameFromEntity(snakeCase(refEntity))
	if comment.refTable != "" {
		refTable = comment.refTable
	}

	return foreignKeySchema{
		column:    column.name,
		refTable:  refTable,
		refColumn: "id",
		onDelete:  comment.deleteRule,
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

func tableNameFromEntity(entityName string) string {
	return pluralizeSnakeCase(snakeCase(entityName))
}

func pascalCase(value string) string {
	var builder strings.Builder
	parts := strings.SplitSeq(value, "_")
	for part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		builder.WriteRune(unicode.ToUpper(runes[0]))
		for _, r := range runes[1:] {
			builder.WriteRune(r)
		}
	}

	return builder.String()
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
