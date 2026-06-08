package release

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// TestLayer names one rung of the test pyramid documented in
// docs/development/architecture/tests.md.
type TestLayer string

const (
	// LayerUnit is a test that lives next to its source and exercises
	// one function or method.
	LayerUnit TestLayer = "unit"
	// LayerIntegration is a test under internal/integration that
	// composes several packages against real fixtures (its contract
	// tests included).
	LayerIntegration TestLayer = "integration"
	// LayerE2E is a test that drives the built binary or otherwise
	// exercises the full process boundary.
	LayerE2E TestLayer = "e2e"
)

// testLayerOrder is the fixed display order — the pyramid base
// (unit) first, its apex (e2e) last.
var testLayerOrder = []TestLayer{LayerUnit, LayerIntegration, LayerE2E}

// TestCounts holds, per layer, the number of top-level test
// functions and the number of test cases. Functions answers "how
// many `func Test…` ran"; Cases counts each leaf subtest, so a
// fixture-driven layer like internal/integration reports far more
// cases than functions.
type TestCounts struct {
	Functions map[TestLayer]int
	Cases     map[TestLayer]int
}

// testKey identifies a top-level test function by its package import
// path and name.
type testKey struct {
	pkg  string
	name string
}

// testFuncRe matches a top-level test entry point at column zero:
// Test*, Example*, or Fuzz*. A method (`func (r R) TestX`) has a
// receiver between `func ` and the name, so it never matches —
// only package-level functions are recorded.
var testFuncRe = regexp.MustCompile(`^func ((?:Test|Example|Fuzz)[A-Za-z0-9_]*)\(`)

// testEvent is the subset of a `go test -json` event we read.
type testEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}

// SummarizeTestRun reads a `go test -json` event stream from r,
// echoing a terse (non-`-v`) reconstruction of the test log to
// logOut as it streams, and returns the per-layer test counts.
// srcRoot is the module root whose *_test.go files classify each
// executed test by file location.
func SummarizeTestRun(srcRoot string, r io.Reader, logOut io.Writer) (TestCounts, error) {
	layers, err := scanTestLayers(srcRoot)
	if err != nil {
		return TestCounts{}, err
	}

	// results records the layer of every (pkg,test) that reached a
	// terminal action; hasChild marks every test that owns a subtest
	// so leaf (case) counting can exclude container nodes.
	results := make(map[testKey]TestLayer)
	hasChild := make(map[testKey]bool)
	log := newTerseLog(logOut)

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev testEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			log.passthrough(line)
			continue
		}
		switch ev.Action {
		case "output":
			log.output(ev)
		case "pass", "fail", "skip":
			log.terminal(ev)
			if ev.Test != "" {
				recordResult(results, hasChild, layers, ev)
			}
		}
	}
	log.flush()
	if err := sc.Err(); err != nil {
		return TestCounts{}, fmt.Errorf("reading test json: %w", err)
	}
	return tallyCounts(results, hasChild), nil
}

// terseLog reconstructs a non-verbose `go test` console log from a
// -json stream. `go test -json` forces the test binary into verbose
// mode, so the stream carries every test's output even on success.
// Plain `go test` instead hides a passing test's output and shows it
// only on failure; terseLog reproduces that by buffering each test's
// output under its top-level test and emitting it only if that test
// fails. Package-level lines (the `ok pkg` / `FAIL pkg` results and
// build errors) pass through immediately.
type terseLog struct {
	out *bufio.Writer
	buf map[testKey][]string
}

func newTerseLog(w io.Writer) *terseLog {
	return &terseLog{out: bufio.NewWriter(w), buf: make(map[testKey][]string)}
}

// passthrough echoes a non-JSON line (e.g. a `go: downloading …`
// notice) verbatim, but drops a `{`-led line — a mangled event from
// `go test -json`'s rare cross-package write interleaving — so the
// log never shows half a JSON record.
func (l *terseLog) passthrough(line []byte) {
	if startsWithBrace(line) {
		return
	}
	_, _ = l.out.Write(line)
	_ = l.out.WriteByte('\n')
}

// output routes one "output" event: verbose scaffolding is dropped,
// a package-level line is shown now, and a test-attributed line is
// buffered under its top-level test until that test resolves.
func (l *terseLog) output(ev testEvent) {
	if !keepTerseOutput(ev.Output) {
		return
	}
	if ev.Test == "" {
		_, _ = l.out.WriteString(ev.Output)
		return
	}
	key := testKey{ev.Package, topLevelTest(ev.Test)}
	l.buf[key] = append(l.buf[key], ev.Output)
}

// terminal handles a pass/fail/skip event: when a top-level test
// resolves it flushes that test's buffered output on failure and
// drops it otherwise. Subtest events wait for their parent, whose
// buffer already holds their output.
func (l *terseLog) terminal(ev testEvent) {
	if ev.Test == "" || strings.Contains(ev.Test, "/") {
		return
	}
	key := testKey{ev.Package, ev.Test}
	if ev.Action == "fail" {
		for _, line := range l.buf[key] {
			_, _ = l.out.WriteString(line)
		}
	}
	delete(l.buf, key)
}

// flush emits any output still buffered for tests that never
// reached a terminal event — a panic that aborts the package leaves
// its output unresolved, and hiding it would lose the crash — then
// flushes the underlying writer.
func (l *terseLog) flush() {
	for _, lines := range l.buf {
		for _, line := range lines {
			_, _ = l.out.WriteString(line)
		}
	}
	l.buf = nil
	_ = l.out.Flush()
}

// topLevelTest returns the parent test name — everything before the
// first '/' — under which a test and all its subtests share one
// output buffer and one layer classification.
func topLevelTest(test string) string {
	if i := strings.IndexByte(test, '/'); i >= 0 {
		return test[:i]
	}
	return test
}

// recordResult files one terminal test result under its pyramid
// layer (defaulting to unit when the source scan did not classify
// it) and marks its ancestors as non-leaf.
func recordResult(
	results map[testKey]TestLayer,
	hasChild map[testKey]bool,
	layers map[testKey]TestLayer,
	ev testEvent,
) {
	layer, ok := layers[testKey{ev.Package, topLevelTest(ev.Test)}]
	if !ok {
		layer = LayerUnit
	}
	results[testKey{ev.Package, ev.Test}] = layer
	markAncestors(hasChild, ev.Package, ev.Test)
}

// tallyCounts reduces the recorded terminal results into per-layer
// counts: a result whose name has no '/' is a top-level function,
// and a result with no recorded child is a leaf case.
func tallyCounts(results map[testKey]TestLayer, hasChild map[testKey]bool) TestCounts {
	counts := TestCounts{
		Functions: make(map[TestLayer]int),
		Cases:     make(map[TestLayer]int),
	}
	for key, layer := range results {
		if !strings.Contains(key.name, "/") {
			counts.Functions[layer]++
		}
		if !hasChild[key] {
			counts.Cases[layer]++
		}
	}
	return counts
}

// startsWithBrace reports whether the first non-space byte of line
// is '{' — the opening of a (here, mangled) JSON event.
func startsWithBrace(line []byte) bool {
	for _, b := range line {
		if b == ' ' || b == '\t' {
			continue
		}
		return b == '{'
	}
	return false
}

// markAncestors flags every ancestor of test (TestA, TestA/b, …) as
// a non-leaf so case counting credits only the leaf subtests.
func markAncestors(hasChild map[testKey]bool, pkg, test string) {
	for i := 0; i < len(test); i++ {
		if test[i] == '/' {
			hasChild[testKey{pkg, test[:i]}] = true
		}
	}
}

// keepTerseOutput reports whether a `go test -json` output line
// belongs in a non-`-v` log. `-json` forces the test binary into
// verbose mode, so the stream carries the per-test scaffolding that
// a plain `go test` hides; dropping it reconstructs the terse log
// (package result lines and any failure detail) the CI job showed
// before this command was introduced.
func keepTerseOutput(s string) bool {
	t := strings.TrimLeft(s, " \t")
	switch {
	case strings.HasPrefix(t, "=== RUN"),
		strings.HasPrefix(t, "=== PAUSE"),
		strings.HasPrefix(t, "=== CONT"),
		strings.HasPrefix(t, "=== NAME"),
		strings.HasPrefix(t, "--- PASS:"),
		strings.HasPrefix(t, "--- SKIP:"),
		t == "PASS\n",
		t == "PASS":
		return false
	}
	return true
}

// scanTestLayers walks the module rooted at root and returns the
// pyramid layer of every top-level test function, keyed by import
// path and name. Nested modules (their own go.mod) and the
// directories the go tool never compiles — testdata, node_modules,
// and names beginning with "." or "_" — are skipped so the map
// matches what `go test ./...` actually runs.
func scanTestLayers(root string) (map[testKey]TestLayer, error) {
	module, err := readModulePath(root)
	if err != nil {
		return nil, err
	}
	layers := make(map[testKey]TestLayer)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if name == "testdata" || name == "node_modules" ||
				strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return fs.SkipDir
			}
			// A nested module is built by its own `go test` run, not by
			// the root `./...`, so its tests are out of this tally.
			if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		// path is always rooted at root, so trimming the prefix is
		// exact and avoids filepath.Rel's unreachable error branch.
		rel := strings.TrimPrefix(strings.TrimPrefix(path, root), string(os.PathSeparator))
		layer := layerForTestFile(rel)
		pkg := importPath(module, filepath.Dir(rel))
		names, scanErr := scanTestFuncNames(path)
		if scanErr != nil {
			return scanErr
		}
		for _, n := range names {
			layers[testKey{pkg, n}] = layer
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return layers, nil
}

// layerForTestFile classifies a repo-root-relative *_test.go path
// into a pyramid layer, matching the tree layout in
// docs/development/architecture/tests.md: internal/integration is the
// integration layer, an `*e2e*_test.go` file (or any file under an
// e2e/ directory) is the e2e layer, and everything else is a unit
// test living beside its source.
func layerForTestFile(rel string) TestLayer {
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "internal/integration/") {
		return LayerIntegration
	}
	segs := strings.Split(rel, "/")
	for _, s := range segs[:len(segs)-1] {
		if s == "e2e" {
			return LayerE2E
		}
	}
	if strings.Contains(strings.ToLower(segs[len(segs)-1]), "e2e") {
		return LayerE2E
	}
	return LayerUnit
}

// importPath joins a module path and a repo-relative directory into
// the package import path the `go test -json` Package field carries.
func importPath(module, relDir string) string {
	relDir = filepath.ToSlash(relDir)
	if relDir == "." || relDir == "" {
		return module
	}
	return module + "/" + relDir
}

// scanTestFuncNames returns the names of every top-level test entry
// point declared in a Go file.
func scanTestFuncNames(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only

	var names []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if m := testFuncRe.FindSubmatch(sc.Bytes()); m != nil {
			names = append(names, string(m[1]))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return names, nil
}

// readModulePath returns the module path from root/go.mod. It reads
// only the `module` line so it needs no module-file dependency and
// never promotes one from indirect to direct.
func readModulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("no module path in %s", filepath.Join(root, "go.mod"))
}

// RenderTestSummaryMarkdown formats counts as a GitHub step-summary
// section: one row per pyramid layer with its function and case
// counts, then a bold total row.
func RenderTestSummaryMarkdown(c TestCounts) string {
	totF := c.Functions[LayerUnit] + c.Functions[LayerIntegration] + c.Functions[LayerE2E]
	totC := c.Cases[LayerUnit] + c.Cases[LayerIntegration] + c.Cases[LayerE2E]
	var b strings.Builder
	b.WriteString("## Test suite\n\n")
	// Lead with a bold one-line headline so the counts read at a
	// glance at the top of the job-summary panel, above the table.
	fmt.Fprintf(&b, "**%s test functions** — %s unit · %s integration · %s end-to-end "+
		"· **%s cases** including subtests\n\n",
		thousands(totF),
		thousands(c.Functions[LayerUnit]),
		thousands(c.Functions[LayerIntegration]),
		thousands(c.Functions[LayerE2E]),
		thousands(totC))
	b.WriteString("| Layer | Test functions | Test cases |\n")
	b.WriteString("| --- | --: | --: |\n")
	for _, layer := range testLayerOrder {
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			layerLabel(layer), thousands(c.Functions[layer]), thousands(c.Cases[layer]))
	}
	fmt.Fprintf(&b, "| **Total** | **%s** | **%s** |\n", thousands(totF), thousands(totC))
	b.WriteString("\n*Test functions count top-level `func Test…`; " +
		"test cases count each leaf subtest (e.g. one per fixture), " +
		"so fixture-driven layers report more cases than functions.*\n")
	return b.String()
}

// RenderTestSummaryLine is the one-line console recap printed
// alongside the step-summary table.
func RenderTestSummaryLine(c TestCounts) string {
	return fmt.Sprintf(
		"test summary: %s functions, %s cases "+
			"(unit %s, integration %s, e2e %s functions)\n",
		thousands(c.Functions[LayerUnit]+c.Functions[LayerIntegration]+c.Functions[LayerE2E]),
		thousands(c.Cases[LayerUnit]+c.Cases[LayerIntegration]+c.Cases[LayerE2E]),
		thousands(c.Functions[LayerUnit]),
		thousands(c.Functions[LayerIntegration]),
		thousands(c.Functions[LayerE2E]),
	)
}

// layerLabel is the human-readable column label for a layer.
func layerLabel(l TestLayer) string {
	switch l {
	case LayerUnit:
		return "Unit"
	case LayerIntegration:
		return "Integration"
	case LayerE2E:
		return "End-to-end"
	default:
		return string(l)
	}
}

// thousands renders a non-negative count with comma group
// separators (1234 → "1,234").
func thousands(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
