//go:build unix || windows || plan9

package build

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// procTargets are the targets checkProcTestFiles matches each file
// against, and whether a spawn test file must build there: exactly
// where exec_other.go's !unix && !windows && !plan9 stubs do not.
// "newos" stands for a future port, which the stubs would cover.
var procTargets = []struct {
	goos, goarch, label string
	want                bool
}{
	{"linux", "amd64", "linux", true},
	{"windows", "amd64", "windows", true},
	{"plan9", "amd64", "plan9", true},
	{"js", "wasm", "js", false},
	{"wasip1", "wasm", "wasip1", false},
	{"newos", "amd64", "a future port", false},
}

// shCallees are calls that make a test need POSIX tools; skipCallees
// are the helpers that skip such a test on plan9. procTestFuncs adds
// the package's own helpers that reach either first.
var (
	shCallees   = []string{"writeScript", "recipeCmd"}
	skipCallees = []string{"skipOnPlan9", "skipWithoutPOSIXTools"}
)

// checkProcTestFile is checkProcTestFiles for one file on its own.
func checkProcTestFile(name string, src []byte) []string {
	return checkProcTestFiles(map[string][]byte{name: src})
}

// checkProcTestFiles reports why files, the _test.go sources of one
// package keyed by base name, break plan 2610030243's contract, or
// nil. A file that go build selects for linux and windows but not js
// is a spawn file: it must also build on plan9, so GOOS=plan9 go vet
// type-checks it, and nowhere exec_other.go's stubs build. Every Test
// in a spawn file that runs an sh script, an `sh` argv, or a recipe,
// itself or through a helper, must call a plan9 skip helper first,
// since plan9 has only rc.
func checkProcTestFiles(files map[string][]byte) []string {
	var errs []string
	parsed := map[string]*ast.File{}
	var funcs []*ast.FuncDecl
	for _, name := range slices.Sorted(maps.Keys(files)) {
		f, err := parser.ParseFile(token.NewFileSet(), name, files[name], parser.SkipObjectResolution)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		parsed[name] = f
		funcs = append(funcs, topLevelFuncs(f)...)
	}
	scan := newShScan(funcs)
	for _, name := range slices.Sorted(maps.Keys(parsed)) {
		spawn, onPlan9, bad, err := procTagMismatch(name, files[name])
		switch {
		case err != nil:
			// go/build's error already names the file.
			errs = append(errs, err.Error())
			continue
		case !spawn:
			continue
		case len(bad) > 0:
			errs = append(errs, fmt.Sprintf(
				"%s: builds on linux and windows but %s; tag it unix || windows || plan9",
				name, strings.Join(bad, " and ")))
		}
		if !onPlan9 {
			continue
		}
		for _, fn := range topLevelFuncs(parsed[name]) {
			if isGoTest(fn) && scan.firstOf(fn.Body.List) == shFirst {
				errs = append(errs, fmt.Sprintf(
					"%s: %s needs sh but does not first call skipOnPlan9 or skipWithoutPOSIXTools",
					name, fn.Name.Name))
			}
		}
	}
	return errs
}

// procTagMismatch reports whether go build selects name for linux and
// windows but not js (a spawn file), whether it selects it for plan9,
// and, for a spawn file, each target it gets wrong.
func procTagMismatch(name string, src []byte) (spawn, onPlan9 bool, bad []string, err error) {
	on := make(map[string]bool, len(procTargets))
	for _, tg := range procTargets {
		if on[tg.goos], err = buildsOn(tg.goos, tg.goarch, name, src); err != nil {
			return false, false, nil, err
		}
	}
	if !on["linux"] || !on["windows"] || on["js"] {
		return false, false, nil, nil
	}
	for _, tg := range procTargets {
		switch {
		case tg.want && !on[tg.goos]:
			bad = append(bad, "not on "+tg.label)
		case !tg.want && on[tg.goos]:
			bad = append(bad, "also on "+tg.label)
		}
	}
	return true, on["plan9"], bad, nil
}

// buildsOn reports whether go build selects the file name, holding src,
// for goos/goarch with cgo off: its //go:build or +build line, release
// and compiler tags, and any GOOS/GOARCH file-name suffix all count.
func buildsOn(goos, goarch, name string, src []byte) (bool, error) {
	ctxt := build.Default
	ctxt.GOOS, ctxt.GOARCH = goos, goarch
	ctxt.CgoEnabled = false
	ctxt.BuildTags = nil
	ctxt.OpenFile = func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(src)), nil
	}
	return ctxt.MatchFile(".", name)
}

// topLevelFuncs returns f's top-level functions that have a body.
func topLevelFuncs(f *ast.File) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil {
			out = append(out, fn)
		}
	}
	return out
}

// isGoTest mirrors go test's rule for a test: "Test" alone, or "Test"
// followed by a rune that is not lower case.
func isGoTest(fn *ast.FuncDecl) bool {
	rest, ok := strings.CutPrefix(fn.Name.Name, "Test")
	if !ok || rest == "" {
		return ok
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

// shOrder is what a statement list reaches first.
type shOrder int

const (
	neitherFirst shOrder = iota
	shFirst
	skipFirst
)

// shScan holds the names whose call needs sh (sh) and those whose call
// skips the test on plan9 (skip).
type shScan struct{ sh, skip map[string]bool }

// newShScan seeds shCallees and skipCallees, then adds every helper in
// funcs that reaches either first, until no helper is added: a helper
// that calls writeScript needs sh, one that calls skipOnPlan9 first
// skips.
func newShScan(funcs []*ast.FuncDecl) shScan {
	s := shScan{sh: map[string]bool{}, skip: map[string]bool{}}
	for _, n := range shCallees {
		s.sh[n] = true
	}
	for _, n := range skipCallees {
		s.skip[n] = true
	}
	for added := true; added; {
		added = false
		for _, fn := range funcs {
			name := fn.Name.Name
			if isGoTest(fn) || s.sh[name] || s.skip[name] {
				continue
			}
			switch s.firstOf(fn.Body.List) {
			case shFirst:
				s.sh[name], added = true, true
			case skipFirst:
				s.skip[name], added = true, true
			case neitherFirst:
			}
		}
	}
	return s
}

// firstOf reports whether stmts reach an sh use or a skip first. Only
// a top-level statement that is a skip call counts as a skip; one in a
// branch, a defer, or a closure does not.
func (s shScan) firstOf(stmts []ast.Stmt) shOrder {
	for _, st := range stmts {
		if s.isSkip(st) {
			return skipFirst
		}
		if s.usesSh(st) {
			return shFirst
		}
	}
	return neitherFirst
}

// isSkip reports whether st is a bare call to a skip helper.
func (s shScan) isSkip(st ast.Stmt) bool {
	es, ok := st.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && s.skip[id.Name]
}

// usesSh reports whether n calls an sh helper or holds an "sh" string
// literal. A closure counts only when its own body reaches sh before
// a skip, so a subtest that skips first covers itself, and a skip
// call's arguments (skipWithoutPOSIXTools(t, "sh")) are not a use.
func (s shScan) usesSh(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n := n.(type) {
		case *ast.FuncLit:
			found = s.firstOf(n.Body.List) == shFirst
			return false
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok {
				if s.skip[id.Name] {
					return false
				}
				found = s.sh[id.Name]
			}
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				v, err := strconv.Unquote(n.Value)
				found = err == nil && v == "sh"
			}
		}
		return !found
	})
	return found
}

// checkProcTestDir runs checkProcTestFiles over every _test.go in dir.
func checkProcTestDir(t *testing.T, dir string) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	require.NoError(t, err)
	require.NotEmpty(t, names)
	files := make(map[string][]byte, len(names))
	for _, name := range names {
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		files[filepath.Base(name)] = src
	}
	return checkProcTestFiles(files)
}
