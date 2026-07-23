package schema

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// enumDef describes a locally-defined Go enum type: a named string/int type
// with an associated const block, found while scanning the entity directory.
type enumDef struct {
	underlying string   // "string", "int", "int32", "int64"
	values     []string // canonical rendered tokens (quoted for string), sorted, comma-joinable for CHECK IN (...)
	maxLen     int      // longest raw label length (string enums only, for VARCHAR sizing)
}

// isEnumUnderlyingType reports whether a named type's underlying builtin can carry an enum.
func isEnumUnderlyingType(name string) bool {
	switch name {
	case "string", "int", "int32", "int64":
		return true
	default:
		return false
	}
}

// scanEnumDefs walks every .go file in the entity directory collecting named
// string/int types that have an associated const block, so struct fields using
// them can be recognized as enums.
func scanEnumDefs(entityDir string) (map[string]enumDef, error) {
	fileSet := token.NewFileSet()
	// ParseDir is deprecated only over build-tag precision, irrelevant for an
	// entity dir; the suggested replacement (x/tools/go/packages) would break
	// this project's zero-runtime-dependency policy.
	packages, err := parser.ParseDir(fileSet, entityDir, nil, 0)
	if err != nil {
		return nil, err
	}

	// First pass: collect named types whose underlying type is a builtin string/int.
	underlyings := make(map[string]string)
	for _, pkg := range packages {
		for _, file := range pkg.Files {
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
					ident, ok := typeSpec.Type.(*ast.Ident)
					if !ok || !isEnumUnderlyingType(ident.Name) {
						continue
					}
					underlyings[typeSpec.Name.Name] = ident.Name
				}
			}
		}
	}

	// Second pass: collect const values for those named types.
	rawValues := make(map[string][]string)
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.CONST {
					continue
				}
				if err := collectEnumConstValues(genDecl, underlyings, rawValues); err != nil {
					return nil, err
				}
			}
		}
	}

	enums := make(map[string]enumDef, len(rawValues))
	for typeName, raw := range rawValues {
		values, maxLen := canonicalizeEnumValues(underlyings[typeName], raw)
		enums[typeName] = enumDef{underlying: underlyings[typeName], values: values, maxLen: maxLen}
	}
	return enums, nil
}

// collectEnumConstValues walks one const( ... ) block, resolving iota and the
// Go rule that an omitted type/value list repeats the previous non-empty one.
func collectEnumConstValues(genDecl *ast.GenDecl, underlyings map[string]string, out map[string][]string) error {
	var currentType *ast.Ident
	var currentValues []ast.Expr

	for i, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		if typeIdent, ok := valueSpec.Type.(*ast.Ident); ok {
			currentType = typeIdent
		}
		if len(valueSpec.Values) > 0 {
			currentValues = valueSpec.Values
		}

		if currentType == nil {
			continue
		}
		if _, isEnum := underlyings[currentType.Name]; !isEnum {
			continue
		}

		for j, name := range valueSpec.Names {
			if name.Name == "_" || j >= len(currentValues) {
				continue
			}
			value, err := evalEnumConstExpr(currentValues[j], i)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", currentType.Name, name.Name, err)
			}
			out[currentType.Name] = append(out[currentType.Name], value)
		}
	}
	return nil
}

// evalEnumConstExpr evaluates one const value expression. Only bare iota,
// int literals, and string literals are supported (hand-rolled, no go/types) —
// anything else (e.g. 1<<iota) is a hard error.
func evalEnumConstExpr(expr ast.Expr, iotaValue int) (string, error) {
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == "iota" {
			return strconv.Itoa(iotaValue), nil
		}
		return "", fmt.Errorf("unsupported enum const expression %q", e.Name)
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			s, err := strconv.Unquote(e.Value)
			if err != nil {
				return "", fmt.Errorf("invalid string const %s: %w", e.Value, err)
			}
			return s, nil
		case token.INT:
			return e.Value, nil
		}
	}
	return "", fmt.Errorf("unsupported enum const expression")
}

// canonicalizeEnumValues dedupes and sorts raw const values into a stable
// rendering, so reordering consts in Go source never changes generated SQL.
func canonicalizeEnumValues(underlying string, raw []string) ([]string, int) {
	if underlying == "string" {
		sorted := slices.Clone(raw)
		slices.Sort(sorted)
		uniq := slices.Compact(sorted)
		maxLen := 0
		rendered := make([]string, len(uniq))
		for i, v := range uniq {
			if len(v) > maxLen {
				maxLen = len(v)
			}
			rendered[i] = quoteSQLString(v)
		}
		return rendered, maxLen
	}

	nums := make([]int, 0, len(raw))
	for _, r := range raw {
		if n, err := strconv.Atoi(r); err == nil {
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	nums = slices.Compact(nums)
	rendered := make([]string, len(nums))
	for i, n := range nums {
		rendered[i] = strconv.Itoa(n)
	}
	return rendered, 0
}

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// enumColumnType maps an enumDef to its SQL column type: sized VARCHAR for
// string enums, the normal int mapping for numeric enums.
func enumColumnType(def enumDef) string {
	switch def.underlying {
	case "string":
		return fmt.Sprintf("VARCHAR(%d)", def.maxLen)
	case "int64":
		return "BIGINT"
	default: // "int", "int32"
		return "INTEGER"
	}
}

// enumTypeName returns the bare identifier name of a field's type expression,
// unwrapping a pointer, for enum lookup. Selector types (pkg.Name) are never
// local enums so they're left unmatched.
func enumTypeName(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return enumTypeName(e.X)
	case *ast.Ident:
		return e.Name, true
	default:
		return "", false
	}
}
