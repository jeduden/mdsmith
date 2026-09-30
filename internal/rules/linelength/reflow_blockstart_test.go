package linelength

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// greedyWrap is plain greedy packing with no block-start guard: each
// line takes every token that fits, and a token wider than width sits
// on its own line. Tests compare wrapTokens with it to prove a case
// reaches the guard (the layouts differ) or stays clear of it (they
// match).
func greedyWrap(tokens []string, width int) []string {
	var lines []string
	cur := tokens[0]
	for _, tok := range tokens[1:] {
		if utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(tok) <= width {
			cur += " " + tok
			continue
		}
		lines = append(lines, cur)
		cur = tok
	}
	return append(lines, cur)
}

// singleParagraph reports whether lines, joined as one Markdown block,
// parse as exactly one paragraph that keeps every line. It asks both the
// canonical parser and the flavor parser, which adds GFM tables,
// footnotes and definition lists, so it does not share code with the
// guard.
func singleParagraph(t *testing.T, lines []string) bool {
	t.Helper()
	src := []byte(strings.Join(lines, "\n") + "\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	if !soleParagraph(f.AST, len(lines)) {
		return false
	}
	var ok bool
	flavor.WithSharedParser(func(p parser.Parser) {
		ok = soleParagraph(p.Parse(text.NewReader(src)), len(lines))
	})
	return ok
}

// soleParagraph reports whether doc holds one paragraph of n lines and
// nothing else.
func soleParagraph(doc ast.Node, n int) bool {
	first := doc.FirstChild()
	return first != nil && first.NextSibling() == nil &&
		first.Kind() == ast.KindParagraph && first.Lines().Len() == n
}

var noGlue = func(string) bool { return false }

// blockStartCases hold one case per kind of block start that can
// interrupt a paragraph. Each width makes plain greedy wrapping put the
// marker at the start of a line; want is the guarded layout.
var blockStartCases = []struct {
	name   string
	tokens []string
	width  int
	want   []string
}{
	{"atx heading", []string{"aaaa", "bbbb", "#", "cc"}, 10,
		[]string{"aaaa", "bbbb # cc"}},
	{"atx heading level 6", []string{"aaaa", "bbbb", "######", "c"}, 14,
		[]string{"aaaa", "bbbb ###### c"}},
	{"thematic break", []string{"aaaa", "bbbb", "***"}, 10,
		[]string{"aaaa", "bbbb ***"}},
	{"spaced thematic break", []string{"aaaa", "bbbb", "_", "_", "_"}, 10,
		[]string{"aaaa bbbb", "_ _", "_"}},
	{"dash thematic break", []string{"aaaa", "bbbb", "--", "-"}, 10,
		[]string{"aaaa", "bbbb -- -"}},
	{"backtick fence with info string", []string{"aaaa", "bbbb", "```go", "cc"}, 10,
		[]string{"aaaa", "bbbb ```go", "cc"}},
	{"tilde fence", []string{"aaaa", "bbbb", "~~~", "cc"}, 10,
		[]string{"aaaa", "bbbb ~~~", "cc"}},
	{"tilde fence with info string", []string{"aaaa", "bbbb", "~~~go", "cc"}, 10,
		[]string{"aaaa", "bbbb ~~~go", "cc"}},
	{"block quote", []string{"aaaa", "bbbb", ">", "cc"}, 10,
		[]string{"aaaa", "bbbb > cc"}},
	{"dash bullet", []string{"aaaa", "bbbb", "-", "cc"}, 10,
		[]string{"aaaa", "bbbb - cc"}},
	// An empty list item cannot interrupt a paragraph, so a lone
	// marker line is safe and the layout keeps the first line full.
	{"plus bullet", []string{"aaaa", "bbbb", "+", "cc"}, 10,
		[]string{"aaaa bbbb", "+", "cc"}},
	{"star bullet", []string{"aaaa", "bbbb", "*", "cc"}, 10,
		[]string{"aaaa bbbb", "*", "cc"}},
	{"ordered item 1.", []string{"aaaa", "bbbb", "1.", "cc"}, 10,
		[]string{"aaaa bbbb", "1.", "cc"}},
	{"ordered item 1)", []string{"aaaa", "bbbb", "1)", "cc"}, 10,
		[]string{"aaaa bbbb", "1)", "cc"}},
	{"setext equals underline", []string{"aaaa", "bbbb", "="}, 10,
		[]string{"aaaa", "bbbb ="}},
	{"setext dash underline", []string{"aaaa", "bbbb", "--"}, 10,
		[]string{"aaaa", "bbbb --"}},
	// HTML blocks of types 1-6, in type order.
	{"html type 1 raw tag", []string{"aaaa", "bbbb", "<pre", "cc"}, 10,
		[]string{"aaaa", "bbbb <pre", "cc"}},
	{"html comment", []string{"aaaa", "bbbb", "<!--", "cc"}, 10,
		[]string{"aaaa", "bbbb <!--", "cc"}},
	{"processing instruction", []string{"aaaa", "bbbb", "<?pi", "cc"}, 10,
		[]string{"aaaa", "bbbb <?pi", "cc"}},
	{"html declaration", []string{"aaaa", "bbbb", "<!X", "cc"}, 10,
		[]string{"aaaa", "bbbb <!X", "cc"}},
	{"html cdata", []string{"aaaa", "bbbb", "<![CDATA[", "c"}, 14,
		[]string{"aaaa", "bbbb <![CDATA[", "c"}},
	{"html block tag", []string{"aaaa", "bbbb", "<div", "cc"}, 10,
		[]string{"aaaa", "bbbb <div", "cc"}},
	// GFM table delimiter rows, which turn the line before them into a
	// table header. "--- |" is a one-column row, so the second layout
	// keeps "---" off the start of a line as well.
	{"table delimiter row", []string{"aa", "|", "bb", "|-|-|", "cccccccc"}, 8,
		[]string{"aa |", "bb |-|-|", "cccccccc"}},
	{"spaced table delimiter row", []string{"aa", "|", "bb", "---", "|", "---", "cc"}, 9,
		[]string{"aa |", "bb --- |", "--- cc"}},
	{"aligned table delimiter row", []string{"aa", "|", "b", ":--|--:", "cccc"}, 9,
		[]string{"aa |", "b :--|--:", "cccc"}},
	{"one-column table delimiter row", []string{"aaaa", "bb", ":-", "cccccc"}, 7,
		[]string{"aaaa", "bb :-", "cccccc"}},
}

// TestWrapTokens_BlockStartNeverLeadsALine pins issue #844 for every
// kind of block start that can interrupt a paragraph (blockStartCases).
// Each case first checks that greedy wrapping splits the paragraph at
// its width. The guarded layout differs, keeps every line within width,
// and parses as the one paragraph it came from. Each want is exact, so
// removing any kind from lint.InterruptsParagraph changes the output and
// fails its case.
func TestWrapTokens_BlockStartNeverLeadsALine(t *testing.T) {
	for _, tc := range blockStartCases {
		t.Run(tc.name, func(t *testing.T) {
			naive := greedyWrap(tc.tokens, tc.width)
			require.False(t, singleParagraph(t, naive),
				"width %d must make greedy wrapping split the paragraph: %q", tc.width, naive)

			got := wrapTokens(tc.tokens, "", tc.width, noGlue)
			assert.Equal(t, tc.want, got)
			assert.True(t, singleParagraph(t, got), "layout %q splits the paragraph", got)
			for _, line := range got {
				assert.LessOrEqual(t, utf8.RuneCountInString(line), tc.width, "line %q", line)
			}
		})
	}
}

// TestWrapTokens_PlainTextWrapsLikeGreedy pins the other direction: text
// that only looks like a marker is no block start, so it wraps exactly
// as plain greedy packing does. Each width puts the token at the start
// of the second line.
func TestWrapTokens_PlainTextWrapsLikeGreedy(t *testing.T) {
	cases := []struct {
		name   string
		tokens []string
	}{
		{"assignment", []string{"aaaa", "bbbb", "=", "y"}},
		{"year ending a sentence", []string{"aaaa", "bbbb", "1999.", "Then"}},
		{"issue reference in link text", []string{"aaaa", "bbbb", "#48](url),"}},
		{"hashtag", []string{"aaaa", "bbbb", "#tag", "cc"}},
		{"ordered item not starting at 1", []string{"aaaa", "bbbb", "2.", "cc"}},
		{"empty star item", []string{"aaaa", "bbbb", "*"}},
		{"empty ordered item", []string{"aaaa", "bbbb", "1."}},
		{"dash word", []string{"aaaa", "bbbb", "-x", "cc"}},
		{"strong emphasis", []string{"aaaa", "bbbb", "**bold**"}},
		{"double dash with text", []string{"aaaa", "bbbb", "--", "cc"}},
		{"inline html", []string{"aaaa", "bbbb", "<span", "cc"}},
		{"html type 7 tag alone on its line", []string{"aaaa", "bbbb", "<span>"}},
		{"backtick fence info with backtick", []string{"aaaa", "bbbb", "```go", "`x`"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			naive := greedyWrap(tc.tokens, 10)
			require.Len(t, naive, 2)
			require.True(t, strings.HasPrefix(naive[1], tc.tokens[2]),
				"width must put %q at line start: %q", tc.tokens[2], naive)
			require.True(t, singleParagraph(t, naive), "greedy layout %q must stay one paragraph", naive)

			assert.Equal(t, naive, wrapTokens(tc.tokens, "", 10, noGlue))
		})
	}
}

// TestWrapTokens_OverflowOnlyWhenNoLayoutFits covers the fallback: when
// no layout within width avoids a block start, the marker stays on the
// line before it, which then runs past width. The overflow stays on the
// one line that needs it.
func TestWrapTokens_OverflowOnlyWhenNoLayoutFits(t *testing.T) {
	cases := []struct {
		name   string
		tokens []string
		want   []string
	}{
		{"marker after a full-width word", []string{"aaaaaaaaaa", "#", "b"},
			[]string{"aaaaaaaaaa #", "b"}},
		{"earlier lines stay within width", []string{"aaaa", "bbbb", "cccccccccc", "#", "d"},
			[]string{"aaaa bbbb", "cccccccccc #", "d"}},
		{"run of markers that cannot lead", []string{"x", "-", "-", "-"},
			[]string{"x - - -"}},
		// "1." may lead a line only alone, and "-" never may, so a
		// line break after "aaaaaaaaa" leads nowhere: the one safe
		// layout keeps all three on one line.
		{"marker that may lead only alone", []string{"aaaaaaaaa", "1.", "-"},
			[]string{"aaaaaaaaa 1. -"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapTokens(tc.tokens, "", 10, noGlue)
			assert.Equal(t, tc.want, got)
			assert.True(t, singleParagraph(t, got), "layout %q splits the paragraph", got)
		})
	}
}

// TestWrapTokens_NoSafeLayout covers a paragraph whose first word opens
// a block wherever the lines break: there is no layout to offer, so
// wrapTokens returns nil and the caller leaves the paragraph alone.
func TestWrapTokens_NoSafeLayout(t *testing.T) {
	assert.Nil(t, wrapTokens([]string{"<!doctype", "html", "page"}, "", 10, noGlue))
}

// TestWrapTokens_OverflowWindow pins maxOverflowUnits. A "-" can never
// lead a line (alone it is a setext underline, with more it is a bullet),
// so "x" and a run of them must share one line. At width 3 the fitting
// line is "x -"; the rest of the run is the overflow. A run that fits in
// the window gives that one line, and one unit more gives no layout.
func TestWrapTokens_OverflowWindow(t *testing.T) {
	run := func(dashes int) []string {
		return append([]string{"x"}, strings.Split(strings.Repeat("-", dashes), "")...)
	}
	within := run(1 + maxOverflowUnits)
	assert.Equal(t, []string{strings.Join(within, " ")}, wrapTokens(within, "", 3, noGlue))
	assert.Nil(t, wrapTokens(run(2+maxOverflowUnits), "", 3, noGlue))
}

// TestWrapTokens_FirstLineIsGuarded covers the paragraph's first line.
// Nothing precedes it inside the paragraph, but a first line that is a
// block start on its own still changes the document: "***" alone is a
// thematic break and a lone "```" opens a fence. Greedy wrapping at
// width 7 splits those paragraphs; the guarded layout keeps the marker
// with the next word, past width. The guard is conservative for a
// first unit that is unsafe only as a setext underline, such as "===":
// alone on the first line it would be harmless, yet it takes the next
// word too. It still gets a layout.
func TestWrapTokens_FirstLineIsGuarded(t *testing.T) {
	cases := []struct {
		name         string
		tokens       []string
		greedySplits bool
		want         []string
	}{
		{"thematic break", []string{"***", "aaaaaaaaaa"}, true, []string{"*** aaaaaaaaaa"}},
		{"fence", []string{"```", "`xxxxxx`"}, true, []string{"``` `xxxxxx`"}},
		{"setext-like first unit", []string{"===", "aaaaaaaaaa"}, false, []string{"=== aaaaaaaaaa"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, singleParagraph(t, []string{strings.Join(tc.tokens, " ")}),
				"%q must be one paragraph line", tc.tokens)
			naive := greedyWrap(tc.tokens, 7)
			require.Equal(t, tc.greedySplits, !singleParagraph(t, naive), "greedy layout %q", naive)
			got := wrapTokens(tc.tokens, "", 7, noGlue)
			assert.Equal(t, tc.want, got)
			assert.True(t, singleParagraph(t, got), "layout %q splits the paragraph", got)
		})
	}
}

// TestWrapTokens_GuardKeepsIndent checks that the indent prefix counts
// toward the width and still leads every line of a guarded layout.
func TestWrapTokens_GuardKeepsIndent(t *testing.T) {
	got := wrapTokens([]string{"aaaa", "bbbb", "#", "cc"}, "  ", 12, noGlue)
	assert.Equal(t, []string{"  aaaa", "  bbbb # cc"}, got)
}

// TestFix_NoSafeLayoutLeavesParagraph: goldmark reads a lowercase
// "<!doctype" line as a paragraph, but the CommonMark spec reads it as
// an HTML block, so every line that starts with it is unsafe. Fix must
// leave the paragraph as it is rather than drop or split it.
func TestFix_NoSafeLayoutLeavesParagraph(t *testing.T) {
	r := &Rule{Max: 40, Reflow: true}
	src := "# Title\n\n<!doctype html is how a page starts, and this line runs past the limit.\n"
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	require.Equal(t, ast.KindParagraph, f.AST.FirstChild().NextSibling().Kind())
	assert.Equal(t, src, string(r.Fix(f)))
}

// TestFix_Issue844Repro runs the issue's own paragraph at max 50. The
// `#48` that lands at the start of a line is paragraph text, so the
// greedy wrap stands, every line fits, and the link survives whole.
func TestFix_Issue844Repro(t *testing.T) {
	r := &Rule{Max: 50, Reflow: true}
	src := "# Title\n\nDiagnostics stopped being tied to the login prompt in " +
		"[Issue #48](https://example.com/repo/issues/48), and the flags were removed later.\n"
	want := "# Title\n\n" +
		"Diagnostics stopped being tied to the login prompt\n" +
		"in [Issue\n" +
		"#48](https://example.com/repo/issues/48), and the\n" +
		"flags were removed later.\n"
	got := fixSource(t, r, src)
	assert.Equal(t, want, got)
	assert.Equal(t, got, fixSource(t, r, got), "reflow is not a fixpoint")

	f, err := lint.NewFile("test.md", []byte(got))
	require.NoError(t, err)
	assert.Empty(t, r.Check(f))
	para := f.AST.FirstChild().NextSibling()
	require.Equal(t, ast.KindParagraph, para.Kind())
	require.Nil(t, para.NextSibling(), "the paragraph must not be split")
	var links []string
	for c := para.FirstChild(); c != nil; c = c.NextSibling() {
		if link, ok := c.(*ast.Link); ok {
			links = append(links, string(link.Destination))
		}
	}
	assert.Equal(t, []string{"https://example.com/repo/issues/48"}, links)
}

// TestFix_BlockStartsStayInsideParagraph reflows a paragraph of block
// markers at a width where greedy wrapping would lead two lines with
// them ("> for a quote," and "1. for a numbered item."). The result is
// still one paragraph, every line fits, and a second run changes
// nothing.
func TestFix_BlockStartsStayInsideParagraph(t *testing.T) {
	r := &Rule{Max: 35, Reflow: true}
	text := "Write # then a space for a heading, > for a quote, - for a bullet and 1. for a numbered item."
	require.False(t, singleParagraph(t, greedyWrap(strings.Fields(text), 35)),
		"greedy wrapping at 35 must split the paragraph")
	src := "# Title\n\n" + text + "\n"
	got := fixSource(t, r, src)
	assert.Equal(t, "# Title\n\n"+
		"Write # then a space for a\n"+
		"heading, > for a quote, - for a\n"+
		"bullet and 1. for a numbered item.\n", got)
	f, err := lint.NewFile("test.md", []byte(got))
	require.NoError(t, err)
	para := f.AST.FirstChild().NextSibling()
	require.Equal(t, ast.KindParagraph, para.Kind(), "got:\n%s", got)
	require.Nil(t, para.NextSibling(), "the paragraph was split:\n%s", got)
	assert.Empty(t, r.Check(f), "got:\n%s", got)
	assert.Equal(t, got, fixSource(t, r, got), "reflow is not a fixpoint")
}

// TestWrapTokens_RandomParagraphsMatchParser checks the guard against the
// canonical parser on seeded random paragraphs that mix words with
// marker-like tokens, at random widths:
//
//   - where greedy packing already gives one paragraph, the layout is the
//     greedy one, so the guard is no broader than the parser;
//   - every other layout keeps the words in order and parses as one
//     paragraph. It may be nil only there: a run of markers that cannot
//     lead a line can outgrow maxOverflowUnits.
//
// Tokens the spec reads as block starts but the parser does not, such as
// "<!doctype", are left out, since there the guard follows the spec. The
// last check makes sure enough cases reached the guard.
func TestWrapTokens_RandomParagraphsMatchParser(t *testing.T) {
	words := []string{"word", "longer", "text", "a"}
	markers := []string{
		"#", "##", "#48", "#tag", ">", ">x", "-", "--", "---", "+", "*", "**",
		"***", "_", "__", "=", "==", "1.", "1)", "2.", "01.", "1999.", "```",
		"```go", "``", "~~~", "~~~go", "`x`", "<div", "</div>", "<!--", "<?pi",
		"<pre", "<!X", "<span>", "<span", "<![CDATA[", "<",
	}
	rng := rand.New(rand.NewPCG(844, 851))
	guarded := 0
	for range 3000 {
		tokens := []string{"start"}
		for n := 2 + rng.IntN(14); len(tokens) < n; {
			pool := words
			if rng.IntN(2) == 0 {
				pool = markers
			}
			tokens = append(tokens, pool[rng.IntN(len(pool))])
		}
		width := 4 + rng.IntN(24)
		got := wrapTokens(tokens, "", width, noGlue)
		if naive := greedyWrap(tokens, width); singleParagraph(t, naive) {
			require.Equal(t, naive, got, "tokens %q width %d", tokens, width)
			continue
		}
		if got == nil {
			continue
		}
		guarded++
		require.Equal(t, tokens, strings.Fields(strings.Join(got, " ")))
		require.True(t, singleParagraph(t, got),
			"tokens %q width %d: layout %q splits the paragraph", tokens, width, got)
	}
	assert.Greater(t, guarded, 1000, "too few cases reached the guard")
}

// TestFix_TableDelimiterRowStaysInsideParagraph is the review repro for
// GFM tables. Greedy wrapping at max 8 puts "|-|-|" on a line of its
// own after "aa | bb", which GFM renderers and the table rules read as
// a table header and delimiter row. The guarded layout keeps the
// delimiter text inside a line, so the paragraph stays one paragraph.
func TestFix_TableDelimiterRowStaysInsideParagraph(t *testing.T) {
	r := &Rule{Max: 8, Reflow: true}
	text := "aa | bb |-|-| cccccccc dddddddd"
	require.False(t, singleParagraph(t, greedyWrap(strings.Fields(text), 8)),
		"greedy wrapping at 8 must build a table")
	got := fixSource(t, r, "# T\n\n"+text+"\n")
	assert.Equal(t, "# T\n\naa |\nbb |-|-|\ncccccccc\ndddddddd\n", got)
	assert.True(t, singleParagraph(t, strings.Split(strings.TrimSuffix(strings.TrimPrefix(got, "# T\n\n"), "\n"), "\n")))
	assert.Equal(t, got, fixSource(t, r, got), "reflow is not a fixpoint")
}
