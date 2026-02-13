// strip-apply-defaults removes all ApplyDefaults methods from a Go source file.
// Usage: go run ./internal/tools/strip-apply-defaults <file.go>
package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: strip-apply-defaults <file.go>")
	}
	filename := os.Args[1]

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("parse: %v", err)
	}

	// Collect positions of doc comments to remove.
	removeComments := map[*ast.CommentGroup]bool{}

	var filtered []ast.Decl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv != nil && fn.Name.Name == "ApplyDefaults" {
			if fn.Doc != nil {
				removeComments[fn.Doc] = true
			}
			continue
		}
		filtered = append(filtered, decl)
	}
	file.Decls = filtered

	var comments []*ast.CommentGroup
	for _, cg := range file.Comments {
		if !removeComments[cg] {
			comments = append(comments, cg)
		}
	}
	file.Comments = comments

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
