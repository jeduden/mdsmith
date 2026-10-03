package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
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

// readTestJSONEvents is the stream TestTestJSONWriter_Write feeds:
// passes, a skip, a failure, subtests, a result whose own output had
// no trailing newline, and the package-level verdict.
func readTestJSONEvents() []testEvent {
	return slices.Concat(
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
}

func TestTestJSONWriter_Write(t *testing.T) {
	stream := "go: downloading example.com/m v1.0.0\n\n" + goTestJSON(t, readTestJSONEvents()...)

	// Feed the stream in 7-byte chunks so lines split across writes.
	var out bytes.Buffer
	w := &testJSONWriter{out: &out}
	for chunk := range slices.Chunk([]byte(stream), 7) {
		n, err := w.Write(chunk)
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
	}
	w.Flush()
	assert.Equal(t, []string{"TestA", "TestD"}, w.passed)
	log := out.String()
	assert.True(t, strings.HasPrefix(log, "go: downloading example.com/m v1.0.0\n"))
	assert.Contains(t, log, "partial--- PASS: TestD (0.00s)\n")
	assert.Contains(t, log, "--- SKIP: TestB (0.00s)\n")
	assert.True(t, strings.HasSuffix(log, "PASS\n"))

	// Output is written as soon as its line completes, not at the end.
	out.Reset()
	w = &testJSONWriter{out: &out}
	line := goTestJSON(t, testEvent{Action: "output", Test: "TestA", Output: "hi\n"})
	_, _ = w.Write([]byte(line[:5]))
	assert.Empty(t, out.String())
	_, _ = w.Write([]byte(line[5:]))
	assert.Equal(t, "hi\n", out.String())
}

func TestTestJSONWriter_Flush(t *testing.T) {
	var out bytes.Buffer
	w := &testJSONWriter{out: &out}
	w.Flush() // nothing buffered: no output
	assert.Empty(t, out.String())

	// A final event with no trailing newline is still decoded.
	line := strings.TrimSuffix(goTestJSON(t, result("pass", "TestA")...), "\n")
	_, _ = w.Write([]byte(line))
	assert.Nil(t, w.passed)
	w.Flush()
	assert.Equal(t, []string{"TestA"}, w.passed)
	assert.Equal(t, "--- PASS: TestA (0.00s)\n", out.String())
}

func TestTestJSONWriter_Line(t *testing.T) {
	var out bytes.Buffer
	w := &testJSONWriter{out: &out}
	w.line([]byte("  "))
	w.line([]byte("not json"))
	// A mangled event is dropped, as terseLog drops it, so the log
	// never shows half a JSON record.
	w.line([]byte(`{"Action":"output","Test":"TestZ`))
	w.line([]byte(`{"Action":"pass","Test":"TestX"}`))
	w.line([]byte(`{"Action":"pass","Test":"TestX/sub"}`))
	w.line([]byte(`{"Action":"skip","Test":"TestY"}`))
	assert.Equal(t, "not json\n", out.String())
	assert.Equal(t, []string{"TestX"}, w.passed)
}

func TestJSWasmDeps_Output(t *testing.T) {
	f := &fakeGo{goroot: "/go"}
	got, err := jsWasmDeps{run: f.run}.output(nil, "env", "GOROOT")
	require.NoError(t, err)
	assert.Equal(t, "/go\n", string(got))

	f.failOn = "env"
	_, err = jsWasmDeps{run: f.run}.output(nil, "env", "GOROOT")
	assert.ErrorContains(t, err, "env boom")
}

func TestJSWasmDeps_ExecFlag(t *testing.T) {
	t.Run("builds the flag from go env GOROOT", func(t *testing.T) {
		f := &fakeGo{goroot: " /go \n"}
		got, err := jsWasmDeps{run: f.run, path: "/bin"}.execFlag()
		require.NoError(t, err)
		assert.Equal(t, "env -i 'PATH=/bin' '/go/lib/wasm/go_js_wasm_exec'", got)
		assert.Equal(t, [][]string{{"env", "GOROOT"}}, f.calls)
		assert.Nil(t, f.envs[0], "go env runs with the native environment")
	})

	t.Run("go env failure", func(t *testing.T) {
		f := &fakeGo{failOn: "env"}
		_, err := jsWasmDeps{run: f.run, path: "/bin"}.execFlag()
		assert.ErrorContains(t, err, "go env GOROOT: env boom")
	})

	t.Run("empty GOROOT", func(t *testing.T) {
		f := &fakeGo{goroot: " "}
		_, err := jsWasmDeps{run: f.run, path: "/bin"}.execFlag()
		assert.ErrorContains(t, err, "go env GOROOT: empty output")
	})

	t.Run("unquotable path", func(t *testing.T) {
		f := &fakeGo{goroot: "/go"}
		_, err := jsWasmDeps{run: f.run, path: `'"`}.execFlag()
		assert.ErrorContains(t, err, "cannot quote")
	})
}

func TestRequireOnePackage(t *testing.T) {
	require.NoError(t, requireOnePackage("m", "./p", []string{"example.com/p"}))
	assert.EqualError(t, requireOnePackage("m", "./...", []string{"a", "b"}),
		"m needs exactly one package; ./... matches 2")
	assert.EqualError(t, requireOnePackage("m", "./none", nil),
		"m needs exactly one package; ./none matches 0")
}

func TestJSWasmDeps_GoTest(t *testing.T) {
	log := goTestJSON(t, slices.Concat(result("pass", "TestA"), result("skip", "TestB"))...)

	t.Run("names become the -run filter", func(t *testing.T) {
		f := &fakeGo{testLog: log}
		var out bytes.Buffer
		passed, err := jsWasmDeps{run: f.run, out: &out}.goTestNamed("./p", "X", []string{"TestA", "TestB"})
		require.NoError(t, err)
		assert.Equal(t, []string{"TestA"}, passed)
		assert.Equal(t, [][]string{{"test", "-json", "-exec=X", "-run", "^(TestA|TestB)$", "./p"}}, f.calls)
		assert.Equal(t, [][]string{{"GOOS=js", "GOARCH=wasm"}}, f.envs)
		assert.Contains(t, out.String(), "--- SKIP: TestB")
	})

	t.Run("no extra args runs the whole package", func(t *testing.T) {
		f := &fakeGo{testLog: log}
		_, err := jsWasmDeps{run: f.run, out: &bytes.Buffer{}}.goTest("./p", "X")
		require.NoError(t, err)
		assert.Equal(t, [][]string{{"test", "-json", "-exec=X", "./p"}}, f.calls)
	})

	t.Run("goTestNamed with no names fails without running go", func(t *testing.T) {
		// An empty -run filter would run the whole package, and an
		// empty want list would then pass checkAllPassed vacuously.
		for _, names := range [][]string{nil, {}} {
			f := &fakeGo{testLog: log}
			_, err := jsWasmDeps{run: f.run, out: &bytes.Buffer{}}.goTestNamed("./p", "X", names)
			require.EqualError(t, err, "no test names to run in ./p under js/wasm")
			assert.Empty(t, f.calls)
		}
	})

	t.Run("go test failure keeps the log", func(t *testing.T) {
		f := &fakeGo{testLog: log, testErr: errors.New("exit status 1")}
		var out bytes.Buffer
		passed, err := jsWasmDeps{run: f.run, out: &out}.goTest("./p", "X")
		assert.Nil(t, passed)
		assert.ErrorContains(t, err, "go test ./p under js/wasm: exit status 1")
		assert.Contains(t, out.String(), "--- PASS: TestA")
	})

	t.Run("a final line without a newline is flushed", func(t *testing.T) {
		f := &fakeGo{testLog: strings.TrimSuffix(goTestJSON(t, result("pass", "TestZ")...), "\n")}
		passed, err := jsWasmDeps{run: f.run, out: &bytes.Buffer{}}.goTest("./p", "X")
		require.NoError(t, err)
		assert.Equal(t, []string{"TestZ"}, passed)
	})
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

func (f *fakeGo) run(stdout io.Writer, env []string, args ...string) error {
	f.calls = append(f.calls, args)
	f.envs = append(f.envs, env)
	out, err := f.answer(len(env) > 0, args)
	_, _ = io.WriteString(stdout, out)
	return err
}

func (f *fakeGo) answer(isJS bool, args []string) (string, error) {
	switch {
	case args[0] == "env":
		if f.failOn == "env" {
			return "", errors.New("env boom")
		}
		return f.goroot + "\n", nil
	case args[0] == "list" && isJS:
		if f.failOn == "jslist" {
			return "", errors.New("jslist boom")
		}
		return f.jsList, nil
	case args[0] == "list":
		if f.failOn == "nativelist" {
			return "", errors.New("nativelist boom")
		}
		return f.nativeList, nil
	case args[0] == "test":
		return f.testLog, f.testErr
	}
	return "", errors.New("unexpected go " + strings.Join(args, " "))
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
		goroot:     "/go",
		jsList:     "#pkg example.com/p\n" + x.bridge + "\n" + x.native + "\n",
		nativeList: "#pkg example.com/p\n" + x.native + "\n",
		testLog:    testLog, testErr: testErr,
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
		f.jsList = "#pkg example.com/p\n" + x.native + "\n"
		err := runJSWasmTestsWith(x.deps(f, &bytes.Buffer{}), "./p")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no js/wasm-only test files")
	})

	t.Run("js-only files without tests", func(t *testing.T) {
		emptyDir, _ := newJSWasmFixture(t, map[string]string{
			"helper_test.go": "//go:build js && wasm\npackage p\n",
		})
		f := &fakeGo{goroot: "/go", jsList: "#pkg example.com/p\n" + filepath.Join(emptyDir, "helper_test.go") + "\n"}
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
	var out bytes.Buffer
	require.NoError(t, osGoRunner(t.TempDir())(&out, nil, "env", "GOROOT"))
	assert.NotEmpty(t, strings.TrimSpace(out.String()))

	out.Reset()
	require.NoError(t, osGoRunner(t.TempDir())(&out, []string{"GOOS=js", "GOARCH=wasm"}, "env", "GOOS"))
	assert.Equal(t, "js", strings.TrimSpace(out.String()))

	assert.Error(t, osGoRunner(t.TempDir())(io.Discard, nil, "no-such-go-subcommand"))
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
	f := &fakeGo{
		jsList:     "#pkg example.com/p\n/p/a_test.go\n/p/b_test.go\n",
		nativeList: "#pkg example.com/p\n/p/b_test.go\n",
	}
	got, err := jsOnlyFilesOf(jsWasmDeps{run: f.run}, "./p")
	require.NoError(t, err)
	assert.Equal(t, []string{"/p/a_test.go"}, got)

	f = &fakeGo{jsList: "#pkg example.com/p\n/p/b_test.go\n", nativeList: "#pkg example.com/p\n/p/b_test.go\n"}
	_, err = jsOnlyFilesOf(jsWasmDeps{run: f.run}, "./p")
	assert.ErrorContains(t, err, "no js/wasm-only test files in ./p")

	// Pass checks match bare test names, so a pattern that spans
	// packages could count another package's same-named test.
	f = &fakeGo{jsList: "#pkg example.com/a\n/a/x_test.go\n#pkg example.com/b\n/b/x_test.go\n"}
	_, err = jsOnlyFilesOf(jsWasmDeps{run: f.run}, "./...")
	assert.EqualError(t, err, "test-js-wasm needs exactly one package; ./... matches 2")

	f = &fakeGo{jsList: ""}
	_, err = jsOnlyFilesOf(jsWasmDeps{run: f.run}, "./none")
	assert.EqualError(t, err, "test-js-wasm needs exactly one package; ./none matches 0")
}

func TestListJSOnlyFiles(t *testing.T) {
	t.Run("extra flags reach the js/wasm list only", func(t *testing.T) {
		f := &fakeGo{
			jsList:     "#pkg example.com/p\n/p/a_test.go\n/p/b_test.go\n",
			nativeList: "#pkg example.com/p\n/p/b_test.go\n",
		}
		got, err := listJSOnlyFiles(jsWasmDeps{run: f.run}, "m", "./p", "-e")
		require.NoError(t, err)
		assert.Equal(t, []string{"/p/a_test.go"}, got)
		require.Len(t, f.calls, 2)
		assert.Equal(t, []string{"list", "-e", "-f", testFilesTemplate, "./p"}, f.calls[0])
		assert.Equal(t, jsWasmEnv, f.envs[0])
		assert.Equal(t, []string{"list", "-e", "-f", testFilesTemplate, "./p"}, f.calls[1])
		assert.Nil(t, f.envs[1])
	})

	t.Run("no js/wasm-only files is not an error", func(t *testing.T) {
		f := &fakeGo{jsList: "#pkg example.com/p\n/p/b_test.go\n", nativeList: "#pkg example.com/p\n/p/b_test.go\n"}
		got, err := listJSOnlyFiles(jsWasmDeps{run: f.run}, "m", "./p")
		require.NoError(t, err)
		assert.Nil(t, got)
		assert.Equal(t, []string{"list", "-f", testFilesTemplate, "./p"}, f.calls[0])
	})

	t.Run("mode names the command in the one-package error", func(t *testing.T) {
		f := &fakeGo{jsList: "#pkg example.com/a\n#pkg example.com/b\n"}
		_, err := listJSOnlyFiles(jsWasmDeps{run: f.run}, "my-mode", "./...")
		assert.EqualError(t, err, "my-mode needs exactly one package; ./... matches 2")
	})

	for _, step := range []string{"jslist", "nativelist"} {
		t.Run("go "+step+" error", func(t *testing.T) {
			f := &fakeGo{jsList: "#pkg example.com/p\n", failOn: step}
			_, err := listJSOnlyFiles(jsWasmDeps{run: f.run}, "m", "./p")
			assert.ErrorContains(t, err, step+" boom")
		})
	}
}

func TestSplitListOutput(t *testing.T) {
	pkgs, files := splitListOutput([]byte("#pkg a\n/a/x_test.go\n\n#pkg b\n/b/y_test.go\n"))
	assert.Equal(t, []string{"a", "b"}, pkgs)
	assert.Equal(t, []string{"/a/x_test.go", "/b/y_test.go"}, files)

	pkgs, files = splitListOutput(nil)
	assert.Nil(t, pkgs)
	assert.Nil(t, files)
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

func TestTestFuncsIn(t *testing.T) {
	read := func(name string) ([]byte, error) {
		return []byte("package p\nimport \"testing\"\nfunc Test" + name + "(t *testing.T) {}\n"), nil
	}
	got, err := testFuncsIn(jsWasmDeps{readFile: read}, []string{"B", "A"})
	require.NoError(t, err)
	assert.Equal(t, []string{"TestB", "TestA"}, got)

	// No Test function is not an error here, unlike testsInFiles.
	none := func(string) ([]byte, error) { return []byte("package p\n"), nil }
	got, err = testFuncsIn(jsWasmDeps{readFile: none}, []string{"x_test.go"})
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = testFuncsIn(jsWasmDeps{}, nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	bad := func(string) ([]byte, error) { return []byte("package"), nil }
	_, err = testFuncsIn(jsWasmDeps{readFile: bad}, []string{"x_test.go"})
	assert.ErrorContains(t, err, "x_test.go: ")
}

func TestCheckAllPassed(t *testing.T) {
	assert.NoError(t, checkAllPassed([]string{"TestA"}, []string{"TestA", "TestExtra"}))
	assert.NoError(t, checkAllPassed(nil, nil))
	err := checkAllPassed([]string{"TestA", "TestB", "TestC"}, []string{"TestB"})
	assert.EqualError(t, err, "1 of 3 js/wasm-only tests passed; not passed: TestA, TestC")
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

// onePkg is go list output naming exactly one package.
const onePkg = "#pkg example.com/p\n"

// TestRunJSWasmPackageWith covers the --all mode: the whole package
// runs under Node, with no -run filter, and any go test failure fails.
func TestRunJSWasmPackageWith(t *testing.T) {
	t.Run("pass runs the whole package", func(t *testing.T) {
		f := &fakeGo{goroot: "/go", jsList: onePkg, testLog: goTestJSON(t, result("pass", "TestA")...)}
		var out bytes.Buffer
		d := jsWasmDeps{run: f.run, path: "/bin", out: &out}
		require.NoError(t, runJSWasmPackageWith(d, "./p", false))
		assert.Contains(t, out.String(), "--- PASS: TestA")
		require.Len(t, f.calls, 4)
		assert.Equal(t, []string{"env", "GOROOT"}, f.calls[0])
		assert.Equal(t, []string{"list", "-e", "-f", testFilesTemplate, "./p"}, f.calls[1])
		assert.Equal(t, []string{"GOOS=js", "GOARCH=wasm"}, f.envs[1])
		assert.Equal(t, []string{"list", "-e", "-f", testFilesTemplate, "./p"}, f.calls[2])
		assert.Equal(t, []string{
			"test", "-json",
			"-exec=env -i 'PATH=/bin' '/go/lib/wasm/go_js_wasm_exec'",
			"./p",
		}, f.calls[3])
		assert.Equal(t, []string{"GOOS=js", "GOARCH=wasm"}, f.envs[3])
	})

	t.Run("a skipped test does not fail", func(t *testing.T) {
		log := goTestJSON(t, append(result("skip", "TestA"), result("pass", "TestB")...)...)
		f := &fakeGo{goroot: "/go", jsList: onePkg, testLog: log}
		d := jsWasmDeps{run: f.run, path: "/bin", out: &bytes.Buffer{}}
		require.NoError(t, runJSWasmPackageWith(d, "./p", false))
	})

	t.Run("no test passed fails", func(t *testing.T) {
		// go test exits 0 on a package with no test files, or whose
		// every test skipped: the gate must not pass vacuously.
		f := &fakeGo{goroot: "/go", jsList: onePkg, testLog: goTestJSON(t, result("skip", "TestA")...)}
		d := jsWasmDeps{run: f.run, path: "/bin", out: &bytes.Buffer{}}
		require.ErrorContains(t, runJSWasmPackageWith(d, "./p", false), "no test passed in ./p under js/wasm")
	})

	t.Run("go test failure fails and still prints the log", func(t *testing.T) {
		f := &fakeGo{
			goroot:  "/go",
			jsList:  onePkg,
			testLog: goTestJSON(t, result("fail", "TestA")...),
			testErr: errors.New("exit status 1"),
		}
		var out bytes.Buffer
		d := jsWasmDeps{run: f.run, path: "/bin", out: &out}
		err := runJSWasmPackageWith(d, "./p", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exit status 1")
		assert.Contains(t, out.String(), "--- FAIL: TestA")
	})
}

// TestRunJSWasmPackageWith_JSOnlySkips covers the --all guarantee that
// a skipped test from the js/wasm-only files fails by name, while a
// skipped test shared with the native build stays allowed.
func TestRunJSWasmPackageWith_JSOnlySkips(t *testing.T) {
	x := newJSWasmPkg(t)

	t.Run("a skipped js/wasm-only test fails by name", func(t *testing.T) {
		log := goTestJSON(t, slices.Concat(
			result("pass", "TestA"), result("skip", "TestB"), result("pass", "TestN"))...)
		f := x.fake(log, nil)
		err := runJSWasmPackageWith(x.deps(f, &bytes.Buffer{}), "./p", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not passed: TestB")
	})

	t.Run("all js/wasm-only tests passing succeeds, shared skip allowed", func(t *testing.T) {
		log := goTestJSON(t, slices.Concat(
			result("pass", "TestA"), result("pass", "TestB"), result("skip", "TestN"))...)
		f := x.fake(log, nil)
		require.NoError(t, runJSWasmPackageWith(x.deps(f, &bytes.Buffer{}), "./p", false))
	})

	t.Run("a package with no js/wasm-only files still runs", func(t *testing.T) {
		f := x.fake(goTestJSON(t, result("pass", "TestN")...), nil)
		f.jsList = "#pkg example.com/p\n" + x.native + "\n"
		require.NoError(t, runJSWasmPackageWith(x.deps(f, &bytes.Buffer{}), "./p", false))
	})

	t.Run("js/wasm-only files with no Test functions still run", func(t *testing.T) {
		// A js/wasm-only helper or TestMain file declares no test to
		// require by name; --all must still run the package.
		dir, _ := newJSWasmFixture(t, map[string]string{
			"helper_test.go": "//go:build js && wasm\npackage p\nimport \"testing\"\nfunc TestMain(m *testing.M) {}\n",
		})
		f := x.fake(goTestJSON(t, result("pass", "TestN")...), nil)
		f.jsList = "#pkg example.com/p\n" + filepath.Join(dir, "helper_test.go") + "\n" + x.native + "\n"
		require.NoError(t, runJSWasmPackageWith(x.deps(f, &bytes.Buffer{}), "./p", false))
		assert.Equal(t, "test", f.calls[len(f.calls)-1][0], "go test must run")
	})

	t.Run("an unreadable js/wasm-only file fails before go test", func(t *testing.T) {
		f := x.fake(goTestJSON(t, result("pass", "TestN")...), nil)
		d := x.deps(f, &bytes.Buffer{})
		d.readFile = func(string) ([]byte, error) { return nil, errors.New("read boom") }
		require.ErrorContains(t, runJSWasmPackageWith(d, "./p", false), "read boom")
		assert.Len(t, f.calls, 3, "go test must not run")
	})

	t.Run("a native go list failure fails before go test", func(t *testing.T) {
		f := x.fake("", nil)
		f.failOn = "nativelist"
		require.ErrorContains(t, runJSWasmPackageWith(x.deps(f, &bytes.Buffer{}), "./p", false),
			"go list (native) ./p: nativelist boom")
	})
}

// TestRunJSWasmPackageWith_RequireJSOnly covers --require-js-only:
// --all then also fails when the package has no js/wasm-only test.
func TestRunJSWasmPackageWith_RequireJSOnly(t *testing.T) {
	x := newJSWasmPkg(t)

	t.Run("requireJSOnly fails before go test with no js/wasm-only tests", func(t *testing.T) {
		// CI passes --require-js-only for internal/build so a lost or
		// renamed exec_other_test.go cannot pass the gate vacuously.
		f := x.fake(goTestJSON(t, result("pass", "TestN")...), nil)
		f.jsList = "#pkg example.com/p\n" + x.native + "\n"
		require.EqualError(t, runJSWasmPackageWith(x.deps(f, &bytes.Buffer{}), "./p", true),
			"no js/wasm-only Test functions in ./p")
		assert.Len(t, f.calls, 3, "go test must not run")
	})

	t.Run("requireJSOnly passes when the js/wasm-only tests pass", func(t *testing.T) {
		log := goTestJSON(t, slices.Concat(result("pass", "TestA"), result("pass", "TestB"))...)
		f := x.fake(log, nil)
		require.NoError(t, runJSWasmPackageWith(x.deps(f, &bytes.Buffer{}), "./p", true))
	})
}

// TestRunJSWasmPackageWith_SetupErrors covers the --all failures that
// come before go test: go env and the -exec flag.
func TestRunJSWasmPackageWith_SetupErrors(t *testing.T) {
	t.Run("go env failure", func(t *testing.T) {
		f := &fakeGo{failOn: "env"}
		d := jsWasmDeps{run: f.run, path: "/bin", out: &bytes.Buffer{}}
		require.ErrorContains(t, runJSWasmPackageWith(d, "./p", false), "go env GOROOT")
	})

	t.Run("empty GOROOT", func(t *testing.T) {
		f := &fakeGo{}
		d := jsWasmDeps{run: f.run, path: "/bin", out: &bytes.Buffer{}}
		require.ErrorContains(t, runJSWasmPackageWith(d, "./p", false), "empty output")
	})

	t.Run("unquotable path", func(t *testing.T) {
		f := &fakeGo{goroot: "/go"}
		d := jsWasmDeps{run: f.run, path: `a'"b`, out: &bytes.Buffer{}}
		require.ErrorContains(t, runJSWasmPackageWith(d, "./p", false), "cannot quote")
	})
}

// TestRunJSWasmPackageWith_OnePackage covers the --all check that
// <pkg> matches exactly one package before go test runs.
func TestRunJSWasmPackageWith_OnePackage(t *testing.T) {
	t.Run("a pattern matching several packages fails before go test", func(t *testing.T) {
		// The no-pass check sums passes across the whole run, so one
		// package with no pass would hide behind another's passes.
		f := &fakeGo{goroot: "/go", jsList: "#pkg example.com/a\n#pkg example.com/b\n"}
		d := jsWasmDeps{run: f.run, path: "/bin", out: &bytes.Buffer{}}
		require.EqualError(t, runJSWasmPackageWith(d, "./...", false),
			"test-js-wasm --all needs exactly one package; ./... matches 2")
		assert.Len(t, f.calls, 2, "go test must not run")
	})

	t.Run("a pattern matching no package fails", func(t *testing.T) {
		f := &fakeGo{goroot: "/go"}
		d := jsWasmDeps{run: f.run, path: "/bin", out: &bytes.Buffer{}}
		require.EqualError(t, runJSWasmPackageWith(d, "./none/...", false),
			"test-js-wasm --all needs exactly one package; ./none/... matches 0")
	})

	t.Run("go list failure", func(t *testing.T) {
		f := &fakeGo{goroot: "/go", failOn: "jslist"}
		d := jsWasmDeps{run: f.run, path: "/bin", out: &bytes.Buffer{}}
		require.ErrorContains(t, runJSWasmPackageWith(d, "./p", false), "go list (js/wasm) ./p: jslist boom")
	})
}

// TestOSJSWasmDeps checks the production wiring hands Node the process
// PATH and streams to the given writer.
func TestOSJSWasmDeps(t *testing.T) {
	t.Setenv("PATH", "/x/bin")
	var out bytes.Buffer
	d := osJSWasmDeps(t.TempDir(), &out)
	assert.Equal(t, "/x/bin", d.path)
	assert.Same(t, &out, d.out)
	require.NotNil(t, d.run)
	require.NotNil(t, d.readFile)
}

// TestRunJSWasmPackage drives the production wiring against a
// package that does not exist: go test fails before Node is needed.
func TestRunJSWasmPackage(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	err = RunJSWasmPackage(root, "./internal/does-not-exist", false, &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go test ./internal/does-not-exist under js/wasm")
}
