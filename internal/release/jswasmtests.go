// Package release: jswasmtests.go runs a package's js/wasm-only Go
// tests under Node for the ci.yml `wasm` job, replacing the inline
// shell that step used to carry (docs/development/release-tooling.md).
//
// A native `go test` never compiles a file that only a js/wasm build
// selects, so those tests need their own run. The runner finds them by
// diffing the test files `go list` reports under GOOS=js GOARCH=wasm
// against a linux/amd64 `go list`, both with cgo off, parses each file with go/parser to list
// its TestXxx(*testing.T) functions, runs exactly those under Node with
// `go test -json`, and fails unless every one of them reports a "pass"
// event. A skipped test therefore fails the step by name; a
// commented-out one is not listed.
//
// With --all (RunJSWasmPackage) it instead runs every test a js/wasm
// build compiles under Node, untagged ones included, and fails on any
// go test failure, when no test passes, or when a test from the
// js/wasm-only files skips, so an untagged test that fails without a
// real process or pipe fails CI and one run covers both guarantees.
// Both modes need <pkg> to match exactly one package.
package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// jsWasmEnv is the extra environment for the js/wasm `go list` and
// `go test` calls. CGO_ENABLED=0: an inherited CGO_ENABLED=1 would set
// the cgo build tag even for js/wasm, which has no cgo, and change the
// files the js/wasm list reports.
var jsWasmEnv = []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}

// nativeListEnv is the extra environment for the native `go list` the
// js/wasm one is diffed against. It pins one target, the linux/amd64 CI
// runner, with cgo off as js/wasm always has it, so the host's OS, arch
// and cgo tags cannot change which test files count as js/wasm-only: a
// //go:build !cgo file is shared everywhere, a !linux one js/wasm-only
// everywhere.
var nativeListEnv = []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}

// pkgLinePrefix starts the line testFilesTemplate prints for each
// package matched, ahead of that package's test files.
const pkgLinePrefix = "#pkg "

// pkgLineTemplate makes `go list` print one pkgLinePrefix line with
// the import path of each package matched.
const pkgLineTemplate = pkgLinePrefix + `{{.ImportPath}}{{"\n"}}`

// testFilesTemplate makes `go list` print, per package, a
// pkgLinePrefix line with its import path, then one absolute path per
// test file, internal (TestGoFiles) and external (XTestGoFiles) alike.
const testFilesTemplate = pkgLineTemplate +
	`{{range .TestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}` +
	`{{range .XTestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}`

// goRunFunc runs `go args...` with env appended to the process
// environment, writing its stdout to stdout as it is produced. Stderr
// is the caller's.
type goRunFunc func(stdout io.Writer, env []string, args ...string) error

// jsWasmDeps are the side effects runJSWasmTestsWith and
// runJSWasmPackageWith need, injectable so a test can drive every
// branch without Go or Node.
type jsWasmDeps struct {
	run      goRunFunc
	readFile func(string) ([]byte, error)
	path     string    // PATH handed to the Node runtime
	out      io.Writer // receives the go test console log
}

// RunJSWasmTests runs pkg's js/wasm-only tests under Node from root
// and fails unless every listed test passes. The go test console log
// (the -v text, rebuilt from the -json stream) is streamed to out as
// each line arrives, so a hung test still shows its progress.
func RunJSWasmTests(root, pkg string, out io.Writer) error {
	return runJSWasmTestsWith(osJSWasmDeps(root, out), pkg)
}

// osGoRunner is the production goRunFunc: it runs go in dir with
// stderr inherited, so `go list` and build errors reach the CI log.
func osGoRunner(dir string) goRunFunc {
	return func(stdout io.Writer, env []string, args ...string) error {
		cmd := exec.Command("go", args...) //nolint:gosec // CI-only; args built here
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdout = stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
}

// output runs go through d.run and returns its whole stdout, for the
// short go env and go list calls.
func (d jsWasmDeps) output(env []string, args ...string) ([]byte, error) {
	var b bytes.Buffer
	err := d.run(&b, env, args...)
	return b.Bytes(), err
}

// RunJSWasmPackage runs every test of pkg that a js/wasm build compiles
// under Node, untagged ones included, and fails on any go test failure
// or when no test passes. pkg must match exactly one package.
// Unlike RunJSWasmTests it does not require every test to pass: a skip
// of a test the native build shares is fine as long as some test
// passed. A skip of a test from the js/wasm-only files still fails, by
// name. requireJSOnly also fails, before go test, when pkg has no
// js/wasm-only Test function, so a lost stub test file cannot pass the
// gate vacuously.
func RunJSWasmPackage(root, pkg string, requireJSOnly bool, out io.Writer) error {
	return runJSWasmPackageWith(osJSWasmDeps(root, out), pkg, requireJSOnly)
}

// osJSWasmDeps is the production jsWasmDeps: go run from root, files
// read from disk, and the process PATH handed to Node.
func osJSWasmDeps(root string, out io.Writer) jsWasmDeps {
	return jsWasmDeps{
		run:      osGoRunner(root),
		readFile: os.ReadFile,
		path:     os.Getenv("PATH"),
		out:      out,
	}
}

func runJSWasmPackageWith(d jsWasmDeps, pkg string, requireJSOnly bool) error {
	execFlag, err := d.execFlag()
	if err != nil {
		return err
	}
	// The js/wasm-only tests must pass by name, as in the default mode;
	// a skip of a test the native build shares stays fine. Unlike the
	// default mode, a package may have no such files or tests.
	// -e lets a missing package still list once so go test reports why.
	// requireJSOnly fails such a package before go test, so it leaves -e
	// off and go list reports the load error itself.
	var listFlags []string
	if !requireJSOnly {
		listFlags = []string{"-e"}
	}
	files, err := listJSOnlyFiles(d, "test-js-wasm --all", pkg, listFlags...)
	if err != nil {
		return err
	}
	names, err := testFuncsIn(d, files)
	if err != nil {
		return err
	}
	if requireJSOnly && len(names) == 0 {
		return fmt.Errorf("no js/wasm-only Test functions in %s", pkg)
	}
	passed, err := d.goTest(pkg, execFlag)
	if err != nil {
		return err
	}
	// go test exits 0 on a package with no test files, or whose every
	// test skipped, so a gate with no pass would pass vacuously.
	if len(passed) == 0 {
		return fmt.Errorf("no test passed in %s under js/wasm", pkg)
	}
	return checkAllPassed(names, passed)
}

// execFlag returns the go test -exec value for d.path and the
// go_js_wasm_exec under `go env GOROOT`, erroring when go env fails or
// prints nothing, or when the flag cannot be quoted.
func (d jsWasmDeps) execFlag() (string, error) {
	out, err := d.output(nil, "env", "GOROOT")
	if err != nil {
		return "", fmt.Errorf("go env GOROOT: %w", err)
	}
	goroot := strings.TrimSpace(string(out))
	if goroot == "" {
		return "", errors.New("go env GOROOT: empty output")
	}
	return jsWasmExecFlag(d.path, goroot)
}

// requireOnePackage errors unless pkgs, the import paths pkg matched,
// holds exactly one. mode names the command in the error.
func requireOnePackage(mode, pkg string, pkgs []string) error {
	if len(pkgs) != 1 {
		return fmt.Errorf("%s needs exactly one package; %s matches %d", mode, pkg, len(pkgs))
	}
	return nil
}

// goTestNamed runs exactly the tests in names under Node; see goTest.
// It errors on an empty names rather than run the whole package, which
// would let checkAllPassed pass on an empty want list.
func (d jsWasmDeps) goTestNamed(pkg, execFlag string, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("no test names to run in %s under js/wasm", pkg)
	}
	return d.goTest(pkg, execFlag, "-run", "^("+strings.Join(names, "|")+")$")
}

// goTest runs `go test -json` with extra args on pkg under Node,
// streaming the console log to d.out, and returns the names that
// reported a pass and no skip (see testJSONWriter.unmasked).
//
// env -i in execFlag: wasm_exec.js caps args plus environment at
// ~8 KB. It wraps only the Node runtime, so the go command keeps the
// full Go and proxy environment. -json, not -v: test2json frames each
// result, so a test whose output lacks a trailing newline still
// reports its own pass event instead of a glued `x--- PASS` line.
func (d jsWasmDeps) goTest(pkg, execFlag string, extra ...string) ([]string, error) {
	args := append([]string{"test", "-json", "-exec=" + execFlag}, extra...)
	w := &testJSONWriter{out: d.out}
	runErr := d.run(w, jsWasmEnv, append(args, pkg)...)
	w.Flush()
	if runErr != nil {
		return nil, fmt.Errorf("go test %s under js/wasm: %w", pkg, runErr)
	}
	return w.unmasked(), nil
}

func runJSWasmTestsWith(d jsWasmDeps, pkg string) error {
	execFlag, err := d.execFlag()
	if err != nil {
		return err
	}
	// pkg must match exactly one package: the pass check matches bare
	// test names, so across packages a same-named test that passed
	// elsewhere could stand in for a skipped one. No js/wasm-only file
	// or Test function is an error, so a broken lookup cannot pass
	// vacuously.
	files, err := listJSOnlyFiles(d, "test-js-wasm", pkg)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no js/wasm-only test files in %s", pkg)
	}
	names, err := testFuncsIn(d, files)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no Test functions in js/wasm-only files %s", strings.Join(files, ", "))
	}
	passed, err := d.goTestNamed(pkg, execFlag, names)
	if err != nil {
		return err
	}
	return checkAllPassed(names, passed)
}

// listJSOnlyFiles lists pkg's test files that only a js/wasm build
// compiles, for both modes; an empty list is the caller's call, as
// --all also runs a package that has none. mode names the command in the one-package error; listFlags go
// to the js/wasm `go list`. --all passes -e, unless requireJSOnly, so a
// missing package still lists once and go test reports why.
func listJSOnlyFiles(d jsWasmDeps, mode, pkg string, listFlags ...string) ([]string, error) {
	args := append(append([]string{"list"}, listFlags...), "-f", testFilesTemplate, pkg)
	jsOut, err := d.output(jsWasmEnv, args...)
	if err != nil {
		return nil, fmt.Errorf("go list (js/wasm) %s: %w", pkg, err)
	}
	pkgs, jsFiles := splitListOutput(jsOut)
	if err := requireOnePackage(mode, pkg, pkgs); err != nil {
		return nil, err
	}
	// -e: a package whose non-test files are all js/wasm-only has no
	// native build, and plain `go list` exits 1 on it. Every one of its
	// test files is then js/wasm-only.
	nativeOut, err := d.output(nativeListEnv, "list", "-e", "-f", testFilesTemplate, pkg)
	if err != nil {
		return nil, fmt.Errorf("go list (native) %s: %w", pkg, err)
	}
	_, nativeFiles := splitListOutput(nativeOut)
	return JSOnlyTestFiles(jsFiles, nativeFiles), nil
}

// testFuncsIn collects the Test functions declared across files, in
// file then source order, each name once: an internal and an external
// test file may both declare TestX, and go test reports both under the
// one name. None is not an error here: under --all a js/wasm-only file
// may hold only helpers or a TestMain.
func testFuncsIn(d jsWasmDeps, files []string) ([]string, error) {
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
		for _, n := range got {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
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
	return fmt.Errorf("%d of %d js/wasm-only tests passed; not passed: %s",
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
// of type *T from the file's "testing" import (`*T` when it is
// dot-imported). Comments are skipped by the parser, so a
// commented-out test is never listed.
func ListTestFuncs(src []byte) ([]string, error) {
	f, funcs, err := topLevelFuncs(src)
	if err != nil {
		return nil, err
	}
	pkgName := testingImportName(f)
	if pkgName == "" {
		return nil, nil
	}
	var names []string
	for _, fn := range funcs {
		if isTestName(fn.Name.Name) && takesTestingT(fn, pkgName) {
			names = append(names, fn.Name.Name)
		}
	}
	return names, nil
}

// testingImportName returns the name the file refers to the "testing"
// package by: "." for a dot import, or "" when it is not imported by a
// usable name.
func testingImportName(f *ast.File) string {
	for _, imp := range f.Imports {
		// go/parser already rejected any path that is not a valid
		// string literal, so Unquote cannot fail here.
		if p, _ := strconv.Unquote(imp.Path.Value); p != "testing" {
			continue
		}
		if imp.Name == nil {
			return "testing"
		}
		if imp.Name.Name == "_" {
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
// *<pkgName>.T, or of type *T when pkgName is "." (a dot import).
func takesTestingT(fn *ast.FuncDecl, pkgName string) bool {
	params := fn.Type.Params.List
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	if pkgName == "." {
		id, ok := star.X.(*ast.Ident)
		return ok && id.Name == "T"
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkgName
}

// testJSONWriter decodes a `go test -json` stream as it is written.
// Each complete line's Output (the `go test -v` text) goes to out at
// once, and the top-level tests that report a "pass" event collect in
// passed, and those that report "skip" in skipped, in stream order. A line that is not a JSON event, such as a
// `go: downloading` notice, passes through to out unchanged; a
// mangled `{`-led event is dropped (see echoNonEvent).
type testJSONWriter struct {
	out     io.Writer
	partial []byte // bytes after the last newline, awaiting the rest
	passed  []string
	skipped []string
}

// unmasked returns passed without the names that also skipped. go test
// reports a TestX of the internal and of the external test package
// under the same name, so one's pass must not hide the other's skip.
// The stream cannot tell the two apart, so a shared test's skip also
// drops a same-named js/wasm-only test's pass: the gate errs toward
// failing.
func (w *testJSONWriter) unmasked() []string {
	var out []string
	for _, n := range w.passed {
		if !slices.Contains(w.skipped, n) {
			out = append(out, n)
		}
	}
	return out
}

// Write buffers p and decodes every line it completes. It never fails:
// a write error on out must not abort the go test run it is fed from.
func (w *testJSONWriter) Write(p []byte) (int, error) {
	w.partial = append(w.partial, p...)
	for {
		i := bytes.IndexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		w.line(w.partial[:i])
		w.partial = w.partial[i+1:]
	}
	// Move the remainder to the front so the buffer does not grow
	// with the length of the whole stream.
	w.partial = append(w.partial[:0:0], w.partial...)
	return len(p), nil
}

// Flush decodes a final line that had no trailing newline.
func (w *testJSONWriter) Flush() {
	if len(w.partial) > 0 {
		w.line(w.partial)
		w.partial = nil
	}
}

// line decodes one stream line: blank lines are dropped, a line that
// is not an event goes through echoNonEvent, and an event's Output is
// echoed with a top-level "pass" or "skip" recorded.
func (w *testJSONWriter) line(b []byte) {
	if len(bytes.TrimSpace(b)) == 0 {
		return
	}
	var ev testEvent
	if err := json.Unmarshal(b, &ev); err != nil {
		echoNonEvent(w.out, b)
		return
	}
	_, _ = io.WriteString(w.out, ev.Output)
	if ev.Test == "" || strings.Contains(ev.Test, "/") {
		return
	}
	switch ev.Action {
	case "pass":
		w.passed = append(w.passed, ev.Test)
	case "skip":
		w.skipped = append(w.skipped, ev.Test)
	}
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

// splitListOutput splits testFilesTemplate output into the import
// paths of the packages it covers and their test file paths.
func splitListOutput(b []byte) (pkgs, files []string) {
	for _, line := range splitLines(b) {
		if p, ok := strings.CutPrefix(line, pkgLinePrefix); ok {
			pkgs = append(pkgs, p)
			continue
		}
		files = append(files, line)
	}
	return pkgs, files
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
