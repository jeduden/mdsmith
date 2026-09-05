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

// BenchmarkCheckBlankBetween_NoBlockquote is a manual regression-detection
// tool for the sawBlockquote gate, not a CI-enforced gate: the gate is a
// pure CPU win (checkBlankBetween's AST/Layer-0 walk is skipped entirely,
// but it never allocated on a no-match walk either way — see
// markdownflavor's BenchmarkBuildAlertSkipMaps for the same rationale), so
// ns/op is too environment-sensitive for a hard b.Fatalf budget the way
// the allocs-based gates elsewhere in this codebase are. Run it manually
// with `-bench` and compare via benchstat before/after a change to
// checkBlankBetween's call site. Measured locally on a 200-section
// no-blockquote fixture: ~7500 ns/op before the gate landed, ~600 ns/op
// after — see the commit that introduced this benchmark.
func BenchmarkCheckBlankBetween_NoBlockquote(b *testing.B) {
	src := []byte(noBlockquoteDoc(200))
	f, err := lint.NewFile("prose.md", src)
	require.NoError(b, err)
	r := &Rule{}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}
