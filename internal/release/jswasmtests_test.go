package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSOnlyTestFiles(t *testing.T) {
	tests := []struct {
		name   string
		js     []string
		native []string
		want   []string
	}{
		{"none", nil, nil, nil},
		{"all shared", []string{"/p/a_test.go"}, []string{"/p/a_test.go"}, nil},
		{
			"js only kept and sorted",
			[]string{"/p/z_test.go", "/p/a_test.go", "/p/shared_test.go"},
			[]string{"/p/shared_test.go", "/p/native_test.go"},
			[]string{"/p/a_test.go", "/p/z_test.go"},
		},
		{"blank lines ignored", []string{"", "/p/a_test.go", ""}, []string{""}, []string{"/p/a_test.go"}},
		{"duplicates collapsed", []string{"/p/a_test.go", "/p/a_test.go"}, nil, []string{"/p/a_test.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, JSOnlyTestFiles(tt.js, tt.native))
		})
	}
}

var listTestFuncsCases = []struct {
	name string
	src  string
	want []string
}{
	{
		"plain tests in source order",
		`package p
import "testing"
func TestB(t *testing.T) {}
func TestA(t *testing.T) {}
`,
		[]string{"TestB", "TestA"},
	},
	{
		"unnamed and underscore parameters",
		`package p
import "testing"
func TestUnnamed(*testing.T) {}
func Test_Under(_ *testing.T) {}
func Test(t *testing.T) {}
`,
		[]string{"TestUnnamed", "Test_Under", "Test"},
	},
	{
		"commented out tests ignored",
		`package p
import "testing"
// func TestLine(t *testing.T) {}
/*
func TestBlock(t *testing.T) {}
*/
func TestKept(t *testing.T) {}
`,
		[]string{"TestKept"},
	},
	{
		"non-tests ignored",
		`package p
import "testing"
func TestMain(m *testing.M) {}
func Testlower(t *testing.T) {}
func BenchmarkX(b *testing.B) {}
func helper(t *testing.T) {}
func TestTwoParams(t *testing.T, x int) {}
func TestNoParams() {}
func TestValue(t testing.T) {}
func TestOtherT(t *other.T) {}
type s struct{}
func (s) TestMethod(t *testing.T) {}
`,
		nil,
	},
	{
		"dot-imported testing",
		`package p
import . "testing"
func TestDot(t *T) {}
func TestQualified(t *testing.T) {}
func TestMain(m *M) {}
`,
		[]string{"TestDot"},
	},
	{
		"aliased testing import",
		`package p
import tst "testing"
func TestAlias(t *tst.T) {}
func TestWrongName(t *testing.T) {}
`,
		[]string{"TestAlias"},
	},
	{
		"no testing import",
		`package p
func TestX(t *testing.T) {}
`,
		nil,
	},
	{
		"multiline signature",
		`package p
import "testing"
func TestWrapped(
t *testing.T,
) {
}
`,
		[]string{"TestWrapped"},
	},
}

func TestListTestFuncs(t *testing.T) {
	for _, tt := range listTestFuncsCases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ListTestFuncs([]byte(tt.src))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("syntax error", func(t *testing.T) {
		_, err := ListTestFuncs([]byte("package p\nfunc TestX(t *testing.T {\n"))
		assert.Error(t, err)
	})
}

// goTestJSON renders events as a `go test -json` stream, one JSON
// object per line.
func goTestJSON(t *testing.T, evs ...testEvent) string {
	t.Helper()
	var b strings.Builder
	for _, ev := range evs {
		line, err := json.Marshal(ev)
		require.NoError(t, err)
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// result is the output-plus-terminal event pair go test -json emits
// when a test resolves with action ("pass", "skip", "fail").
func result(action, test string) []testEvent {
	return []testEvent{
		{Action: "output", Test: test, Output: "--- " + strings.ToUpper(action) + ": " + test + " (0.00s)\n"},
		{Action: action, Test: test},
	}
}

func TestReadTestJSON(t *testing.T) {
	evs := slices.Concat(
		[]testEvent{{Action: "run", Test: "TestA"}},
		result("pass", "TestA/sub"),
		result("pass", "TestA"),
		result("skip", "TestB"),
		result("pass", "TestC/inner"),
		result("fail", "TestC"),
		// Output without a trailing newline: the -v text glues the
		// result line onto it, but the pass event still stands alone.
		[]testEvent{{Action: "output", Test: "TestD", Output: "partial"}},
		result("pass", "TestD"),
		[]testEvent{
			{Action: "output", Output: "PASS\n"},
			{Action: "pass"}, // package-level: no Test
		},
	)
	stream := "go: downloading example.com/m v1.0.0\n\n" + goTestJSON(t, evs...)

	log, passed := ReadTestJSON([]byte(stream))
	assert.Equal(t, []string{"TestA", "TestD"}, passed)
	assert.Contains(t, string(log), "go: downloading example.com/m v1.0.0\n")
	assert.Contains(t, string(log), "partial--- PASS: TestD (0.00s)\n")
	assert.Contains(t, string(log), "--- SKIP: TestB (0.00s)\n")
	assert.True(t, strings.HasSuffix(string(log), "PASS\n"))

	log, passed = ReadTestJSON(nil)
	assert.Nil(t, log)
	assert.Nil(t, passed)
}

func TestJSWasmExecFlag(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		goroot  string
		want    string
		wantErr string
	}{
		{
			"plain",
			"/usr/bin:/bin", "/go",
			`env -i 'PATH=/usr/bin:/bin' '/go/lib/wasm/go_js_wasm_exec'`,
			"",
		},
		{
			"spaces survive in quotes",
			"/a b:/c", "/go root",
			`env -i 'PATH=/a b:/c' '/go root/lib/wasm/go_js_wasm_exec'`,
			"",
		},
		{
			"single quote uses double quotes",
			"/it's", "/go",
			`env -i "PATH=/it's" '/go/lib/wasm/go_js_wasm_exec'`,
			"",
		},
		{"both quote kinds rejected", `/a'b"c`, "/go", "", "cannot quote"},
		{"both quote kinds in goroot rejected", "/bin", `/g'o"`, "", "cannot quote"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsWasmExecFlag(tt.path, tt.goroot)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQuoteExecArg(t *testing.T) {
	got, err := quoteExecArg("a b")
	require.NoError(t, err)
	assert.Equal(t, "'a b'", got)

	got, err = quoteExecArg("it's")
	require.NoError(t, err)
	assert.Equal(t, `"it's"`, got)

	_, err = quoteExecArg(`'"`)
	assert.Error(t, err)
}

// fakeGo answers the go invocations runJSWasmTestsWith makes: `go env
// GOROOT`, the two `go list` calls (told apart by GOOS=js in env), and
// `go test`. It records every call so tests can pin the argv.
type fakeGo struct {
	goroot     string
	jsList     string
	nativeList string
	testLog    string
	testErr    error
	failOn     string // "env", "jslist", "nativelist"
	calls      [][]string
	envs       [][]string
}

func (f *fakeGo) run(env []string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	f.envs = append(f.envs, env)
	isJS := len(env) > 0
	switch {
	case args[0] == "env":
		if f.failOn == "env" {
			return nil, errors.New("env boom")
		}
		return []byte(f.goroot + "\n"), nil
	case args[0] == "list" && isJS:
		if f.failOn == "jslist" {
			return nil, errors.New("jslist boom")
		}
		return []byte(f.jsList), nil
	case args[0] == "list":
		if f.failOn == "nativelist" {
			return nil, errors.New("nativelist boom")
		}
		return []byte(f.nativeList), nil
	case args[0] == "test":
		return []byte(f.testLog), f.testErr
	}
	return nil, errors.New("unexpected go " + strings.Join(args, " "))
}

func newJSWasmFixture(t *testing.T, files map[string]string) (dir string, read func(string) ([]byte, error)) {
	t.Helper()
	dir = t.TempDir()
	for name, src := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600))
	}
	return dir, os.ReadFile
}

const jsBridgeSrc = `//go:build js && wasm
package p
import "testing"
func TestA(t *testing.T) {}
// func TestGone(t *testing.T) {}
func TestB(t *testing.T) {}
`

// jsWasmFixture is a package whose bridge_test.go is js/wasm-only
// (TestA, TestB, and a commented-out TestGone) beside a native test.
type jsWasmFixture struct {
	bridge, native string
	read           func(string) ([]byte, error)
}

func newJSWasmPkg(t *testing.T) jsWasmFixture {
	t.Helper()
	dir, read := newJSWasmFixture(t, map[string]string{
		"bridge_test.go": jsBridgeSrc,
		"native_test.go": "package p\nimport \"testing\"\nfunc TestN(t *testing.T) {}\n",
	})
	return jsWasmFixture{
		bridge: filepath.Join(dir, "bridge_test.go"),
		native: filepath.Join(dir, "native_test.go"),
		read:   read,
	}
}

// fake returns a fakeGo whose go env and go list answers describe x.
func (x jsWasmFixture) fake(testLog string, testErr error) *fakeGo {
	return &fakeGo{
		goroot: "/go", jsList: x.bridge + "\n" + x.native + "\n", nativeList: x.native + "\n",
		testLog: testLog, testErr: testErr,
	}
}

func (x jsWasmFixture) deps(f *fakeGo, out *bytes.Buffer) jsWasmDeps {
	return jsWasmDeps{run: f.run, readFile: x.read, path: "/bin", out: out}
}

func TestRunJSWasmTestsWith(t *testing.T) {
	x := newJSWasmPkg(t)

	t.Run("all pass", func(t *testing.T) {
		f := x.fake(goTestJSON(t, append(result("pass", "TestA"), result("pass", "TestB")...)...), nil)
		var out bytes.Buffer
		require.NoError(t, runJSWasmTestsWith(x.deps(f, &out), "./p"))
		assert.Contains(t, out.String(), "--- PASS: TestB")

		require.Len(t, f.calls, 4)
		assert.Equal(t, []string{"env", "GOROOT"}, f.calls[0])
		assert.Equal(t, []string{"GOOS=js", "GOARCH=wasm"}, f.envs[1])
		assert.Equal(t, []string{"list", "-f", testFilesTemplate, "./p"}, f.calls[1])
		assert.Nil(t, f.envs[2])
		assert.Equal(t, []string{"list", "-e", "-f", testFilesTemplate, "./p"}, f.calls[2])
		assert.Equal(t, []string{
			"test", "-json",
			"-exec=env -i 'PATH=/bin' '/go/lib/wasm/go_js_wasm_exec'",
			"-run", "^(TestA|TestB)$",
			"./p",
		}, f.calls[3])
		assert.Equal(t, []string{"GOOS=js", "GOARCH=wasm"}, f.envs[3])
	})

	t.Run("skipped test is named", func(t *testing.T) {
		f := x.fake(goTestJSON(t, append(result("pass", "TestA"), result("skip", "TestB")...)...), nil)
		var out bytes.Buffer
		err := runJSWasmTestsWith(x.deps(f, &out), "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1 of 2")
		assert.Contains(t, err.Error(), "TestB")
		assert.NotContains(t, err.Error(), "TestA")
	})

	t.Run("failing go test fails and still prints the log", func(t *testing.T) {
		f := x.fake(goTestJSON(t, result("fail", "TestA")...), errors.New("exit status 1"))
		var out bytes.Buffer
		err := runJSWasmTestsWith(x.deps(f, &out), "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exit status 1")
		assert.Contains(t, out.String(), "--- FAIL: TestA")
	})
}

// TestRunJSWasmTestsWithErrors covers each step that can fail
// before or instead of go test.
func TestRunJSWasmTestsWithErrors(t *testing.T) {
	x := newJSWasmPkg(t)

	t.Run("no js-only files", func(t *testing.T) {
		f := x.fake("", nil)
		f.jsList = x.native + "\n"
		err := runJSWasmTestsWith(x.deps(f, &bytes.Buffer{}), "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no js/wasm-only test files")
	})

	t.Run("js-only files without tests", func(t *testing.T) {
		emptyDir, _ := newJSWasmFixture(t, map[string]string{
			"helper_test.go": "//go:build js && wasm\npackage p\n",
		})
		f := &fakeGo{goroot: "/go", jsList: filepath.Join(emptyDir, "helper_test.go") + "\n"}
		err := runJSWasmTestsWith(x.deps(f, &bytes.Buffer{}), "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no Test functions")
	})

	for _, step := range []string{"env", "jslist", "nativelist"} {
		t.Run("go "+step+" error", func(t *testing.T) {
			f := x.fake("", nil)
			f.failOn = step
			err := runJSWasmTestsWith(x.deps(f, &bytes.Buffer{}), "./p")
			require.Error(t, err)
			assert.Contains(t, err.Error(), step+" boom")
		})
	}

	t.Run("empty goroot", func(t *testing.T) {
		f := x.fake("", nil)
		f.goroot = ""
		err := runJSWasmTestsWith(x.deps(f, &bytes.Buffer{}), "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GOROOT")
	})
}

// TestRunJSWasmTestsWithDepFaults covers a failing file read, an
// unparsable file, and a PATH the -exec flag cannot quote.
func TestRunJSWasmTestsWithDepFaults(t *testing.T) {
	x := newJSWasmPkg(t)

	t.Run("unreadable file", func(t *testing.T) {
		f := x.fake("", nil)
		d := x.deps(f, &bytes.Buffer{})
		d.readFile = func(string) ([]byte, error) { return nil, errors.New("read boom") }
		err := runJSWasmTestsWith(d, "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read boom")
	})

	t.Run("unparsable file", func(t *testing.T) {
		f := x.fake("", nil)
		d := x.deps(f, &bytes.Buffer{})
		d.readFile = func(string) ([]byte, error) { return []byte("package"), nil }
		err := runJSWasmTestsWith(d, "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), x.bridge)
	})

	t.Run("unquotable PATH", func(t *testing.T) {
		f := x.fake("", nil)
		d := x.deps(f, &bytes.Buffer{})
		d.path = `'"`
		err := runJSWasmTestsWith(d, "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot quote")
	})
}

func TestSplitLines(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, splitLines([]byte("a\r\n\nb\n")))
	assert.Nil(t, splitLines(nil))
}

func TestOSGoRunner(t *testing.T) {
	out, err := osGoRunner(t.TempDir())(nil, "env", "GOROOT")
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(out)))

	out, err = osGoRunner(t.TempDir())([]string{"GOOS=js", "GOARCH=wasm"}, "env", "GOOS")
	require.NoError(t, err)
	assert.Equal(t, "js", strings.TrimSpace(string(out)))

	_, err = osGoRunner(t.TempDir())(nil, "no-such-go-subcommand")
	assert.Error(t, err)
}

// TestRunJSWasmTests drives the production wiring against a
// package with no js/wasm-only test files. It runs the real go env
// and go list but stops before go test, so it needs no Node.
func TestRunJSWasmTests(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	err = RunJSWasmTests(root, "./internal/release", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no js/wasm-only test files")
}

func TestJSOnlyFilesOf(t *testing.T) {
	f := &fakeGo{jsList: "/p/a_test.go\n/p/b_test.go\n", nativeList: "/p/b_test.go\n"}
	got, err := jsOnlyFilesOf(jsWasmDeps{run: f.run}, "./p")
	require.NoError(t, err)
	assert.Equal(t, []string{"/p/a_test.go"}, got)

	f = &fakeGo{jsList: "/p/b_test.go\n", nativeList: "/p/b_test.go\n"}
	_, err = jsOnlyFilesOf(jsWasmDeps{run: f.run}, "./p")
	assert.ErrorContains(t, err, "no js/wasm-only test files in ./p")
}

func TestTestsInFiles(t *testing.T) {
	read := func(name string) ([]byte, error) {
		return []byte("package p\nimport \"testing\"\nfunc Test" + name + "(t *testing.T) {}\n"), nil
	}
	got, err := testsInFiles(jsWasmDeps{readFile: read}, []string{"B", "A"})
	require.NoError(t, err)
	assert.Equal(t, []string{"TestB", "TestA"}, got)

	none := func(string) ([]byte, error) { return []byte("package p\n"), nil }
	_, err = testsInFiles(jsWasmDeps{readFile: none}, []string{"x_test.go"})
	assert.ErrorContains(t, err, "no Test functions in js/wasm-only files x_test.go")
}

func TestCheckAllPassed(t *testing.T) {
	assert.NoError(t, checkAllPassed([]string{"TestA"}, []string{"TestA", "TestExtra"}))
	assert.NoError(t, checkAllPassed(nil, nil))
	err := checkAllPassed([]string{"TestA", "TestB", "TestC"}, []string{"TestB"})
	assert.EqualError(t, err, "1 of 3 js/wasm tests passed; not passed: TestA, TestC")
}

func TestTestingImportName(t *testing.T) {
	tests := []struct {
		name, imports, want string
	}{
		{"default", `import "testing"`, "testing"},
		{"alias", `import tt "testing"`, "tt"},
		{"blank", `import _ "testing"`, ""},
		{"dot", `import . "testing"`, "."},
		{"absent", `import "fmt"`, ""},
		{"among others", "import (\n\"fmt\"\n\"testing\"\n)", "testing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parser.ParseFile(token.NewFileSet(), "", "package p\n"+tt.imports+"\n", parser.ImportsOnly)
			require.NoError(t, err)
			assert.Equal(t, tt.want, testingImportName(f))
		})
	}
}

func TestIsTestName(t *testing.T) {
	for name, want := range map[string]bool{
		"Test":      true,
		"TestA":     true,
		"Test_x":    true,
		"Test1":     true,
		"Testlower": false,
		"Tes":       false,
		"BenchTest": false,
	} {
		assert.Equal(t, want, isTestName(name), name)
	}
}

func TestTakesTestingT(t *testing.T) {
	tests := []struct {
		sig  string
		want bool
	}{
		{"func F(t *testing.T)", true},
		{"func F(*testing.T)", true},
		{"func F(t *testing.B)", false},
		{"func F(t testing.T)", false},
		{"func F(t *T)", false},
		{"func F(t *other.T)", false},
		{"func F(a, b *testing.T)", false},
		{"func F(t *testing.T, n int)", false},
		{"func F()", false},
	}
	for _, tt := range tests {
		t.Run(tt.sig, func(t *testing.T) {
			assert.Equal(t, tt.want, takesTestingT(parseFuncDecl(t, tt.sig), "testing"))
		})
	}

	// A dot import refers to the type as a bare T.
	assert.True(t, takesTestingT(parseFuncDecl(t, "func F(t *T)"), "."))
	assert.False(t, takesTestingT(parseFuncDecl(t, "func F(t *testing.T)"), "."))
	assert.False(t, takesTestingT(parseFuncDecl(t, "func F(m *M)"), "."))
}

// parseFuncDecl parses one function declaration with an empty body.
func parseFuncDecl(t *testing.T, sig string) *ast.FuncDecl {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "", "package p\n"+sig+" {}\n", 0)
	require.NoError(t, err)
	return f.Decls[0].(*ast.FuncDecl)
}
