// fix-enum-fields replaces plain "string" struct fields with their generated
// enum types in an oapi-codegen output file.
//
// It looks for type declarations like `type XxxYyy string` preceded by a
// comment `#/components/schemas/Xxx/properties/yyy` and patches the
// corresponding struct field from `string` to the enum type.
//
// Usage: go run ./internal/tools/fix-enum-fields <file.go>
package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"regexp"
	"strings"
)

// schemaPropertyRe matches comments like:
//
//	#/components/schemas/CheckResponse/properties/reason
var schemaPropertyRe = regexp.MustCompile(`#/components/schemas/(\w+)/properties/(\w+)`)

type enumInfo struct {
	structName string
	fieldName  string // snake_case from the schema
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: fix-enum-fields <file.go>")
	}
	filename := os.Args[1]

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("parse: %v", err)
	}

	// Phase 1: find enum types derived from schema properties.
	// We look for `type XxxYyy string` where the preceding comment contains
	// `#/components/schemas/Xxx/properties/yyy`.
	// Key: enumInfo{structName, fieldName(snake_case)} -> Go enum type name
	enumTypes := map[enumInfo]string{}

	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE || len(gd.Specs) != 1 {
			continue
		}
		ts, ok := gd.Specs[0].(*ast.TypeSpec)
		if !ok {
			continue
		}
		// Must be `type Foo string`
		ident, ok := ts.Type.(*ast.Ident)
		if !ok || ident.Name != "string" {
			continue
		}
		// Check doc comments for schema property pattern
		if gd.Doc == nil {
			continue
		}
		for _, c := range gd.Doc.List {
			m := schemaPropertyRe.FindStringSubmatch(c.Text)
			if m == nil {
				continue
			}
			enumTypes[enumInfo{structName: m[1], fieldName: m[2]}] = ts.Name.Name
		}
	}

	if len(enumTypes) == 0 {
		return
	}

	// Phase 2: patch struct fields.
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE || len(gd.Specs) != 1 {
			continue
		}
		ts, ok := gd.Specs[0].(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			continue
		}
		structName := ts.Name.Name

		for _, field := range st.Fields.List {
			if len(field.Names) != 1 {
				continue
			}
			// Field must currently be `string`
			ident, ok := field.Type.(*ast.Ident)
			if !ok || ident.Name != "string" {
				continue
			}

			// Derive the snake_case field name from the json tag
			snakeName := jsonFieldName(field)
			if snakeName == "" {
				continue
			}

			if enumType, ok := enumTypes[enumInfo{structName: structName, fieldName: snakeName}]; ok {
				field.Type = ast.NewIdent(enumType)
			}
		}
	}

	out, err := os.Create(filename)
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	defer out.Close()

	cfg := printer.Config{Mode: printer.TabIndent | printer.UseSpaces, Tabwidth: 8}
	if err := cfg.Fprint(out, fset, file); err != nil {
		log.Fatalf("print: %v", err)
	}
}

// jsonFieldName extracts the field name from a struct field's `json` tag.
func jsonFieldName(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	tag := field.Tag.Value
	// Strip backticks
	tag = strings.Trim(tag, "`")

	// Find json:"..."
	const prefix = `json:"`
	idx := strings.Index(tag, prefix)
	if idx < 0 {
		return ""
	}
	rest := tag[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	jsonVal := rest[:end]

	// Take just the name part (before comma)
	if comma := strings.Index(jsonVal, ","); comma >= 0 {
		jsonVal = jsonVal[:comma]
	}
	return jsonVal
}
