// Package release: jswasmtests.go runs a package's js/wasm-only Go
// tests under Node for the ci.yml `wasm` job, replacing the inline
// shell that step used to carry (docs/development/release-tooling.md).
//
// A native `go test` never compiles a file that only a js/wasm build
// selects, so those tests need their own run. The runner finds them by
// diffing the test files `go list` reports under GOOS=js GOARCH=wasm
// against a native `go list`, parses each file with go/parser to list
// its TestXxx(*testing.T) functions, runs exactly those under Node, and
// fails unless every one of them reports `--- PASS`. A skipped test
// therefore fails the step by name; a commented-out one is not listed.
package release

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// jsWasmEnv is the extra environment for the js/wasm `go list` and
// `go test` calls.
var jsWasmEnv = []string{"GOOS=js", "GOARCH=wasm"}

// testFilesTemplate makes `go list` print one absolute path per test
// file, internal (TestGoFiles) and external (XTestGoFiles) alike.
const testFilesTemplate = `{{range .TestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}` +
	`{{range .XTestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}`

// passLineRE matches a top-level `--- PASS: TestX (0.00s)` line from
// `go test -v`. Subtest lines are indented, so the anchor skips them.
var passLineRE = regexp.MustCompile(`(?m)^--- PASS: (Test\S*) \(`)

// goRunFunc runs `go args...` with env appended to the process
// environment and returns its stdout. Stderr is the caller's.
type goRunFunc func(env []string, args ...string) ([]byte, error)

// jsWasmDeps are the side effects runJSWasmTestsWith needs, injectable so
// a test can drive every branch without Go or Node.
type jsWasmDeps struct {
	run      goRunFunc
	readFile func(string) ([]byte, error)
	path     string    // PATH handed to the Node runtime
	out      io.Writer // receives the go test -v log
}

// RunJSWasmTests runs pkg's js/wasm-only tests under Node from root
// and fails unless every listed test passes. The go test -v log is
// written to out.
func RunJSWasmTests(root, pkg string, out io.Writer) error {
	return runJSWasmTestsWith(jsWasmDeps{
		run:      osGoRunner(root),
		readFile: os.ReadFile,
		path:     os.Getenv("PATH"),
		out:      out,
	}, pkg)
}

// osGoRunner is the production goRunFunc: it runs go in dir with
// stderr inherited, so `go list` and build errors reach the CI log.
func osGoRunner(dir string) goRunFunc {
	return func(env []string, args ...string) ([]byte, error) {
		cmd := exec.Command("go", args...) //nolint:gosec // CI-only; args built here
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		cmd.Stderr = os.Stderr
		return cmd.Output()
	}
}

func runJSWasmTestsWith(d jsWasmDeps, pkg string) error {
	gorootOut, err := d.run(nil, "env", "GOROOT")
	if err != nil {
		return fmt.Errorf("go env GOROOT: %w", err)
	}
	goroot := strings.TrimSpace(string(gorootOut))
	if goroot == "" {
		return errors.New("go env GOROOT: empty output")
	}
	files, err := jsOnlyFilesOf(d, pkg)
	if err != nil {
		return err
	}
	names, err := testsInFiles(d, files)
	if err != nil {
		return err
	}
	execFlag, err := jsWasmExecFlag(d.path, goroot)
	if err != nil {
		return err
	}
	// env -i in -exec: wasm_exec.js caps args plus environment at
	// ~8 KB. It wraps only the Node runtime, so the go command keeps
	// the full Go and proxy environment.
	log, runErr := d.run(jsWasmEnv, "test", "-v", "-exec="+execFlag,
		"-run", "^("+strings.Join(names, "|")+")$", pkg)
	_, _ = d.out.Write(log)
	if runErr != nil {
		return fmt.Errorf("go test %s under js/wasm: %w", pkg, runErr)
	}
	return checkAllPassed(names, PassedTests(log))
}

// jsOnlyFilesOf lists pkg's test files that only a js/wasm build
// compiles, erroring when there are none so a broken lookup cannot
// pass vacuously.
func jsOnlyFilesOf(d jsWasmDeps, pkg string) ([]string, error) {
	jsOut, err := d.run(jsWasmEnv, "list", "-f", testFilesTemplate, pkg)
	if err != nil {
		return nil, fmt.Errorf("go list (js/wasm) %s: %w", pkg, err)
	}
	nativeOut, err := d.run(nil, "list", "-f", testFilesTemplate, pkg)
	if err != nil {
		return nil, fmt.Errorf("go list (native) %s: %w", pkg, err)
	}
	files := JSOnlyTestFiles(splitLines(jsOut), splitLines(nativeOut))
	if len(files) == 0 {
		return nil, fmt.Errorf("no js/wasm-only test files in %s", pkg)
	}
	return files, nil
}

// testsInFiles collects the Test functions declared across files, in
// file then source order, erroring when there are none.
func testsInFiles(d jsWasmDeps, files []string) ([]string, error) {
	var names []string
	for _, f := range files {
		src, err := d.readFile(f)
		if err != nil {
			return nil, err
		}
		got, err := ListTestFuncs(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		names = append(names, got...)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no Test functions in js/wasm-only files %s", strings.Join(files, ", "))
	}
	return names, nil
}

// checkAllPassed errors with the names in want that are missing from
// passed: a skipped test, or one -run did not select.
func checkAllPassed(want, passed []string) error {
	var missing []string
	for _, n := range want {
		if !slices.Contains(passed, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%d of %d js/wasm tests passed; not passed: %s",
		len(want)-len(missing), len(want), strings.Join(missing, ", "))
}

// JSOnlyTestFiles returns the sorted, de-duplicated entries of jsFiles
// that are absent from nativeFiles. Blank entries are ignored.
func JSOnlyTestFiles(jsFiles, nativeFiles []string) []string {
	var out []string
	for _, f := range jsFiles {
		if f == "" || slices.Contains(nativeFiles, f) || slices.Contains(out, f) {
			continue
		}
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

// ListTestFuncs parses a Go source file and returns, in source order,
// the top-level functions `go test` runs as tests: named Test or
// TestX where X is not a lowercase letter, with exactly one parameter
// of type *T from the file's "testing" import. Comments are skipped by
// the parser, so a commented-out test is never listed.
func ListTestFuncs(src []byte) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	pkgName := testingImportName(f)
	if pkgName == "" {
		return nil, nil
	}
	var names []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !isTestName(fn.Name.Name) || !takesTestingT(fn, pkgName) {
			continue
		}
		names = append(names, fn.Name.Name)
	}
	return names, nil
}

// testingImportName returns the name the file refers to the "testing"
// package by, or "" when it is not imported by a usable name.
func testingImportName(f *ast.File) string {
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err != nil || p != "testing" {
			continue
		}
		if imp.Name == nil {
			return "testing"
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return ""
		}
		return imp.Name.Name
	}
	return ""
}

// isTestName mirrors go test's rule: "Test" alone, or "Test" followed
// by a character that is not a lowercase ASCII letter.
func isTestName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Test")
	return ok && (rest == "" || rest[0] < 'a' || rest[0] > 'z')
}

// takesTestingT reports whether fn has exactly one parameter, of type
// *<pkgName>.T.
func takesTestingT(fn *ast.FuncDecl, pkgName string) bool {
	params := fn.Type.Params.List
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkgName
}

// PassedTests returns the top-level test names that `go test -v`
// reported as `--- PASS` in log, in log order.
func PassedTests(log []byte) []string {
	matches := passLineRE.FindAllSubmatch(log, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, string(m[1]))
	}
	return out
}

// jsWasmExecFlag builds the go test -exec value that runs the Node
// wrapper go_js_wasm_exec under `env -i` with only PATH set.
func jsWasmExecFlag(path, goroot string) (string, error) {
	pathArg, err := quoteExecArg("PATH=" + path)
	if err != nil {
		return "", err
	}
	execArg, err := quoteExecArg(goroot + "/lib/wasm/go_js_wasm_exec")
	if err != nil {
		return "", err
	}
	return "env -i " + pathArg + " " + execArg, nil
}

// quoteExecArg quotes s for go's -exec splitting, which honours single
// and double quotes but has no escapes: single quotes unless s holds
// one, then double quotes, and an error when s holds both.
func quoteExecArg(s string) (string, error) {
	switch {
	case !strings.Contains(s, "'"):
		return "'" + s + "'", nil
	case !strings.Contains(s, `"`):
		return `"` + s + `"`, nil
	}
	return "", fmt.Errorf("cannot quote %q for go test -exec: it holds both quote kinds", s)
}

// splitLines splits go list output into non-empty, CR-trimmed lines.
func splitLines(b []byte) []string {
	var out []string
	for _, line := range bytes.Split(b, []byte("\n")) {
		if s := strings.TrimRight(string(line), "\r"); s != "" {
			out = append(out, s)
		}
	}
	return out
}
