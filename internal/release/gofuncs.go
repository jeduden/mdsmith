// Package release: gofuncs.go is the one Go-source scanner the
// release tooling uses to find test functions. test-summary
// (testsummary.go) and test-js-wasm (jswasmtests.go) both read
// declarations through it, so a commented-out function, one inside a
// string literal, or a signature wrapped over several lines is seen
// the same way by both; each caller then applies its own name rule.
package release

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// topLevelFuncs parses a Go source file and returns its AST and its
// package-level function declarations (methods excluded) in source
// order. A file that does not parse is an error.
func topLevelFuncs(src []byte) (*ast.File, []*ast.FuncDecl, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	var funcs []*ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
			funcs = append(funcs, fn)
		}
	}
	return f, funcs, nil
}
