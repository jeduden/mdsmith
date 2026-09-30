package refactor

import (
	"bytes"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moveAndApply runs Move over files and returns each file's rewritten
// text, keyed like files. Files without edits come back unchanged.
func moveAndApply(t *testing.T, files map[string]string, src, dst string) map[string]string {
	t.Helper()
	plan, err := Move(newMemWorkspace(files), src, dst)
	require.NoError(t, err)
	out := make(map[string]string, len(files))
	for rel, body := range files {
		out[rel] = applyEditsToSource(body, plan.Edits[rel])
	}
	return out
}

// TestMove_IncomingEmptyTextLinkRepointedOnce pins the corruption bug:
// a text-less `[](a.md)` after `[x](a.md)` on one row must get its own
// edit. Before, both edges landed on the first link, which was
// rewritten twice (`docs/a.md/a.md`) while the empty one stayed stale.
func TestMove_IncomingEmptyTextLinkRepointedOnce(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md": "# A\n",
		"b.md": "[x](a.md) and [](a.md)\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "[x](docs/a.md) and [](docs/a.md)\n", got["b.md"])
}

// TestMove_IncomingEmptyTextLinkOffFirstRow pins that a text-less link
// on a row other than the first is still repointed.
func TestMove_IncomingEmptyTextLinkOffFirstRow(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md": "# A\n",
		"b.md": "# B\n\n[](a.md)\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "# B\n\n[](docs/a.md)\n", got["b.md"])
}

// TestMove_IncomingLabelContentNeverRewritten pins that a
// destination-shaped string inside a link's own label (a code span
// here) is not taken for the link's destination.
func TestMove_IncomingLabelContentNeverRewritten(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md": "# A\n",
		"b.md": "[`](a.md?x)` t](a.md)\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "[`](a.md?x)` t](docs/a.md)\n", got["b.md"])
}

// TestMove_IncomingImageInLinkAndAngleFragment pins that an image
// nested in a link keeps its own destination while the link's is
// repointed, and that an angle-bracketed destination keeps its
// brackets and fragment.
func TestMove_IncomingImageInLinkAndAngleFragment(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md": "# A\n",
		"b.md": "[![alt](img.png)](a.md) [t](<a.md#frag>)\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "[![alt](img.png)](docs/a.md) [t](<docs/a.md#frag>)\n", got["b.md"])
}

// TestMove_NestedTitleOrDestinationHoldingLabelEnd pins that a `](`
// inside the title or the destination of an image that ends a link's
// label is skipped: the link's own destination is repointed, on a row
// or across rows, and the title keeps its text.
func TestMove_NestedTitleOrDestinationHoldingLabelEnd(t *testing.T) {
	b := "[![i](x.png \"t](y)\")](a.md)\n\n" +
		"[![j](a.md 'u](a.md)')](a.md)\n\n" +
		"> [![k](x.png\n> \"v\n> ](a.md)\")](a.md)\n\n" +
		"[*![l](x](y).png)*](a.md)\n"
	got := moveAndApply(t, map[string]string{"a.md": "# A\n", "b.md": b}, "a.md", "docs/a.md")
	assert.Equal(t,
		"[![i](x.png \"t](y)\")](docs/a.md)\n\n"+
			"[![j](docs/a.md 'u](a.md)')](docs/a.md)\n\n"+
			"> [![k](x.png\n> \"v\n> ](a.md)\")](docs/a.md)\n\n"+
			"[*![l](x](y).png)*](docs/a.md)\n",
		got["b.md"])
}

// TestMove_IncomingPercentEncodedAndAngleForms pins the percent-escape
// repro: an escaped `my%20file.md` is decoded before it is compared, a
// bare `my file.md` is not a link at all, and the angle form keeps its
// literal space. Each destination is rewritten exactly once.
func TestMove_IncomingPercentEncodedAndAngleForms(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"my file.md": "# M\n",
		"b.md":       "[x](my%20file.md) [y](my file.md) [z](<my file.md>)\n",
	}, "my file.md", "docs/my file.md")
	assert.Equal(t,
		"[x](docs/my%20file.md) [y](my file.md) [z](<docs/my file.md>)\n",
		got["b.md"])
}

// TestMove_OutboundSpellingKeptWhileItStillResolves pins that a
// destination in the moved file that still names its target from the
// new directory is left as the author spelled it, while a self-link,
// whose target moved, is repointed.
func TestMove_OutboundSpellingKeptWhileItStillResolves(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md": "[s](sub/../b.md) [t](./x/../b.md) [self](a.md)\n",
		"b.md": "# B\n",
	}, "a.md", "a2.md")
	assert.Equal(t, "[s](sub/../b.md) [t](./x/../b.md) [self](a2.md)\n", got["a.md"])
}

// TestMove_OutboundPercentEncodedDestination pins that the moved
// file's own escaped destination is decoded, recomputed, and
// re-escaped the way the author wrote it.
func TestMove_OutboundPercentEncodedDestination(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md":       "[m](my%20file.md) [c](caf%C3%A9.md)\n",
		"my file.md": "# M\n",
		"café.md":    "# C\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "[m](../my%20file.md) [c](../caf%C3%A9.md)\n", got["a.md"])
}

// TestMove_RefDefQueryAndEncodedDestinations pins that a ref-def
// destination with a `?query`, a percent-escape, or the angle form is
// repointed, keeping the query, the fragment, and the author's
// escaping.
func TestMove_RefDefQueryAndEncodedDestinations(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"my file.md": "# M\n",
		"b.md": "[q]: my%20file.md?x\n" +
			"[f]: my%20file.md?x#top\n" +
			"[e]: my%20file.md\n" +
			"[a]: <my file.md>\n",
	}, "my file.md", "docs/my file.md")
	assert.Equal(t,
		"[q]: docs/my%20file.md?x\n"+
			"[f]: docs/my%20file.md?x#top\n"+
			"[e]: docs/my%20file.md\n"+
			"[a]: <docs/my file.md>\n",
		got["b.md"])
}

// TestMove_IncomingSplitLabels pins that a link whose label spans rows
// is repointed, whether the destination follows on the label's last
// row or on the row after the `(`, including inside a block quote.
func TestMove_IncomingSplitLabels(t *testing.T) {
	b := "[a\nb](a.md) [c](\n  a.md)\n\n> [d\n> e](a.md \"t\") [f](\n> a.md)\n"
	got := moveAndApply(t, map[string]string{"a.md": "# A\n", "b.md": b}, "a.md", "docs/a.md")
	assert.Equal(t,
		"[a\nb](docs/a.md) [c](\n  docs/a.md)\n\n> [d\n> e](docs/a.md \"t\") [f](\n> docs/a.md)\n",
		got["b.md"])
}

// TestMove_OutboundSplitLabel pins the same for the moved file's own
// links, with CRLF line endings.
func TestMove_OutboundSplitLabel(t *testing.T) {
	a := "[a\r\n](./b.md) [c](\r\n./b.md)\r\n"
	got := moveAndApply(t, map[string]string{"a.md": a, "b.md": "# B\n"}, "a.md", "docs/a.md")
	assert.Equal(t, "[a\r\n](../b.md) [c](\r\n../b.md)\r\n", got["a.md"])
}

// TestMove_RefDefSplitAcrossRows pins that a ref-def whose label spans
// rows, or whose destination sits on the next row, is repointed.
func TestMove_RefDefSplitAcrossRows(t *testing.T) {
	b := "[multi\nline]: a.md\n\n[next]:\n  a.md \"t\"\n"
	got := moveAndApply(t, map[string]string{"a.md": "# A\n", "b.md": b}, "a.md", "docs/a.md")
	assert.Equal(t, "[multi\nline]: docs/a.md\n\n[next]:\n  docs/a.md \"t\"\n", got["b.md"])
}

// TestMove_QuestionMarkDestinationIsEscaped pins that a `?` in the new
// path is written as `%3F`: a literal `?` would start a query string,
// so the link would name a different file.
func TestMove_QuestionMarkDestinationIsEscaped(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md": "# A\n",
		"b.md": "[a](a.md#s)\n\n[r]: a.md\n",
	}, "a.md", "what?.md")
	assert.Equal(t, "[a](what%3F.md#s)\n\n[r]: what%3F.md\n", got["b.md"])
}

// TestMove_UnbalancedParenDestinationIsEscaped pins that a `(` or `)`
// without a partner in the new path is escaped in a bare destination,
// where it would end the destination early, while balanced parens and
// the angle form stay literal.
func TestMove_UnbalancedParenDestinationIsEscaped(t *testing.T) {
	b := "[a](a.md) [b](<a.md>)\n\n[r]: a.md\n"
	got := moveAndApply(t, map[string]string{"a.md": "# A\n", "b.md": b}, "a.md", "docs/a).md")
	assert.Equal(t, "[a](docs/a%29.md) [b](<docs/a).md>)\n\n[r]: docs/a%29.md\n", got["b.md"])
	got = moveAndApply(t, map[string]string{"a.md": "# A\n", "b.md": b}, "a.md", "docs/a(1).md")
	assert.Equal(t, "[a](docs/a(1).md) [b](<docs/a(1).md>)\n\n[r]: docs/a(1).md\n", got["b.md"])
}

// TestMove_EntityShapedDestinationIsEscaped pins that a `&` in the new
// path is written as `%26`: a renderer decodes `&amp;` in a
// destination, so a literal one would name another file.
func TestMove_EntityShapedDestinationIsEscaped(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md": "# A\n",
		"b.md": "[x](a.md) [y](<a.md>)\n\n[r]: a.md\n",
	}, "a.md", "d/a&amp;b.md")
	assert.Equal(t, "[x](d/a%26amp;b.md) [y](<d/a%26amp;b.md>)\n\n[r]: d/a%26amp;b.md\n", got["b.md"])
}

// TestMove_QuestionMarkFilenameNotTruncated pins that a destination
// naming an existing file with `?` in its name is matched whole, not
// cut at the `?`: the literal and the escaped spelling both follow the
// move, and the moved file's own link to itself stays put.
func TestMove_QuestionMarkFilenameNotTruncated(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"what?.md": "[self](what?.md) [o](other.md?x)\n",
		"other.md": "# O\n",
		"b.md":     "[l](what?.md) [e](what%3F.md)\n",
	}, "what?.md", "docs/what?.md")
	assert.Equal(t, "[l](docs/what%3F.md) [e](docs/what%3F.md)\n", got["b.md"])
	assert.Equal(t, "[self](what?.md) [o](../other.md?x)\n", got["what?.md"])
}

// TestMove_QuestionMarkLiteralOnlyReferrer pins that a file whose only
// reference is the literal `what?.md` is still read: the index takes
// that link for the file `what`, so it records no edge to the moved
// file, and the file holds no `]:` either.
func TestMove_QuestionMarkLiteralOnlyReferrer(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"what?.md": "# W\n",
		"b.md":     "[l](what?.md)\n",
	}, "what?.md", "docs/what?.md")
	assert.Equal(t, "[l](docs/what%3F.md)\n", got["b.md"])
}

// TestMove_OutboundQuestionMarkFilename pins the outbound side: a
// link from the moved file to an existing `what?.md` is recomputed
// whole, while a query on a file that exists keeps its query.
func TestMove_OutboundQuestionMarkFilename(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md":     "[w](what?.md) [q](b.md?plain=1)\n",
		"what?.md": "# W\n",
		"b.md":     "# B\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "[w](../what%3F.md) [q](../b.md?plain=1)\n", got["a.md"])
}

// TestDestLocator_AutoLinkMovesCursorPastClosingBracket pins that an
// autolink moves the cursor past its closing `>`, for a URL and an
// email autolink alike. Pos()+len(Label) stopped two bytes short.
func TestDestLocator_AutoLinkMovesCursorPastClosingBracket(t *testing.T) {
	body := "<http://h.io> <x@y.io>\n"
	lf, err := lint.NewFile("a.md", []byte(body))
	require.NoError(t, err)
	var ends []int
	_ = ast.Walk(lf.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if al, ok := n.(*ast.AutoLink); ok && entering {
			d := destLocator{lf: lf}
			_, _ = d.visit(al, true)
			ends = append(ends, d.cursor)
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, []int{len("<http://h.io>"), len("<http://h.io> <x@y.io>")}, ends)
}

// TestDestLocator_RawHTMLWithoutSegmentsKeepsCursor pins that a raw
// HTML node with no segments neither panics nor moves the cursor.
func TestDestLocator_RawHTMLWithoutSegmentsKeepsCursor(t *testing.T) {
	d := destLocator{cursor: 4}
	assert.NotPanics(t, func() { _, _ = d.visit(ast.NewRawHTML(), true) })
	assert.Equal(t, 4, d.cursor)
}

// TestDestLocator_CursorNeverMovesBack pins the monotonic guard: a
// node that ends, or a link that opens, before the cursor leaves it
// where it is, so an earlier `](` is never reached again.
func TestDestLocator_CursorNeverMovesBack(t *testing.T) {
	d := destLocator{cursor: 9}
	d.advance(true, 3)
	assert.Equal(t, 9, d.cursor)
	d.linkNode(true, 2, nil, true)
	assert.Equal(t, 9, d.cursor)
}

// TestDedupeEdits pins the safety net: an identical edit reported
// twice for one file is kept once, while distinct edits all stay.
func TestDedupeEdits(t *testing.T) {
	e := func(line, s, en int, txt string) Edit {
		return Edit{Range: Range{
			Start: Position{Line: line, Character: s},
			End:   Position{Line: line, Character: en},
		}, NewText: txt}
	}
	changes := map[string][]Edit{
		"b.md": {e(0, 4, 8, "docs/a.md"), e(0, 4, 8, "docs/a.md"), e(0, 4, 8, "x.md"), e(1, 4, 8, "docs/a.md")},
		"c.md": {e(2, 0, 1, "y")},
	}
	dedupeEdits(changes)
	assert.Equal(t, []Edit{e(0, 4, 8, "docs/a.md"), e(0, 4, 8, "x.md"), e(1, 4, 8, "docs/a.md")}, changes["b.md"])
	assert.Equal(t, []Edit{e(2, 0, 1, "y")}, changes["c.md"])
}

func TestLabelEnd(t *testing.T) {
	src := []byte(`[a\](x) b](y) [r]: z`)
	closeParen := bytes.Index(src, []byte("](y)")) + 2
	assert.Equal(t, closeParen, labelEnd(src, 0, '('), "an escaped `\\]` does not close the label")
	assert.Equal(t, closeParen, labelEnd(src, -3, '('), "a negative start clamps to zero")
	assert.Equal(t, bytes.Index(src, []byte("]:"))+2, labelEnd(src, 0, ':'))
	assert.Equal(t, -1, labelEnd([]byte("[a] b"), 0, '('))
}

func TestDestStart(t *testing.T) {
	for name, tc := range map[string]struct {
		src, dest string
		start     int
		angle     bool
	}{
		"spaces and tabs":           {"( \tb.md)", "b.md", 3, false},
		"angle brackets":            {"(<b c.md>)", "b c.md", 2, true},
		"next row in a block quote": {"(\r\n> >  b.md)", "b.md", 8, false},
	} {
		t.Run(name, func(t *testing.T) {
			start, angle, ok := destStart([]byte(tc.src), 1, []byte(tc.dest))
			require.True(t, ok)
			assert.Equal(t, tc.start, start)
			assert.Equal(t, tc.angle, angle)
		})
	}
	for name, tc := range map[string]struct{ src, dest string }{
		"other bytes":        {"(b.md)", "c.md"},
		"past the end":       {"(b", "b.md"},
		"unclosed angle":     {"(<b.md", "b.md"},
		"angle closed wrong": {"(<b.md)", "b.md"},
	} {
		t.Run(name+" is not found", func(t *testing.T) {
			_, _, ok := destStart([]byte(tc.src), 1, []byte(tc.dest))
			assert.False(t, ok)
		})
	}
}

func TestSkipGap(t *testing.T) {
	for name, tc := range map[string]struct {
		src  string
		want int
	}{
		"no gap":                    {"b.md", 0},
		"spaces and tabs":           {" \tb.md", 2},
		"next row in a block quote": {" \r\n> >  b.md", 8},
		"end of source":             {"  ", 2},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, skipGap([]byte(tc.src), 0))
		})
	}
}

func TestTitleEnd(t *testing.T) {
	for name, tc := range map[string]struct {
		src  string
		want int
	}{
		"no title":                  {`)](a.md)`, 0},
		"double-quoted":             {` "t](y)")`, 8},
		"single-quoted":             {` 't](y)')`, 8},
		"parenthesized":             {` (t])`, 5},
		"escaped closer":            {` "a\"](b")`, 9},
		"next row in a block quote": {"\n> \"t\n> ](y)\")", 13},
		"unterminated":              {` "t](y)`, 0},
		"end of source":             {` `, 0},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, titleEnd([]byte(tc.src), 0))
		})
	}
}

// TestDestLocator_SkipsWhatItCannotFind pins that the locator records
// nothing rather than guessing when the bytes goldmark parsed are not
// where the cursor leads, for an inline node and a ref-def alike.
func TestDestLocator_SkipsWhatItCannotFind(t *testing.T) {
	newLoc := func(body string, cursor int) *destLocator {
		lf, err := lint.NewFile("a.md", []byte(body))
		require.NoError(t, err)
		return &destLocator{lf: lf, fileLines: splitLines(lf.Source), cursor: cursor}
	}
	d := newLoc("[a](b.md)\n", 9)
	d.locate([]byte("b.md"))
	assert.Empty(t, d.dests, "no `](` after the cursor")

	d = newLoc("[a](b.md)\n", 0)
	d.locate([]byte("c.md"))
	assert.Empty(t, d.dests, "other bytes after the `](`")
	assert.Equal(t, 4, d.cursor, "the cursor still passes the `](`")

	d = newLoc("[l] no colon\n", 0)
	d.locateRefDef(ast.NewLinkReferenceDefinition([]byte("l"), []byte("a.md"), nil))
	assert.Empty(t, d.dests, "no `]:`")

	d = newLoc("[l]: a.md\n", 0)
	d.locateRefDef(ast.NewLinkReferenceDefinition([]byte("l"), []byte("b.md"), nil))
	assert.Empty(t, d.dests, "other bytes after the `]:`")
}

// TestDestResolverTarget pins how a destination is read: decoded, with
// the query and fragment outside the path token, and a literal `?`
// kept in the path only when that names a file and the query-stripped
// path does not.
func TestDestResolverTarget(t *testing.T) {
	ws := stubWorkspace{files: []string{"what?.md", "both", "both?.md"}}
	for dest, want := range map[string]destRef{
		"a.md#f":         {target: "a.md", path: "a.md", tokLen: 4},
		"a.md?x#f":       {target: "a.md", path: "a.md", tokLen: 4},
		"my%20file.md?x": {target: "my file.md", path: "my file.md", tokLen: 12},
		"what?.md#f":     {target: "what?.md", path: "what?.md", tokLen: 8},
		"what%3F.md":     {target: "what?.md", path: "what?.md", tokLen: 10},
		"both?.md":       {target: "both", path: "both", tokLen: 4},
		"nope.md?100%":   {target: "nope.md", path: "nope.md", tokLen: 7},
	} {
		t.Run(dest, func(t *testing.T) {
			r := &destResolver{ws: ws, src: "a.md"}
			got, ok := r.target("a.md", []byte(dest))
			require.True(t, ok)
			assert.Equal(t, want, got)
		})
	}
	for _, dest := range []string{"https://x.io/a.md", "#top", "../x.md", "../what?.md"} {
		t.Run(dest+" names no workspace file", func(t *testing.T) {
			r := &destResolver{ws: ws, src: "a.md"}
			_, ok := r.target("a.md", []byte(dest))
			assert.False(t, ok)
		})
	}
}

// TestEscapeSetFor pins which bytes a token's own escapes add, and that
// a `%` not followed by two hex digits adds none.
func TestEscapeSetFor(t *testing.T) {
	s := escapeSetFor("my%20caf%c3%A9.md", false)
	assert.True(t, s[' '], "an escaped space")
	assert.True(t, s[0xC3] && s[0xA9] && s[0x80] && s[0xFF], "one escaped high byte escapes them all")
	assert.False(t, s['/'], "a slash never")
	for _, tok := range []string{"100%.md", "a%4G.md", "a%G4.md", "a%"} {
		s = escapeSetFor(tok, false)
		assert.False(t, s[0xFF] || s[0xC3], "%q adds no high bytes", tok)
	}
}

func TestUnhex(t *testing.T) {
	for c, want := range map[byte]byte{'0': 0, '9': 9, 'a': 10, 'F': 15} {
		got, ok := unhex(c)
		assert.True(t, ok)
		assert.Equal(t, want, got)
	}
	for _, c := range []byte("gG.%") {
		_, ok := unhex(c)
		assert.False(t, ok, "%q", c)
	}
}

func TestParensPair(t *testing.T) {
	var none, open escapeSet
	open['('] = true
	for name, tc := range map[string]struct {
		p    string
		esc  *escapeSet
		want bool
	}{
		"no parens":            {"a.md", &none, true},
		"nested pairs":         {"a((b)c).md", &none, true},
		"close before open":    {"a)(.md", &none, false},
		"left open":            {"a(.md", &none, false},
		"escaped open unpairs": {"(a).md", &open, false},
		"escaped open is moot": {"(a.md", &open, true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, parensPair(tc.p, tc.esc))
		})
	}
}

func TestEncodeLike(t *testing.T) {
	for name, tc := range map[string]struct {
		path, oldTok string
		angle        bool
		want         string
	}{
		"nothing to escape":           {"docs/a.md", "a.md", false, "docs/a.md"},
		"reserved bytes":              {"a b/c?d#e%f<g>\t\x7f.md", "x.md", false, "a%20b/c%3Fd%23e%25f%3Cg%3E%09%7F.md"},
		"entity, escape and quote":    {"a&amp;b\\c\"d.md", "x.md", true, "a%26amp;b%5Cc%22d.md"},
		"angle keeps a space":         {"docs/my file.md", "my file.md", true, "docs/my file.md"},
		"author escaped a byte":       {"docs/(a).md", "%28a%29.md", false, "docs/%28a%29.md"},
		"unbalanced parens":           {"docs/a)(.md", "a.md", false, "docs/a%29%28.md"},
		"one escaped paren unpairs":   {"docs/(a).md", "%28a).md", false, "docs/%28a%29.md"},
		"angle keeps unpaired parens": {"docs/a).md", "a.md", true, "docs/a).md"},
		"lowercase escape":            {"docs/café.md", "caf%c3%a9.md", false, "docs/caf%C3%A9.md"},
		"escaped UTF-8 escapes all":   {"naïve/café.md", "caf%C3%A9.md", false, "na%C3%AFve/caf%C3%A9.md"},
		"a slash is never escaped":    {"docs/a.md", "x%2Fa.md", false, "docs/a.md"},
		"literal UTF-8 stays literal": {"docs/café.md", "café.md", false, "docs/café.md"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, encodeLike(tc.path, tc.oldTok, tc.angle))
		})
	}
}
