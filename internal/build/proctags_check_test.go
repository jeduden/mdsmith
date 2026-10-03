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
	"path"
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

// checkProcTestFiles reports why files, the _test.go sources of one
// package keyed by base name, break plan 2610030243's contract, or
// nil. A file that go build selects for linux and windows but not js
// is a spawn file: it must also build on plan9, so GOOS=plan9 go vet
// type-checks it, and nowhere exec_other.go's stubs build. Every Test,
// Fuzz, or Benchmark function that builds on plan9 but not js and
// reaches sh, itself or through a helper that builds on plan9, must
// skip on plan9 first, since plan9 has only rc.
func checkProcTestFiles(files map[string][]byte) []string {
	var errs []string
	var tests []fileFunc
	scan := shScan{order: map[string]shOrder{}, helpers: map[string][]ast.Stmt{}}
	fset := token.NewFileSet()
	for _, name := range slices.Sorted(maps.Keys(files)) {
		f, err := parser.ParseFile(fset, name, files[name], parser.SkipObjectResolution)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		on, spawn, bad, err := procTagMismatch(name, files[name])
		if err != nil {
			// go/build's error already names the file.
			errs = append(errs, err.Error())
			continue
		}
		if spawn && len(bad) > 0 {
			errs = append(errs, fmt.Sprintf(
				"%s: builds on linux and windows but %s; tag it unix || windows || plan9",
				name, strings.Join(bad, " and ")))
		}
		// Only plan9's build of the package runs on plan9, so only its
		// files hold the helpers a test reaches there. A test that also
		// builds on js is left to the js/wasm gate, where sh fails too.
		if on["plan9"] {
			tests = scan.index(name, f, tests, !on["js"])
		}
	}
	for _, tf := range tests {
		if scan.firstOf(tf.fn.Body.List) == shFirst {
			errs = append(errs, fmt.Sprintf(
				"%s: %s needs sh but does not first skip on plan9",
				tf.file, tf.fn.Name.Name))
		}
	}
	return errs
}

// fileFunc is a test function and the file that declares it.
type fileFunc struct {
	file string
	fn   *ast.FuncDecl
}

// procTagMismatch reports which procTargets go build selects name for,
// keyed by GOOS, whether that makes it a spawn file (linux and windows
// but not js), and, for a spawn file, each target it gets wrong.
func procTagMismatch(name string, src []byte) (on map[string]bool, spawn bool, bad []string, err error) {
	on = make(map[string]bool, len(procTargets))
	for _, tg := range procTargets {
		if on[tg.goos], err = buildsOn(tg.goos, tg.goarch, name, src); err != nil {
			return nil, false, nil, err
		}
	}
	if !on["linux"] || !on["windows"] || on["js"] {
		return on, false, nil, nil
	}
	for _, tg := range procTargets {
		switch {
		case tg.want && !on[tg.goos]:
			bad = append(bad, "not on "+tg.label)
		case !tg.want && on[tg.goos]:
			bad = append(bad, "also on "+tg.label)
		}
	}
	return on, true, bad, nil
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

// isGoTest mirrors go test's rule for what it runs: a test, a fuzz
// target (whose seeds run as tests), or a benchmark, named "Test",
// "Fuzz", or "Benchmark" alone or followed by a rune that is not lower
// case. A method is never one.
func isGoTest(fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		return false
	}
	for _, prefix := range []string{"Test", "Fuzz", "Benchmark"} {
		if rest, ok := strings.CutPrefix(fn.Name.Name, prefix); ok {
			if rest == "" {
				return true
			}
			r, _ := utf8.DecodeRuneInString(rest)
			return !unicode.IsLower(r)
		}
	}
	return false
}

// shOrder is what a statement list reaches first.
type shOrder int

const (
	neitherFirst shOrder = iota
	shFirst
	skipFirst
)

// shScan holds what a call to each resolved helper reaches first
// (order) and the bodies of the helpers not yet resolved (helpers). A
// function or func-valued var is keyed by its name, a method by "."
// and its name, since only a selector call can reach it.
type shScan struct {
	order   map[string]shOrder
	helpers map[string][]ast.Stmt
}

// index records f's helpers in s and, when withTests, appends its test
// functions to tests, which it returns.
func (s shScan) index(name string, f *ast.File, tests []fileFunc, withTests bool) []fileFunc {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			switch {
			case d.Body == nil:
			case isGoTest(d):
				if withTests {
					tests = append(tests, fileFunc{name, d})
				}
			case d.Recv != nil:
				s.helpers["."+d.Name.Name] = d.Body.List
			default:
				s.helpers[d.Name.Name] = d.Body.List
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, v := range vs.Values {
					if lit, ok := v.(*ast.FuncLit); ok && i < len(vs.Names) {
						s.helpers[vs.Names[i].Name] = lit.Body.List
					}
				}
			}
		}
	}
	return tests
}

// orderOf reports what a call to the helper key reaches first,
// resolving its body once, so helpers resolve in any declaration
// order. A call back into a helper that is still resolving, as
// recursion makes, reaches neither; so does an unknown key.
func (s shScan) orderOf(key string) shOrder {
	if o, ok := s.order[key]; ok {
		return o
	}
	body, ok := s.helpers[key]
	if !ok {
		return neitherFirst
	}
	s.order[key] = neitherFirst
	o := s.firstOf(body)
	s.order[key] = o
	return o
}

// calleeKey returns the helper key a call to, or a reference to, e
// names, or "" for anything else.
func calleeKey(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return "." + e.Sel.Name
	}
	return ""
}

// firstOf reports whether stmts reach an sh use or a plan9 skip first.
// Only a top-level statement counts as a skip; one in a branch other
// than a plan9 one, a defer, or a closure does not.
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

// isSkip reports whether st skips on plan9: a call to a helper that
// skips first, `if runtime.GOOS == "plan9"` around a Skip call, or a
// `switch runtime.GOOS` whose "plan9" case calls Skip.
func (s shScan) isSkip(st ast.Stmt) bool {
	switch st := st.(type) {
	case *ast.ExprStmt:
		call, ok := st.X.(*ast.CallExpr)
		return ok && s.orderOf(calleeKey(call.Fun)) == skipFirst
	case *ast.IfStmt:
		cond, ok := st.Cond.(*ast.BinaryExpr)
		return ok && st.Init == nil && cond.Op == token.EQL &&
			(isGOOS(cond.X) && isPlan9(cond.Y) || isPlan9(cond.X) && isGOOS(cond.Y)) &&
			callsSkip(st.Body.List)
	case *ast.SwitchStmt:
		if st.Init != nil || !isGOOS(st.Tag) {
			return false
		}
		for _, c := range st.Body.List {
			cc := c.(*ast.CaseClause)
			if slices.ContainsFunc(cc.List, isPlan9) && callsSkip(cc.Body) {
				return true
			}
		}
	}
	return false
}

// isGOOS reports whether e is runtime.GOOS.
func isGOOS(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "GOOS" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "runtime"
}

// isPlan9 reports whether e is the string literal "plan9".
func isPlan9(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `"plan9"`
}

// callsSkip reports whether a top-level statement in stmts is a Skip,
// Skipf, or SkipNow call on a test or benchmark.
func callsSkip(stmts []ast.Stmt) bool {
	return slices.ContainsFunc(stmts, func(st ast.Stmt) bool {
		es, ok := st.(*ast.ExprStmt)
		if !ok {
			return false
		}
		call, ok := es.X.(*ast.CallExpr)
		return ok && isSkipCall(call)
	})
}

// isSkipCall reports whether call is x.Skip, x.Skipf, or x.SkipNow.
func isSkipCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "Skip", "Skipf", "SkipNow":
		return true
	}
	return false
}

// usesSh reports whether n calls an sh helper, passes one by name (as
// t.Run(name, scriptTest) does), or holds a string literal whose first
// word is sh or a path ending in /sh: an "sh" argv, a "sh -c" recipe,
// or a "#!/bin/sh" script. A closure counts only when its own body
// reaches sh before a skip, so a subtest that skips first covers
// itself. The arguments of a skip helper or a Skip call, such as
// skipWithoutPOSIXTools(t, "sh"), and the operands of == or != are not
// a use.
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
			if isSkipCall(n) {
				return false
			}
			switch s.orderOf(calleeKey(n.Fun)) {
			case skipFirst:
				return false
			case shFirst:
				found = true
			case neitherFirst:
			}
			for _, arg := range n.Args {
				if s.orderOf(calleeKey(arg)) == shFirst {
					found = true
				}
			}
		case *ast.BinaryExpr:
			// A comparison with "sh" reads a name; it runs nothing.
			return n.Op != token.EQL && n.Op != token.NEQ
		case *ast.BasicLit:
			found = isShLiteral(n)
		}
		return !found
	})
	return found
}

// isShLiteral reports whether lit is a string whose first word, after
// any #! prefix, is sh or a path ending in /sh.
func isShLiteral(lit *ast.BasicLit) bool {
	if lit.Kind != token.STRING {
		return false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}
	words := strings.Fields(v)
	return len(words) > 0 && path.Base(strings.TrimPrefix(words[0], "#!")) == "sh"
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
