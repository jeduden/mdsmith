package blockquotewhitespace

import (
	"bytes"
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
// checkBlankBetween's call site. On this 200-section no-blockquote
// fixture the gate cuts Check's time by about 10x: ~7,500 → ~600 ns/op
// when it landed, ~15,000 → ~1,500 ns/op on a loaded 4-core container;
// absolute figures vary by machine. The sentinel test
// TestCheckBlankBetween_GateSkipsWalkWithoutMarkerLine below is what
// fails in CI if the gate is removed.
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

// TestCheckBlankBetween_GateSkipsWalkWithoutMarkerLine is the sentinel
// that fails if the sawBlockquote gate is removed. On a consistent File
// the gate's only effect is CPU time, which CI cannot assert on, so
// this test builds an inconsistent one: its AST holds two sibling
// blockquotes split by a blank line (parsed from "> a\n\n> b\n"), but
// its Source and Lines are the same bytes with each '>' blanked out,
// so no line carries a blockquote marker. Byte offsets still line up,
// so the AST walk would resolve the gap and report MD028; the gate
// must skip that walk because the MD027 scan saw no '>' line. The gate
// sits ahead of the AST/nil-AST dispatch, so pinning it on the AST
// path covers both.
func TestCheckBlankBetween_GateSkipsWalkWithoutMarkerLine(t *testing.T) {
	quoted, err := lint.NewFile("quoted.md", []byte("> a\n\n> b\n"))
	require.NoError(t, err)
	f, err := lint.NewFile("gate.md", []byte("  a\n\n  b\n"))
	require.NoError(t, err)
	f.AST = quoted.AST

	assert.Empty(t, (&Rule{}).Check(f),
		"MD028 walk ran on a file with no '>' marker line: the sawBlockquote gate is gone")
}

// fixRegexOracle is the regex-only Fix prefix rewrite the byte gate in
// Fix replaced; the equivalence test pins the gate to it.
func fixRegexOracle(line []byte) []byte {
	prefix := reBlockquotePrefix.Find(line)
	if !reMultiSpace.Match(prefix) {
		return line
	}
	fixed := reMultiSpace.ReplaceAllLiteral(prefix, bqFixedSpace)
	content := line[len(prefix):]
	if len(content) == 0 {
		return bytes.TrimRight(fixed, " \t")
	}
	return append(append([]byte{}, fixed...), content...)
}

// TestFixGateMatchesRegexRewrite checks Fix's byte gate rewrites every
// line shape exactly as the regex-only path did.
func TestFixGateMatchesRegexRewrite(t *testing.T) {
	for _, line := range []string{
		"", "prose", "  indented prose", "> one", ">  two", ">   three",
		">\ttab", ">  \t", ">>  x", "> >  x", ">  >  x",
		"  >  x", ">", ">  ", "text >  not a marker", "> text >  later",
		">\t  x", "> \t x",
	} {
		f, err := lint.NewFile("t.md", []byte(line+"\n"))
		require.NoError(t, err)
		want := string(fixRegexOracle([]byte(line)))
		got := strings.TrimSuffix(string((&Rule{}).Fix(f)), "\n")
		assert.Equal(t, want, got, "line %q", line)
	}
}

// BenchmarkFix_ProseHeavy is Fix on a mostly-prose file: the byte gate
// should keep ordinary lines off the regex path.
func BenchmarkFix_ProseHeavy(b *testing.B) {
	src := []byte(strings.Repeat("plain prose line with some words in it\n", 2000) +
		">  quoted\n")
	f, err := lint.NewFile("t.md", src)
	if err != nil {
		b.Fatal(err)
	}
	r := &Rule{}
	b.ReportAllocs()
	for b.Loop() {
		_ = r.Fix(f)
	}
}
