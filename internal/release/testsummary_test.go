package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayerForTestFile(t *testing.T) {
	cases := []struct {
		rel  string
		want TestLayer
	}{
		{"internal/integration/rules_test.go", LayerIntegration},
		{"internal/integration/sub/contract_test.go", LayerIntegration},
		{"cmd/mdsmith/e2e_test.go", LayerE2E},
		{"cmd/mdsmith/explain_e2e_test.go", LayerE2E},
		{"some/pkg/e2e/walk_test.go", LayerE2E},
		{"cmd/mdsmith/lsp_test.go", LayerUnit},
		{"cmd/mdsmith/main_unit_test.go", LayerUnit},
		{"internal/rules/foo/foo_test.go", LayerUnit},
		{"pkg/markdown/parse_test.go", LayerUnit},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, layerForTestFile(c.rel), c.rel)
	}
}

func TestImportPath(t *testing.T) {
	const m = "example.com/m"
	assert.Equal(t, m, importPath(m, "."))
	assert.Equal(t, m, importPath(m, ""))
	assert.Equal(t, m+"/cmd/app", importPath(m, "cmd/app"))
	assert.Equal(t, m+"/cmd/app", importPath(m, filepath.FromSlash("cmd/app")))
}

func TestReadModulePath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module example.com/m\n\ngo 1.24\n"), 0o644))
	got, err := readModulePath(root)
	require.NoError(t, err)
	assert.Equal(t, "example.com/m", got)

	// Missing go.mod is an error.
	_, err = readModulePath(t.TempDir())
	assert.Error(t, err)

	// A go.mod without a module line is an error.
	noMod := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(noMod, "go.mod"),
		[]byte("go 1.24\n"), 0o644))
	_, err = readModulePath(noMod)
	assert.Error(t, err)
}

func TestScanTestFuncNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x_test.go")
	src := "package x\n\n" +
		"import \"testing\"\n\n" +
		"func TestAlpha(t *testing.T) {}\n" +
		"func ExampleBeta() {}\n" +
		"func FuzzGamma(f *testing.F) {}\n" +
		"func (r recv) TestMethodNotCounted() {}\n" +
		"func helperNotCounted() {}\n" +
		"func Outer() {\n\tfunc() { _ = \"not a TestNested\" }()\n}\n"
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	names, err := scanTestFuncNames(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"TestAlpha", "ExampleBeta", "FuzzGamma"}, names)

	_, err = scanTestFuncNames(filepath.Join(dir, "missing_test.go"))
	assert.Error(t, err)
}

// writeTestModule lays down a small module tree exercising every
// classification and skip rule, and returns its root.
func writeTestModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	write("go.mod", "module example.com/m\n\ngo 1.24\n")
	write("foo/foo_test.go", "package foo\nimport \"testing\"\nfunc TestU(t *testing.T) {}\n")
	write("internal/integration/it_test.go",
		"package integration\nimport \"testing\"\nfunc TestIt(t *testing.T) {}\n")
	write("cmd/app/e2e_test.go", "package app\nimport \"testing\"\nfunc TestE2EThing(t *testing.T) {}\n")
	// Skipped: testdata, nested module, dot-dir.
	write("foo/testdata/ignored_test.go", "package x\nfunc TestSkippedTestdata() {}\n")
	write("sub/go.mod", "module example.com/sub\n")
	write("sub/nested_test.go", "package sub\nfunc TestSkippedNested() {}\n")
	write(".hidden/h_test.go", "package h\nfunc TestSkippedHidden() {}\n")
	return root
}

func TestScanTestLayers(t *testing.T) {
	root := writeTestModule(t)
	layers, err := scanTestLayers(root)
	require.NoError(t, err)

	assert.Equal(t, LayerUnit, layers[testKey{"example.com/m/foo", "TestU"}])
	assert.Equal(t, LayerIntegration,
		layers[testKey{"example.com/m/internal/integration", "TestIt"}])
	assert.Equal(t, LayerE2E, layers[testKey{"example.com/m/cmd/app", "TestE2EThing"}])

	// The three skip rules: testdata dir, nested module, dot-dir.
	for _, name := range []string{"TestSkippedTestdata", "TestSkippedNested", "TestSkippedHidden"} {
		for key := range layers {
			assert.NotEqual(t, name, key.name, "%s should have been skipped", name)
		}
	}

	// Missing go.mod surfaces as an error.
	_, err = scanTestLayers(t.TempDir())
	assert.Error(t, err)
}

// evLine marshals one go-test JSON event line.
func evLine(t *testing.T, action, pkg, test, output string) string {
	t.Helper()
	b, err := json.Marshal(testEvent{Action: action, Package: pkg, Test: test, Output: output})
	require.NoError(t, err)
	return string(b) + "\n"
}

func TestSummarizeTestRun(t *testing.T) {
	root := writeTestModule(t)

	var stream strings.Builder
	// Unit: one function, no subtests.
	stream.WriteString(evLine(t, "run", "example.com/m/foo", "TestU", ""))
	stream.WriteString(evLine(t, "output", "example.com/m/foo", "TestU", "=== RUN   TestU\n"))
	stream.WriteString(evLine(t, "output", "example.com/m/foo", "TestU", "    hello from log\n"))
	stream.WriteString(evLine(t, "output", "example.com/m/foo", "TestU", "--- PASS: TestU (0.00s)\n"))
	stream.WriteString(evLine(t, "pass", "example.com/m/foo", "TestU", ""))
	stream.WriteString(evLine(t, "output", "example.com/m/foo", "", "ok  \texample.com/m/foo\t0.10s\n"))
	// Integration: one function fanning out to two leaf subtests.
	ip := "example.com/m/internal/integration"
	stream.WriteString(evLine(t, "run", ip, "TestIt", ""))
	stream.WriteString(evLine(t, "pass", ip, "TestIt/A", ""))
	stream.WriteString(evLine(t, "skip", ip, "TestIt/B", ""))
	stream.WriteString(evLine(t, "pass", ip, "TestIt", ""))
	// E2E: one function.
	stream.WriteString(evLine(t, "pass", "example.com/m/cmd/app", "TestE2EThing", ""))
	// A package-level pass (empty Test) is skipped, not counted.
	stream.WriteString(evLine(t, "pass", "example.com/m/foo", "", ""))
	// A mangled JSON event (drop from log, do not count).
	stream.WriteString(`{"Time":"x","Action":"output","Package":"example.com/m/foo","Test":"TestU` + "\n")
	// A genuine non-JSON notice (echo to log).
	stream.WriteString("go: downloading example.com/x v1.0.0\n")
	// A blank line (ignored).
	stream.WriteString("\n")

	var log bytes.Buffer
	counts, err := SummarizeTestRun(root, strings.NewReader(stream.String()), &log)
	require.NoError(t, err)

	assert.Equal(t, 1, counts.Functions[LayerUnit])
	assert.Equal(t, 1, counts.Functions[LayerIntegration])
	assert.Equal(t, 1, counts.Functions[LayerE2E])
	assert.Equal(t, 1, counts.Cases[LayerUnit])
	assert.Equal(t, 2, counts.Cases[LayerIntegration], "two leaf subtests")
	assert.Equal(t, 1, counts.Cases[LayerE2E])

	logStr := log.String()
	assert.Contains(t, logStr, "ok  \texample.com/m/foo")
	assert.Contains(t, logStr, "go: downloading example.com/x")
	assert.NotContains(t, logStr, "hello from log", "a passing test's output is hidden")
	assert.NotContains(t, logStr, "=== RUN")
	assert.NotContains(t, logStr, "--- PASS:")
	assert.NotContains(t, logStr, `"Action":"output"`, "mangled event must not leak")
}

func TestSummarizeTestRunUnclassifiedFallsToUnit(t *testing.T) {
	root := writeTestModule(t)
	// A package/test the source scan never saw defaults to unit.
	stream := evLine(t, "pass", "example.com/m/ghost", "TestPhantom", "")
	counts, err := SummarizeTestRun(root, strings.NewReader(stream), &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Functions[LayerUnit])
	assert.Equal(t, 1, counts.Cases[LayerUnit])
}

func TestSummarizeTestRunErrorsWithoutModule(t *testing.T) {
	_, err := SummarizeTestRun(t.TempDir(), strings.NewReader(""), &bytes.Buffer{})
	assert.Error(t, err)
}

// TestSummarizeTestRunLogHidesPassShowsFail pins the terse-log
// contract: a passing test's output (including a noisy multi-line
// dump like a CLI usage block) is hidden, while a failing test's
// output — and a failing subtest's, via the parent — is shown.
func TestSummarizeTestRunLogHidesPassShowsFail(t *testing.T) {
	root := writeTestModule(t)
	var s strings.Builder
	// Passing test prints a usage-like block — must be hidden.
	s.WriteString(evLine(t, "output", "example.com/m/foo", "TestU", "    Usage: do the thing\n"))
	s.WriteString(evLine(t, "output", "example.com/m/foo", "TestU", "    build-website [--no-fix]\n"))
	s.WriteString(evLine(t, "pass", "example.com/m/foo", "TestU", ""))
	// Failing test with a detail line and a failing subtest — shown.
	s.WriteString(evLine(t, "output", "example.com/m/foo", "TestBoom", "    foo_test.go:9: boom detail\n"))
	s.WriteString(evLine(t, "output", "example.com/m/foo", "TestBoom/case", "    sub detail\n"))
	s.WriteString(evLine(t, "fail", "example.com/m/foo", "TestBoom/case", ""))
	s.WriteString(evLine(t, "output", "example.com/m/foo", "TestBoom", "--- FAIL: TestBoom (0.00s)\n"))
	s.WriteString(evLine(t, "fail", "example.com/m/foo", "TestBoom", ""))
	// Package result line — always shown.
	s.WriteString(evLine(t, "output", "example.com/m/foo", "", "FAIL\texample.com/m/foo\t0.01s\n"))

	var log bytes.Buffer
	_, err := SummarizeTestRun(root, strings.NewReader(s.String()), &log)
	require.NoError(t, err)
	out := log.String()
	assert.NotContains(t, out, "Usage: do the thing", "passing test output hidden")
	assert.NotContains(t, out, "build-website", "passing test output hidden")
	assert.Contains(t, out, "boom detail", "failing test output shown")
	assert.Contains(t, out, "sub detail", "failing subtest output shown via parent")
	assert.Contains(t, out, "--- FAIL: TestBoom")
	assert.Contains(t, out, "FAIL\texample.com/m/foo")
}

// TestSummarizeTestRunLogFlushesUnresolved covers the flush of output
// for a test that never reaches a terminal event — a panic aborts the
// package, and that crash must not be swallowed.
func TestSummarizeTestRunLogFlushesUnresolved(t *testing.T) {
	root := writeTestModule(t)
	s := evLine(t, "output", "example.com/m/foo", "TestPanic", "panic: boom\n")
	var log bytes.Buffer
	_, err := SummarizeTestRun(root, strings.NewReader(s), &log)
	require.NoError(t, err)
	assert.Contains(t, log.String(), "panic: boom")
}

// errReader fails on the first read so the scanner surfaces a
// non-EOF error.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestSummarizeTestRunScannerError(t *testing.T) {
	_, err := SummarizeTestRun(writeTestModule(t), errReader{}, &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading test json")
}

// TestScanTestLayersScanError drives the propagation of a per-file
// scan failure: a _test.go line longer than scanTestFuncNames'
// 1 MiB token cap makes the scanner return bufio.ErrTooLong, which
// must surface as an error from scanTestLayers.
func TestScanTestLayersScanError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module example.com/m\n"), 0o644))
	huge := append([]byte("// "), bytes.Repeat([]byte("a"), 2*1024*1024)...)
	require.NoError(t, os.WriteFile(filepath.Join(root, "huge_test.go"), huge, 0o644))

	_, err := scanTestLayers(root)
	require.Error(t, err)

	// The same oversized line trips scanTestFuncNames directly.
	_, err = scanTestFuncNames(filepath.Join(root, "huge_test.go"))
	assert.Error(t, err)
}

func TestTallyCounts(t *testing.T) {
	results := map[testKey]TestLayer{
		{"p", "TestSolo"}:  LayerUnit, // function + leaf
		{"p", "TestPar"}:   LayerE2E,  // function, has child below
		{"p", "TestPar/a"}: LayerE2E,  // leaf only
		{"p", "TestPar/b"}: LayerE2E,  // leaf only
		{"q", "TestI"}:     LayerIntegration,
	}
	hasChild := map[testKey]bool{}
	for key := range results {
		markAncestors(hasChild, key.pkg, key.name)
	}
	got := tallyCounts(results, hasChild)
	assert.Equal(t, 1, got.Functions[LayerUnit])
	assert.Equal(t, 1, got.Functions[LayerE2E])         // TestPar
	assert.Equal(t, 1, got.Functions[LayerIntegration]) // TestI
	assert.Equal(t, 1, got.Cases[LayerUnit])            // TestSolo
	assert.Equal(t, 2, got.Cases[LayerE2E])             // TestPar/a, /b
	assert.Equal(t, 1, got.Cases[LayerIntegration])     // TestI
}

func TestRenderTestSummaryMarkdown(t *testing.T) {
	c := TestCounts{
		Functions: map[TestLayer]int{LayerUnit: 7919, LayerIntegration: 28, LayerE2E: 284},
		Cases:     map[TestLayer]int{LayerUnit: 9088, LayerIntegration: 629, LayerE2E: 288},
	}
	got := RenderTestSummaryMarkdown(c)
	assert.Contains(t, got, "## Test suite")
	// Bold one-line headline leads the panel, above the table.
	assert.Contains(t, got,
		"**8,231 test functions** — 7,919 unit · 28 integration · 284 end-to-end · **10,005 cases** including subtests")
	assert.Less(t, strings.Index(got, "8,231 test functions"), strings.Index(got, "| Layer |"),
		"headline appears above the table")
	assert.Contains(t, got, "| Unit | 7,919 | 9,088 |")
	assert.Contains(t, got, "| Integration | 28 | 629 |")
	assert.Contains(t, got, "| End-to-end | 284 | 288 |")
	assert.Contains(t, got, "| **Total** | **8,231** | **10,005** |")
	// Layers render base-first.
	assert.Less(t, strings.Index(got, "| Unit |"), strings.Index(got, "| Integration |"))
	assert.Less(t, strings.Index(got, "| Integration |"), strings.Index(got, "| End-to-end |"))
}

func TestRenderTestSummaryLine(t *testing.T) {
	c := TestCounts{
		Functions: map[TestLayer]int{LayerUnit: 10, LayerIntegration: 2, LayerE2E: 3},
		Cases:     map[TestLayer]int{LayerUnit: 12, LayerIntegration: 9, LayerE2E: 3},
	}
	got := RenderTestSummaryLine(c)
	assert.Equal(t,
		"test summary: 15 functions, 24 cases (unit 10, integration 2, e2e 3 functions)\n",
		got)
}

func TestKeepTerseOutput(t *testing.T) {
	drop := []string{
		"=== RUN   TestX\n",
		"=== PAUSE TestX\n",
		"=== CONT  TestX\n",
		"=== NAME  TestX\n",
		"--- PASS: TestX (0.00s)\n",
		"    --- PASS: TestX/sub (0.00s)\n",
		"--- SKIP: TestX (0.00s)\n",
		"PASS\n",
		"PASS",
	}
	for _, s := range drop {
		assert.False(t, keepTerseOutput(s), "%q", s)
	}
	keep := []string{
		"--- FAIL: TestY (0.01s)\n",
		"ok  \tpkg\t0.10s\n",
		"FAIL\tpkg\t0.10s\n",
		"?   \tpkg\t[no test files]\n",
		"    some_test.go:42: boom\n",
	}
	for _, s := range keep {
		assert.True(t, keepTerseOutput(s), "%q", s)
	}
}

func TestStartsWithBrace(t *testing.T) {
	assert.True(t, startsWithBrace([]byte(`{"a":1}`)))
	assert.True(t, startsWithBrace([]byte("  \t{x")))
	assert.False(t, startsWithBrace([]byte("go: downloading x")))
	assert.False(t, startsWithBrace([]byte("")))
	assert.False(t, startsWithBrace([]byte("   ")))
}

func TestMarkAncestors(t *testing.T) {
	hasChild := map[testKey]bool{}
	markAncestors(hasChild, "pkg", "TestA/b/c")
	assert.True(t, hasChild[testKey{"pkg", "TestA"}])
	assert.True(t, hasChild[testKey{"pkg", "TestA/b"}])
	assert.False(t, hasChild[testKey{"pkg", "TestA/b/c"}], "the leaf is not its own ancestor")

	// A top-level test with no subtests marks nothing.
	markAncestors(hasChild, "pkg", "TestSolo")
	assert.False(t, hasChild[testKey{"pkg", "TestSolo"}])
}

func TestThousands(t *testing.T) {
	cases := map[int]string{
		0: "0", 5: "5", 42: "42", 999: "999",
		1000: "1,000", 12345: "12,345", 100000: "100,000",
		1000000: "1,000,000",
	}
	for n, want := range cases {
		assert.Equal(t, want, thousands(n), "%d", n)
	}
}

func TestLayerLabel(t *testing.T) {
	assert.Equal(t, "Unit", layerLabel(LayerUnit))
	assert.Equal(t, "Integration", layerLabel(LayerIntegration))
	assert.Equal(t, "End-to-end", layerLabel(LayerE2E))
	assert.Equal(t, "other", layerLabel(TestLayer("other")))
}
