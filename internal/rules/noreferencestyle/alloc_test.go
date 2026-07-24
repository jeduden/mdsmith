package noreferencestyle

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParagraphEndLine_ZeroAllocs confirms paragraphEndLine allocates nothing
// after changing its signature to accept pre-split lines instead of raw source.
func TestParagraphEndLine_ZeroAllocs(t *testing.T) {
	lines := [][]byte{
		[]byte("First line of paragraph."),
		[]byte("Second line of paragraph."),
		[]byte(""),
		[]byte("After blank."),
	}
	defLines := map[int]struct{}{}

	allocs := testing.AllocsPerRun(100, func() {
		paragraphEndLine(lines, 0, defLines)
	})
	assert.Equal(t, 0.0, allocs, "paragraphEndLine allocs: want 0, got %v", allocs)
}

// TestParagraphEndLine_Correctness verifies behavior is unchanged by the
// signature change from []byte source to [][]byte lines.
func TestParagraphEndLine_Correctness(t *testing.T) {
	lines := [][]byte{
		[]byte("First line."),
		[]byte("Second line."),
		[]byte(""),
		[]byte("After blank."),
	}
	defLines := map[int]struct{}{3: {}}

	// paragraph starting at line 0 ends before the blank (line 2 → index 2)
	end := paragraphEndLine(lines, 0, defLines)
	assert.Equal(t, 2, end, "should stop at blank line (index 2)")

	// paragraph starting at line 3 (index 3 is "After blank.")
	// defLines has line 3 (1-based), so it stops before consuming it
	lines2 := [][]byte{
		[]byte("Para start."),
	}
	end2 := paragraphEndLine(lines2, 0, map[int]struct{}{})
	assert.Equal(t, 1, end2, "single-line paragraph ends at 1")
}

// manyFootnoteRefsSource builds a document with n footnote references,
// each in its own paragraph, so scanFootnoteReferences/
// scanFootnoteDefinitions see n regex matches per call.
func manyFootnoteRefsSource(n int) []byte {
	var src []byte
	for i := 0; i < n; i++ {
		src = append(src, []byte(fmt.Sprintf("See [^ref%d] for details.\n\n", i))...)
	}
	for i := 0; i < n; i++ {
		src = append(src, []byte(fmt.Sprintf("[^ref%d]: some definition text here.\n", i))...)
	}
	return src
}

// TestScanFootnoteReferences_PresizedAllocs pins
// docs/development/high-performance-go.md's "pre-size slices" pattern:
// scanFootnoteReferences knows FindAllSubmatchIndex's match count up
// front, so out should be allocated once via
// make([]footnoteOccurrence, 0, len(matches)) instead of growing from a
// nil slice across repeated append calls.
func TestScanFootnoteReferences_PresizedAllocs(t *testing.T) {
	f, err := lint.NewFile("refs.md", manyFootnoteRefsSource(20))
	require.NoError(t, err)
	codeLines := map[int]struct{}{}

	allocs := testing.AllocsPerRun(50, func() {
		scanFootnoteReferences(f, codeLines, nil)
	})
	assert.LessOrEqualf(t, allocs, 64.0,
		"scanFootnoteReferences allocs regressed: got %v, want <= 64", allocs)
}

// TestScanFootnoteDefinitions_PresizedAllocs mirrors
// TestScanFootnoteReferences_PresizedAllocs for scanFootnoteDefinitions.
func TestScanFootnoteDefinitions_PresizedAllocs(t *testing.T) {
	f, err := lint.NewFile("defs.md", manyFootnoteRefsSource(20))
	require.NoError(t, err)
	codeLines := map[int]struct{}{}

	allocs := testing.AllocsPerRun(50, func() {
		scanFootnoteDefinitions(f, codeLines)
	})
	assert.LessOrEqualf(t, allocs, 43.0,
		"scanFootnoteDefinitions allocs regressed: got %v, want <= 43", allocs)
}

// TestScanFootnoteReferences_NoMatches_ReturnsNil and
// TestScanFootnoteDefinitions_NoMatches_ReturnsNil pin
// docs/development/high-performance-go.md's "return nil, not []T{}"
// convention for the zero-match case: make([]footnoteOccurrence, 0,
// len(matches)) with len(matches) == 0 still returns a non-nil empty
// slice in Go, so the pre-sizing fix checks len(out) == 0 after the
// filter loop and returns nil, matching the convention this same PR
// applies to headingincrement and tableformat.
func TestScanFootnoteReferences_NoMatches_ReturnsNil(t *testing.T) {
	f, err := lint.NewFile("clean.md", []byte("No footnotes here.\n"))
	require.NoError(t, err)
	out := scanFootnoteReferences(f, map[int]struct{}{}, nil)
	assert.Nil(t, out, "scanFootnoteReferences must return nil when there are no matches")
}

func TestScanFootnoteDefinitions_NoMatches_ReturnsNil(t *testing.T) {
	f, err := lint.NewFile("clean.md", []byte("No footnotes here.\n"))
	require.NoError(t, err)
	out := scanFootnoteDefinitions(f, map[int]struct{}{})
	assert.Nil(t, out, "scanFootnoteDefinitions must return nil when there are no matches")
}

// TestScanFootnoteReferences_AllFilteredOut_ReturnsNil and
// TestScanFootnoteDefinitions_AllFilteredOut_ReturnsNil cover the case
// the zero-match guard alone misses: matches is non-empty, but every
// entry is filtered out by isFootnoteDefinitionAt/codeLines/codeSpans
// (here, a footnote-shaped token inside a code span). The result must
// still be nil, not the pre-sized empty backing array. Caught by code
// review round 3.
func TestScanFootnoteReferences_AllFilteredOut_ReturnsNil(t *testing.T) {
	f, err := lint.NewFile("codespan.md", []byte("Use the `[^1]` token.\n"))
	require.NoError(t, err)
	codeSpans := f.CodeSpanLiteralRanges()
	out := scanFootnoteReferences(f, map[int]struct{}{}, codeSpans)
	assert.Nil(t, out, "scanFootnoteReferences must return nil when every match is filtered out")
}

func TestScanFootnoteDefinitions_AllFilteredOut_ReturnsNil(t *testing.T) {
	src := "Example:\n\n```text\n[^1]: not a real definition\n```\n"
	f, err := lint.NewFile("codeblock.md", []byte(src))
	require.NoError(t, err)
	codeLines := lint.CollectCodeBlockLines(f)
	out := scanFootnoteDefinitions(f, codeLines)
	assert.Nil(t, out, "scanFootnoteDefinitions must return nil when every match is filtered out")
}

// TestMayContainFootnote pins the cheap byte-needle gate that lets
// checkFootnotes skip both regex passes on a file that cannot possibly
// contain footnote syntax — the same "gate expensive analyzers behind
// a cheap pre-check" pattern nobareurls' mayContainURL already applies
// (docs/development/high-performance-go.md). Every footnoteRefRE/
// footnoteDefRE match requires the literal bytes "[^" somewhere in the
// source, so their absence rules out both regexes without running them.
func TestMayContainFootnote(t *testing.T) {
	assert.False(t, mayContainFootnote([]byte("No footnotes here, just prose.\n")))
	assert.True(t, mayContainFootnote([]byte("See [^note] for details.\n")))
	assert.True(t, mayContainFootnote([]byte("[^note]: a definition.\n")))
}

// footnoteCheckBudgetNs pins the gate's real effect: without it,
// footnoteRefRE and footnoteDefRE each run a full FindAllSubmatchIndex
// over the whole file on every Check call. Gated: ~1us (one
// bytes.Contains scan). Ungated: ~580us (two full regex passes over
// ~28KB). The budget keeps roughly the same ~15-20x headroom over the
// gated baseline that BenchmarkCheckCorpusSmall/Large use (see
// internal/engine/bench_test.go), well above measurement noise, while
// staying two orders of magnitude below the ungated cost.
const footnoteCheckBudgetNs = 50_000

// BenchmarkCheckFootnotes_NoNeedle exercises checkFootnotes on prose with
// no footnote syntax; benchstat-friendly (no assertion), consumed by
// TestCheckFootnotes_NoNeedleBudget below for the enforced gate.
func BenchmarkCheckFootnotes_NoNeedle(b *testing.B) {
	var src []byte
	for i := 0; i < 200; i++ {
		src = append(src, []byte(
			"This is a representative paragraph of prose with no "+
				"footnote syntax at all, just regular Markdown text.\n\n",
		)...)
	}
	f, err := lint.NewFile("prose.md", src)
	require.NoError(b, err)
	r := &Rule{}

	for i := 0; i < b.N; i++ {
		r.checkFootnotes(f)
	}
}

// TestCheckFootnotes_NoNeedleBudget pins the ns/op regression gate under
// a normal `go test` run. CI's check-bench/markdown-bench jobs only run
// `-bench` against internal/engine, pkg/markdown, internal/lsp, and
// cue/cuelite (see .github/workflows/ci.yml) — a plain
// BenchmarkCheckFootnotes_NoNeedle with an inline b.Fatalf would never
// execute in CI and the assertion would be dead code. testing.Benchmark
// runs the benchmark function programmatically so the assertion lands
// in a Test that `go test ./...` (and therefore CI) actually runs,
// matching paragraphstructure.TestCheckAllocBudget's rationale for its
// own Benchmark/Test pair.
func TestCheckFootnotes_NoNeedleBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("perf gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("perf gate skipped under -race; the race detector's " +
			"instrumentation overhead perturbs the ns/op measurement")
	}
	result := testing.Benchmark(BenchmarkCheckFootnotes_NoNeedle)
	perOp := float64(result.NsPerOp())
	t.Logf("checkFootnotes on a no-footnote file = %.0f ns/op (budget = %d)",
		perOp, footnoteCheckBudgetNs)
	if perOp > footnoteCheckBudgetNs {
		t.Fatalf("checkFootnotes on a no-footnote file: %.0f ns/op, budget = %d; "+
			"the mayContainFootnote gate may have been removed or bypassed",
			perOp, footnoteCheckBudgetNs)
	}
}
