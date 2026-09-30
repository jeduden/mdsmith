package linelength

import (
	"math/rand/v2"
	"regexp"
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
// interrupt a paragraph, in CommonMark or in an extension (tables,
// footnotes, definition lists). Each width makes plain greedy wrapping
// put the marker at the start of a line; want is the guarded layout.
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
	{"plus bullet", []string{"aaaa", "bbbb", "+", "cc"}, 10,
		[]string{"aaaa", "bbbb + cc"}},
	{"star bullet", []string{"aaaa", "bbbb", "*", "cc"}, 10,
		[]string{"aaaa", "bbbb * cc"}},
	{"ordered item 1.", []string{"aaaa", "bbbb", "1.", "cc"}, 10,
		[]string{"aaaa", "bbbb 1. cc"}},
	{"ordered item 1)", []string{"aaaa", "bbbb", "1)", "cc"}, 10,
		[]string{"aaaa", "bbbb 1) cc"}},
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
	// A footnote definition, with or without a space after the colon.
	{"footnote definition", []string{"aaaa", "bbbb", "[^1]:", "cc"}, 10,
		[]string{"aaaa", "bbbb [^1]:", "cc"}},
	{"footnote definition without a space", []string{"aaaa", "bbbb", "[^1]:c"}, 11,
		[]string{"aaaa", "bbbb [^1]:c"}},
	// A definition-list description needs a space after the ':', so a
	// lone ':' may lead a line and the first line stays full.
	{"definition list description", []string{"aaaa", "bbbb", ":", "cc"}, 10,
		[]string{"aaaa bbbb", ":", "cc"}},
}

// TestWrapTokens_BlockStartNeverLeadsALine pins issue #844 for every
// kind of block start that can interrupt a paragraph (blockStartCases).
// Each case first checks that greedy wrapping splits the paragraph at
// its width. The guarded layout differs, keeps every line within width,
// and parses as the one paragraph it came from, under the canonical and
// the flavor parser. Each want is exact, so removing any kind from the
// guard (unsafeLine) changes the output and fails its case.
func TestWrapTokens_BlockStartNeverLeadsALine(t *testing.T) {
	for _, tc := range blockStartCases {
		t.Run(tc.name, func(t *testing.T) {
			naive := greedyWrap(tc.tokens, tc.width)
			require.False(t, singleParagraph(t, naive),
				"width %d must make greedy wrapping split the paragraph: %q", tc.width, naive)

			got := wrapTokens(tc.tokens, nil, "", tc.width, noGlue)
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
		{"empty ordered item not starting at 1", []string{"aaaa", "bbbb", "2."}},
		{"lone colon", []string{"aaaa", "bbbb", ":"}},
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

			assert.Equal(t, naive, wrapTokens(tc.tokens, nil, "", 10, noGlue))
		})
	}
}

// TestWrapTokens_BareMarkerNeverStandsAlone covers a list marker with
// nothing after it. CommonMark lets no empty item interrupt a paragraph,
// so the greedy layout is one paragraph, yet some renderers and
// formatters read such a line as an empty list item. Reflow does not
// rely on the rule: it moves the word before the marker down, as for a
// real block start. A bare "2." could not interrupt even with content,
// so it still wraps as text (see TestWrapTokens_PlainTextWrapsLikeGreedy).
func TestWrapTokens_BareMarkerNeverStandsAlone(t *testing.T) {
	for _, marker := range []string{"*", "+", "1.", "1)", "01."} {
		t.Run(marker, func(t *testing.T) {
			tokens := []string{"aaaa", "bbbb", marker}
			naive := greedyWrap(tokens, 10)
			require.Equal(t, []string{"aaaa bbbb", marker}, naive)
			require.True(t, singleParagraph(t, naive), "greedy layout %q must stay one paragraph", naive)
			assert.Equal(t, []string{"aaaa", "bbbb " + marker}, wrapTokens(tokens, nil, "", 10, noGlue))
		})
	}
}

func TestIsBareListMarker(t *testing.T) {
	for _, line := range []string{"-", "+", "*", "1.", "1)", "01.", "000000001)", "   *", "1. "} {
		assert.True(t, isBareListMarker([]byte(line)), "%q", line)
	}
	for _, line := range []string{"", "    *", "2.", "10.", "0000000001.", "1", ".", "1:", "* x", "1.x", "**", "a."} {
		assert.False(t, isBareListMarker([]byte(line)), "%q", line)
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
		// Neither "1." (bare or with "-" after it) nor "-" may lead a
		// line, so a line break after "aaaaaaaaa" leads nowhere: the
		// one safe layout keeps all three on one line.
		{"markers after a nearly full word", []string{"aaaaaaaaa", "1.", "-"},
			[]string{"aaaaaaaaa 1. -"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapTokens(tc.tokens, nil, "", 10, noGlue)
			assert.Equal(t, tc.want, got)
			assert.True(t, singleParagraph(t, got), "layout %q splits the paragraph", got)
		})
	}
}

// TestWrapTokens_NoSafeLayout covers a paragraph whose first word opens
// a block wherever the lines break: there is no layout to offer, so
// wrapTokens returns nil and the caller leaves the paragraph alone.
func TestWrapTokens_NoSafeLayout(t *testing.T) {
	assert.Nil(t, wrapTokens([]string{"<!doctype", "html", "page"}, nil, "", 10, noGlue))
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
	assert.Equal(t, []string{strings.Join(within, " ")}, wrapTokens(within, nil, "", 3, noGlue))
	assert.Nil(t, wrapTokens(run(2+maxOverflowUnits), nil, "", 3, noGlue))
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
			got := wrapTokens(tc.tokens, nil, "", 7, noGlue)
			assert.Equal(t, tc.want, got)
			assert.True(t, singleParagraph(t, got), "layout %q splits the paragraph", got)
		})
	}
}

// TestWrapTokens_GuardKeepsIndent checks that the indent prefix counts
// toward the width and still leads every line of a guarded layout.
func TestWrapTokens_GuardKeepsIndent(t *testing.T) {
	got := wrapTokens([]string{"aaaa", "bbbb", "#", "cc"}, nil, "  ", 12, noGlue)
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

// conservativeLine matches lines the guard rejects although a paragraph
// line before them may keep the paragraph whole: a bare list marker,
// and a GFM table delimiter row, which builds a table only when the line
// before it has as many cells. The patterns are written out here so the
// test does not share code with the guard.
var (
	bareMarkerLine   = regexp.MustCompile(`^ {0,3}([-+*]|0{0,8}1[.)])$`)
	delimiterRowLine = regexp.MustCompile(`^ {0,3}\|?[ \t]*:?-+:?[ \t]*(\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)
	dashesOnlyLine   = regexp.MustCompile(`^-+$`)
)

func conservativeLine(line string) bool {
	return bareMarkerLine.MatchString(line) ||
		delimiterRowLine.MatchString(line) && !dashesOnlyLine.MatchString(line)
}

// safeLayout reports whether lines form one paragraph under both parsers
// and hold no line the guard rejects on purpose (conservativeLine).
func safeLayout(t *testing.T, lines []string) bool {
	t.Helper()
	for _, line := range lines {
		if conservativeLine(line) {
			return false
		}
	}
	return singleParagraph(t, lines)
}

// TestWrapTokens_RandomParagraphsMatchParser checks the guard against the
// canonical and flavor parsers on seeded random paragraphs that mix
// words with marker-like tokens, at random widths. The tokens include
// pipes, table delimiter rows, footnote labels and a lone ':', so the
// extension block starts are exercised too:
//
//   - where greedy packing already gives a safe layout (safeLayout), the
//     layout is the greedy one, so the guard is no broader than the
//     parsers plus the conservative lines;
//   - every other layout keeps the words in order and is safe. It may be
//     nil only there: a run of markers that cannot lead a line can
//     outgrow maxOverflowUnits.
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
		"|", "|-|-|", "-|-", ":-", "--|", "[^x]:", "[^1]:y", "[^]:", ":", "::",
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
		got := wrapTokens(tokens, nil, "", width, noGlue)
		if naive := greedyWrap(tokens, width); safeLayout(t, naive) {
			require.Equal(t, naive, got, "tokens %q width %d", tokens, width)
			continue
		}
		if got == nil {
			continue
		}
		guarded++
		require.Equal(t, tokens, strings.Fields(strings.Join(got, " ")))
		require.True(t, safeLayout(t, got),
			"tokens %q width %d: layout %q is not safe", tokens, width, got)
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
	body := strings.TrimSuffix(strings.TrimPrefix(got, "# T\n\n"), "\n")
	assert.True(t, singleParagraph(t, strings.Split(body, "\n")), "layout %q", body)
	assert.Equal(t, got, fixSource(t, r, got), "reflow is not a fixpoint")
}

func TestUnsafeLine(t *testing.T) {
	for _, line := range []string{"# x", "> x", "|-|-|", "[^1]: x", ": x", "*", "1."} {
		assert.True(t, unsafeLine([]byte(line)), "%q", line)
	}
	for _, line := range []string{"text", "#48", "2.", ":", "-x"} {
		assert.False(t, unsafeLine([]byte(line)), "%q", line)
	}
}

// planner builds a linePlanner over units the way wrapTokens does.
func planner(units []string, indent string, width int) *linePlanner {
	return &linePlanner{units: units, indent: indent, indentW: utf8.RuneCountInString(indent), width: width}
}

func TestLinePlanner_FitEnd(t *testing.T) {
	p := planner([]string{"aa", "bb", "ccc"}, "", 5)
	assert.Equal(t, 2, p.fitEnd(0), `"aa bb" fills the width`)
	assert.Equal(t, 3, p.fitEnd(2))
	wide := planner([]string{"aaaaaaa", "b"}, "", 5)
	assert.Equal(t, 1, wide.fitEnd(0), "a unit wider than width still fits alone")
	indented := planner([]string{"aa", "bb"}, "  ", 5)
	assert.Equal(t, 1, indented.fitEnd(0), "the indent counts toward the width")
}

func TestLinePlanner_Render(t *testing.T) {
	p := planner([]string{"aa", "bb", "cc"}, "  ", 10)
	assert.Equal(t, "  aa bb", string(p.render(0, 2)))
	assert.Equal(t, "  cc", string(p.render(2, 3)), "the scratch buffer is reused")
}

func TestLinePlanner_Breaks(t *testing.T) {
	p := planner([]string{"aa", "#", "b"}, "", 10)
	assert.False(t, p.breaks(0, 3))
	assert.True(t, p.breaks(1, 3), `"# b" is a heading`)
	assert.True(t, p.breaks(1, 2), `a lone "#" is an empty heading`)
}

// TestLinePlanner_Pick walks each preference in order: a line after
// which everything fits, the longest fitting line before a later
// overflow, the shortest overflowing line, and no line at all.
func TestLinePlanner_Pick(t *testing.T) {
	p := planner([]string{"x", "aaaaaaaaaa", "#", "b"}, "", 10)
	plans := make([]linePlan, 5)
	plans[4] = linePlan{end: 4, fits: true}
	plans[3] = p.pick(3, plans)
	assert.Equal(t, linePlan{end: 4, fits: true}, plans[3], `"b" fits`)
	plans[2] = p.pick(2, plans)
	assert.Equal(t, linePlan{}, plans[2], `"#" can lead no line`)
	plans[1] = p.pick(1, plans)
	assert.Equal(t, linePlan{end: 3}, plans[1], `"aaaaaaaaaa #" overflows`)
	plans[0] = p.pick(0, plans)
	assert.Equal(t, linePlan{end: 1}, plans[0], `"x" fits, but a later line overflows`)
}

func TestLinePlanner_Layout(t *testing.T) {
	p := planner([]string{"aaaa", "bbbb", "#", "cc"}, "", 10)
	assert.Equal(t, []string{"aaaa", "bbbb # cc"}, p.layout())
	assert.Nil(t, planner([]string{"<!doctype", "html"}, "", 10).layout())
}

// flavorBlocks lists the kinds of the block nodes the flavor parser
// builds for src, in document order. The flavor parser adds footnotes,
// definition lists and GFM tables, which the canonical parser leaves
// out, so the list shows whether a fix changed any of those blocks.
func flavorBlocks(t *testing.T, src string) []string {
	t.Helper()
	var kinds []string
	flavor.WithSharedParser(func(p parser.Parser) {
		doc := p.Parse(text.NewReader([]byte(src)))
		_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && n.Type() == ast.TypeBlock {
				kinds = append(kinds, n.Kind().String())
			}
			return ast.WalkContinue, nil
		})
	})
	return kinds
}

// TestFix_FirstLineKeepsItsStart covers a paragraph that opens with a
// footnote definition or a definition line. The canonical parser reads
// both as paragraph text, and every layout's first line starts with the
// same marker, so the guard must not reject that start on line 1. The
// paragraph wraps, the marker keeps the word after it, every line fits,
// and the flavor parser sees the same blocks as before.
func TestFix_FirstLineKeepsItsStart(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{
			"footnote definition",
			"[^1]: A footnote whose text is quite long and goes on past thirty.",
			"[^1]: A footnote whose text is\nquite long and goes on past\nthirty.",
		},
		{
			"definition line",
			": a description that is quite long and goes on past thirty",
			": a description that is quite\nlong and goes on past thirty",
		},
	}
	r := &Rule{Max: 30, Reflow: true}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "# T\n\n" + tc.text + "\n"
			got := fixSource(t, r, src)
			assert.Equal(t, "# T\n\n"+tc.want+"\n", got)
			f, err := lint.NewFile("test.md", []byte(got))
			require.NoError(t, err)
			assert.Empty(t, r.Check(f), "got:\n%s", got)
			assert.Equal(t, flavorBlocks(t, src), flavorBlocks(t, got))
			assert.Equal(t, got, fixSource(t, r, got), "reflow is not a fixpoint")
		})
	}
}

// TestWrapTokens_FirstLineKeepsItsStart drives keepsStart through the
// planner. first is the paragraph's first line as written. Line 1 may
// open the extension block that first opens, and its marker keeps the
// word after it, past width when a later line needs that. A marker that
// stood alone on first stays alone. Without the check, the definition
// case would put a lone ":" on line 1 and the footnote cases would have
// no layout at all.
func TestWrapTokens_FirstLineKeepsItsStart(t *testing.T) {
	cases := []struct {
		name   string
		first  string
		tokens []string
		width  int
		want   []string
	}{
		{"definition marker keeps its word", ": a # b",
			[]string{":", "a", "#", "b"}, 4, []string{": a #", "b"}},
		{"footnote marker keeps its word", "[^1]: a # b",
			[]string{"[^1]:", "a", "#", "b"}, 7, []string{"[^1]: a #", "b"}},
		{"lone footnote marker stays alone", "[^1]:",
			[]string{"[^1]:", "aaaa", "bbbb"}, 10, []string{"[^1]:", "aaaa bbbb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, wrapTokens(tc.tokens, []byte(tc.first), "", tc.width, noGlue))
		})
	}
}

func TestKeepsStart(t *testing.T) {
	cases := []struct {
		first, line string
		want        bool
	}{
		{"", "text", true},
		{"text more", "text", true},
		{"*** text", "***", false},
		{"* text", "*", false},
		{"<!doctype html page", "<!doctype html", false},
		{"--- | --- text", "--- | ---", false},
		{"[^1]: a b", "[^1]: a", true},
		{"[^1]: a b", "[^1]:", false},
		{"[^1]:", "[^1]:", true},
		{"[^1]:", "[^1]: a", false},
		{": a b", ": a", true},
		{": a b", ":", false},
		{"  : a b", "  : a", true},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, keepsStart([]byte(tc.first), []byte(tc.line)), "first %q, line %q", tc.first, tc.line)
	}
}

// TestFix_LeavesExistingBlockStartsAlone covers paragraphs whose source
// already has a line after the first that unsafeLine rejects. The
// canonical parser reads such a line as paragraph text, so the
// paragraph reaches reflow, but joining it would change the document:
// the flavor parser and GFM renderers see a definition list, a table or
// a footnote there. Fix leaves the paragraph as written, the reverse of
// the guard that keeps reflow from starting such a line.
func TestFix_LeavesExistingBlockStartsAlone(t *testing.T) {
	cases := []struct{ name, text string }{
		{"definition list", "Term\n: Definition text that runs well past the thirty column limit."},
		{"table without outer pipes", "Name | Value that is long enough\n--- | ---\na | b"},
		{"table with CRLF", "Name | Value that is long enough\r\n--- | ---\r\na | b"},
		{"footnote definition", "The claim in this sentence is backed by a source, yes.[^1]\n[^1]: The footnote."},
		{"bare list marker", "Some text that is long enough to be flagged at thirty\n*\nmore"},
		{"spec-only html start", "Some text that is long enough to be flagged at thirty\n<!doctype html>"},
	}
	r := &Rule{Max: 30, Reflow: true}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nl := "\n"
			if strings.Contains(tc.text, "\r\n") {
				nl = "\r\n"
			}
			src := "# T" + nl + nl + tc.text + nl
			f, err := lint.NewFile("test.md", []byte(src))
			require.NoError(t, err)
			para := f.AST.FirstChild().NextSibling()
			require.Equal(t, ast.KindParagraph, para.Kind())
			require.Equal(t, strings.Count(tc.text, "\n")+1, para.Lines().Len(),
				"the canonical parser must read every line as paragraph text")
			require.NotEmpty(t, r.Check(f), "the paragraph must have a flagged line")
			assert.Equal(t, src, string(r.Fix(f)))
		})
	}
}

func TestHasUnsafeContinuation(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte(": a\nb\n: c\nd\r\n|-|\r\n"))
	require.NoError(t, err)
	assert.False(t, hasUnsafeContinuation(f, 1, 2), "the first line is not checked")
	assert.True(t, hasUnsafeContinuation(f, 1, 3), `": c" is a definition`)
	assert.False(t, hasUnsafeContinuation(f, 3, 4))
	assert.True(t, hasUnsafeContinuation(f, 4, 5), "a CRLF line ending is ignored")
}
