//go:build unix || windows || plan9

package build

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// procTagPlatforms are the build-tag sets checkProcTestFile evaluates a
// file's //go:build line against.
var procTagPlatforms = map[string]map[string]bool{
	"linux":   {"linux": true, "unix": true, "amd64": true},
	"windows": {"windows": true, "amd64": true},
	"plan9":   {"plan9": true, "amd64": true},
	"js":      {"js": true, "wasm": true},
}

// shCallees are calls that make a test need POSIX tools; skipCallees
// are the helpers that skip such a test on plan9.
var (
	shCallees   = map[string]bool{"writeScript": true, "recipeCmd": true}
	skipCallees = map[string]bool{"skipOnPlan9": true, "skipWithoutPOSIXTools": true}
)

// checkProcTestFile reports why a test file breaks plan 2610030243's
// contract, or nil. A file built on unix and windows but not js must
// also build on plan9, so GOOS=plan9 go vet type-checks it. In such a
// file, every Test that runs an sh script, an `sh` argv, or a recipe
// must call a plan9 skip helper, since plan9 has only rc.
func checkProcTestFile(name string, src []byte) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", name, err)}
	}
	expr := buildConstraint(src)
	ok := func(platform string) bool {
		if expr == nil {
			return true
		}
		return expr.Eval(func(tag string) bool { return procTagPlatforms[platform][tag] })
	}
	if !ok("linux") || !ok("windows") || ok("js") {
		return nil
	}
	if !ok("plan9") {
		return []string{fmt.Sprintf(
			"%s: build tag %q drops plan9; use unix || windows || plan9",
			name, expr.String())}
	}
	var errs []string
	for _, decl := range f.Decls {
		fn, isFn := decl.(*ast.FuncDecl)
		if !isFn || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		if needsSh, skipped := scanTestBody(fn.Body); needsSh && !skipped {
			errs = append(errs, fmt.Sprintf(
				"%s: %s needs sh but never calls skipOnPlan9 or skipWithoutPOSIXTools",
				name, fn.Name.Name))
		}
	}
	return errs
}

// buildConstraint returns the file's //go:build expression, or nil.
func buildConstraint(src []byte) constraint.Expr {
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "package ") {
			return nil
		}
		if constraint.IsGoBuild(line) {
			expr, err := constraint.Parse(line)
			if err == nil {
				return expr
			}
		}
	}
	return nil
}

// scanTestBody reports whether body needs sh and whether it calls a
// plan9 skip helper.
func scanTestBody(body *ast.BlockStmt) (needsSh, skipped bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if id, isIdent := n.Fun.(*ast.Ident); isIdent {
				needsSh = needsSh || shCallees[id.Name]
				skipped = skipped || skipCallees[id.Name]
			}
		case *ast.BasicLit:
			if s, err := strconv.Unquote(n.Value); n.Kind == token.STRING && err == nil && s == "sh" {
				needsSh = true
			}
		}
		return true
	})
	return needsSh, skipped
}

// checkProcTestDir runs checkProcTestFile over every _test.go in dir.
func checkProcTestDir(t *testing.T, dir string) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	require.NoError(t, err)
	require.NotEmpty(t, names)
	sort.Strings(names)
	errs := make([]string, 0, len(names))
	for _, name := range names {
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		errs = append(errs, checkProcTestFile(filepath.Base(name), src)...)
	}
	return errs
}
