package blockquotewhitespace

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
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

// astModes exercises both File constructions the gate must handle
// identically: a fully parsed File (f.AST set, dispatches to
// checkBlankBetween) and a parse-skipped File (f.AST nil, dispatches
// to checkBlankBetweenLayer0). The gate itself runs once, ahead of
// that dispatch, so both paths share it — but only testing one path
// leaves the other unverified.
var astModes = []struct {
	name    string
	newFile func(t testing.TB, src string) *lint.File
}{
	{"AST", func(t testing.TB, src string) *lint.File {
		f, err := lint.NewFile("prose.md", []byte(src))
		require.NoError(t, err)
		return f
	}},
	{"NilAST", func(t testing.TB, src string) *lint.File {
		return lint.NewFileLines("prose.md", []byte(src))
	}},
}

// TestCheckBlankBetween_GateSkipsCleanDocument is the functional
// (non-perf) regression test for the sawBlockquote gate, now that
// nothing ns/op-based enforces it in CI (see the comment on
// BenchmarkCheckBlankBetween_NoBlockquote above): a representative
// multi-section document with no blockquote-marker line anywhere — the
// exact shape the gate targets — produces no MD028 diagnostics, on
// both the AST and nil-AST paths.
func TestCheckBlankBetween_GateSkipsCleanDocument(t *testing.T) {
	for _, mode := range astModes {
		t.Run(mode.name, func(t *testing.T) {
			f := mode.newFile(t, noBlockquoteDoc(50))
			r := &Rule{}
			assert.Empty(t, r.Check(f))
		})
	}
}

// TestCheckBlankBetween_GateDoesNotSuppressRealViolations is the other
// half of the functional pin: a document with the same no-blockquote
// prose the gate is meant to skip, PLUS two real blank-line-separated
// blockquotes appended, must still report MD028 on both the AST and
// nil-AST paths. A gate that is wrong in the unsafe direction —
// skipping the walk when it shouldn't — would silently swallow this
// diagnostic instead of merely costing more CPU, which is why this
// test matters more than the benchmark above: a perf regression is
// slow; a correctness regression here is silent.
func TestCheckBlankBetween_GateDoesNotSuppressRealViolations(t *testing.T) {
	src := noBlockquoteDoc(50) + "> first quote\n\n> second quote\n"
	for _, mode := range astModes {
		t.Run(mode.name, func(t *testing.T) {
			f := mode.newFile(t, src)
			r := &Rule{}
			diags := r.Check(f)
			require.Len(t, diags, 1)
			assert.Equal(t, "blank line between blockquotes", diags[0].Message)
		})
	}
}

// TestCheckBlankBetween_GateSeesListNestedBlockquote covers a
// blockquote whose marker line sits inside a list item, indented past
// the list marker — the gate's line scan skips leading whitespace
// before checking for '>', so list indentation must not hide the
// marker from it. Both a clean list (no MD028) and a violating one
// (two list-nested blockquotes split by a blank line) are checked, on
// both the AST and nil-AST paths.
func TestCheckBlankBetween_GateSeesListNestedBlockquote(t *testing.T) {
	clean := "- item\n\n  > a single list-nested quote\n"
	violating := "- item\n\n  > first list-nested quote\n\n  > second list-nested quote\n"
	for _, mode := range astModes {
		t.Run(mode.name+"/Clean", func(t *testing.T) {
			f := mode.newFile(t, clean)
			assert.Empty(t, (&Rule{}).Check(f))
		})
		t.Run(mode.name+"/Violating", func(t *testing.T) {
			f := mode.newFile(t, violating)
			diags := (&Rule{}).Check(f)
			require.Len(t, diags, 1)
			assert.Equal(t, "blank line between blockquotes", diags[0].Message)
		})
	}
}
