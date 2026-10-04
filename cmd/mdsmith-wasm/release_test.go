//go:build !js

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEngineReleasesNoFunc scans the engine's non-test Go files for any
// use of a Release selector (js.Func.Release, f.Release()). The engine
// registers only load-time funcs (the API, the shared methods, and the
// shared Promise executor) and must release none: every Promise is built
// with the shared executor bound per call, and every session method is a
// shared func bound to the session id, so releasing one would break
// every later call. The scan keeps that invariant pinned without a
// release seam that production code never calls. It runs natively,
// where the package's files are readable.
func TestEngineReleasesNoFunc(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Release" {
				t.Errorf("%s: the engine releases a func", fset.Position(sel.Pos()))
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("no engine source file was scanned")
	}
}
