package blockquotewhitespace

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// noBlockquoteDoc builds a representative Markdown file with headings and
// prose paragraphs but not a single blockquote marker line — the shape of
// most real workspace files, since MDS059 is enabled by default and its
// MD028 half only ever fires on a document containing at least two
// sibling blockquotes.
func noBlockquoteDoc(sections int) string {
	var b strings.Builder
	b.WriteString("# Title\n\n")
	for i := 0; i < sections; i++ {
		b.WriteString("## Section\n\n")
		b.WriteString("A representative paragraph of prose with no ")
		b.WriteString("blockquote marker anywhere in the document.\n\n")
	}
	return b.String()
}

// BenchmarkCheckBlankBetween_NoBlockquote exercises checkBlankBetween's
// gate on a file with no '>' anywhere; benchstat-friendly (no assertion),
// consumed by TestCheckBlankBetween_NoBlockquoteBudget below for the
// enforced gate.
func BenchmarkCheckBlankBetween_NoBlockquote(b *testing.B) {
	src := []byte(noBlockquoteDoc(200))
	f, err := lint.NewFile("prose.md", src)
	require.NoError(b, err)
	r := &Rule{}

	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}

// blankBetweenNoBlockquoteBudgetNs pins the gate's real effect. Without
// the sawBlockquote gate, MDS059's MD028 half walks the full AST (or the
// Layer 0 BlockSpan list) on every default-enabled Check call, even
// though a file with zero blockquote-marker lines cannot contain the
// violation. The gate reuses the MD027 line scan already run in the same
// Check call, so the skip costs nothing extra when it fires. Gated:
// ~600ns on the 200-section fixture below. Ungated: ~7500ns (the full
// AST walk). The budget sits with headroom above the gated baseline
// while staying well under the ungated cost, so a regression that drops
// the gate trips this test.
const blankBetweenNoBlockquoteBudgetNs = 3_000

// TestCheckBlankBetween_NoBlockquoteBudget pins the ns/op regression gate
// under a normal `go test` run. See noreferencestyle's
// TestCheckFootnotes_NoNeedleBudget for why the assertion runs through
// testing.Benchmark inside a Test rather than a bare Benchmark: CI's
// bench jobs don't cover internal/rules, so a bare b.Fatalf here would
// never execute.
func TestCheckBlankBetween_NoBlockquoteBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("perf gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("perf gate skipped under -race; the race detector's " +
			"instrumentation overhead perturbs the ns/op measurement")
	}
	result := testing.Benchmark(BenchmarkCheckBlankBetween_NoBlockquote)
	perOp := float64(result.NsPerOp())
	t.Logf("Check on a 200-section no-blockquote file = %.0f ns/op (budget = %d)",
		perOp, blankBetweenNoBlockquoteBudgetNs)
	require.LessOrEqualf(t, perOp, float64(blankBetweenNoBlockquoteBudgetNs),
		"Check ns/op = %.0f exceeds budget %d: checkBlankBetween must be "+
			"gated behind the MD027 scan's sawBlockquote flag instead of "+
			"walking the AST/Layer-0 spans on every file",
		perOp, blankBetweenNoBlockquoteBudgetNs)
}
