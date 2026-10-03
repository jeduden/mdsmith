package refactor

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkRef_DefAndShortcutUse(t *testing.T) {
	src := []byte("# T\n\nSee [spec].\n\n[spec]: https://x.example\n")
	edits, err := callLinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 2)
	// Both edits replace the label text with the new name.
	for _, e := range edits {
		assert.Equal(t, "rfc", e.NewText)
		assert.Equal(t, e.Range.Start.Line, e.Range.End.Line)
	}
}

func TestHasLinkRefIn(t *testing.T) {
	ps := parseSource([]byte("# T\n\nSee [the spec][Spec].\n\n[Spec]: u\n"))
	// Matches case-insensitively via CommonMark label normalization.
	assert.True(t, hasLinkRefIn(ps, "spec"))
	assert.True(t, hasLinkRefIn(ps, "SPEC"))
	assert.False(t, hasLinkRefIn(ps, "ghost"))
	// A def-shaped line inside a code fence is not a real definition.
	fenced := parseSource([]byte("# T\n\n```\n[fake]: u\n```\n"))
	assert.False(t, hasLinkRefIn(fenced, "fake"))
}

func TestLinkRefPlan(t *testing.T) {
	ps := parseSource([]byte("See [a][Spec].\n\n[Spec]: u\n[Other]: v\n"))
	// The old label is normalized, so a differently-cased spelling
	// still finds the def and its use.
	p, err := linkRefPlan("k.md", ps, "SPEC", "Doc")
	require.NoError(t, err)
	assert.Nil(t, p.FileOp)
	require.Len(t, p.Edits["k.md"], 2)
	for _, e := range p.Edits["k.md"] {
		assert.Equal(t, "Doc", e.NewText)
	}

	_, err = linkRefPlan("k.md", ps, "spec", "  ")
	assert.ErrorIs(t, err, ErrEmptyLabel)
	_, err = linkRefPlan("k.md", ps, "spec", "a]b")
	assert.ErrorAs(t, err, new(InvalidLabelRuneError))
	_, err = linkRefPlan("k.md", ps, "spec", "other")
	assert.Equal(t, LabelConflictError{Conflict: "Other"}, err)
}

func TestRefDefMatchesIn(t *testing.T) {
	body := []byte("[Spec]: u\n\n```\n[fake]: v\n```\n")
	got := refDefMatchesIn(body, parseBody(body))
	// The fenced def-shaped line is consumed by the code block.
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].bodyLine) // 1-based
	assert.Equal(t, "Spec", got[0].rawLabel)
	assert.Equal(t, "spec", got[0].normLabel)
	assert.Nil(t, refDefMatchesIn([]byte("no defs\n"), parseBody([]byte("no defs\n"))))
}

func TestLinkRef_PlanKeysUnderFileKeyNoFileOp(t *testing.T) {
	src := []byte("# T\n\nSee [spec].\n\n[spec]: u\n")
	plan, err := LinkRef("docs/a.md", src, "spec", "rfc")
	require.NoError(t, err)
	// A label rename is file-local: every edit groups under the file's
	// own key and nothing else, and no file operation is planned.
	require.Len(t, plan.Edits, 1)
	assert.Len(t, plan.Edits["docs/a.md"], 2)
	assert.Nil(t, plan.FileOp)
}

func TestLinkRef_DefAndFullUse(t *testing.T) {
	src := []byte("# T\n\nSee [the spec][spec] here.\n\n[spec]: u\n")
	edits, err := callLinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 2)
}

func TestLinkRef_EmptyLabel(t *testing.T) {
	_, err := callLinkRef([]byte("[a]: u\n"), "a", "   ")
	assert.ErrorIs(t, err, ErrEmptyLabel)
}

func TestLinkRef_InvalidRune(t *testing.T) {
	_, err := callLinkRef([]byte("[a]: u\n"), "a", "bad]label")
	var ire InvalidLabelRuneError
	require.True(t, errors.As(err, &ire))
	assert.Equal(t, ']', ire.Rune)
}

func TestLinkRef_NewlineRuneRejected(t *testing.T) {
	_, err := callLinkRef([]byte("[a]: u\n"), "a", "two\nlines")
	var ire InvalidLabelRuneError
	require.True(t, errors.As(err, &ire))
	assert.Equal(t, '\n', ire.Rune)
}

func TestLinkRef_LabelConflict(t *testing.T) {
	src := []byte("[a]: u1\n[Beta]: u2\n")
	_, err := callLinkRef(src, "a", "beta")
	var lce LabelConflictError
	require.True(t, errors.As(err, &lce))
	assert.Equal(t, "Beta", lce.Conflict)
}

func TestLinkRef_SameNormalizedFormRefreshesCasing(t *testing.T) {
	// "a b" and "A  B" normalize identically; the rename is allowed
	// and must not self-collide.
	src := []byte("Use [a b].\n\n[a b]: u\n")
	edits, err := callLinkRef(src, "a b", "A  B")
	require.NoError(t, err)
	require.Len(t, edits, 2)
}

func TestLinkRef_NormalizesOldLabel(t *testing.T) {
	// LinkRef normalizes oldLabel internally, so callers may pass
	// the raw label text without pre-normalizing it.
	src := []byte("See [spec].\n\n[spec]: u\n")
	edits, err := callLinkRef(src, "Spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 2)
}

func TestLinkRef_CodeFenceDefNotRewritten(t *testing.T) {
	src := []byte("Use [spec].\n\n```\n[spec]: fake\n```\n\n[spec]: real\n")
	edits, err := callLinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	// The fenced `[spec]: fake` is content, not a def: only the real
	// def plus the one use are rewritten.
	require.Len(t, edits, 2)
	for _, e := range edits {
		assert.NotEqual(t, 3, e.Range.Start.Line, "fence line must not be edited")
	}
}

func TestLinkRef_WithFrontMatterLineOffset(t *testing.T) {
	src := []byte("---\ntitle: x\n---\n# H\n\nSee [s].\n\n[s]: u\n")
	edits, err := callLinkRef(src, "s", "t")
	require.NoError(t, err)
	require.Len(t, edits, 2)
	// Edits land on body lines shifted by the 3-line front matter.
	for _, e := range edits {
		assert.GreaterOrEqual(t, e.Range.Start.Line, 3)
	}
}

func TestValidRefDefBodyLines(t *testing.T) {
	body := []byte("para\n\n[a]: u\n\n```\n[b]: v\n```\n")
	// ValidRefDefBodyLines returns map[int]struct{} — presence means valid def.
	got := ValidRefDefBodyLines(body)
	_, ok3 := got[3]
	assert.True(t, ok3, "real def on body line 3")
	_, ok6 := got[6]
	assert.False(t, ok6, "fenced def-shaped line is not a def")
}

func TestBodyAndFMOffset(t *testing.T) {
	t.Run("no front matter", func(t *testing.T) {
		body, off := BodyAndFMOffset([]byte("# H\n"))
		assert.Equal(t, 0, off)
		assert.Equal(t, "# H\n", string(body))
	})
	t.Run("with front matter", func(t *testing.T) {
		_, off := BodyAndFMOffset([]byte("---\na: b\n---\n# H\n"))
		assert.Equal(t, 3, off)
	})
}

func TestLinkRef_OtherReferenceLabelsUntouched(t *testing.T) {
	// A second reference use with a different label exercises the
	// label-mismatch skip in the AST walk: only [spec] is rewritten.
	src := []byte("See [spec] and [misc].\n\n[spec]: u\n[misc]: v\n")
	edits, err := callLinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 2, "only the spec def + spec use")
}

func TestLinkRef_NoMatchingLabel(t *testing.T) {
	// Renaming a label with no def and no use yields no edits and
	// no error (the caller decides whether that is meaningful).
	edits, err := callLinkRef([]byte("# Just a heading\n"), "ghost", "spirit")
	require.NoError(t, err)
	assert.Empty(t, edits)
}

func TestNormalizedLabel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"spec", "spec"},
		{"Spec", "spec"},
		{"API Docs", "api docs"},
		{"API  Docs", "api docs"},
		{"  leading  ", "leading"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizedLabel([]byte(tc.input)))
		})
	}
}

func TestInvalidLinkRefRune(t *testing.T) {
	assert.Equal(t, rune(0), invalidLinkRefRune("plain label"))
	assert.Equal(t, rune(0), invalidLinkRefRune(""))
	assert.Equal(t, '\n', invalidLinkRefRune("a\nb"))
	assert.Equal(t, '\r', invalidLinkRefRune("a\rb"))
	assert.Equal(t, '[', invalidLinkRefRune("a[b"))
	assert.Equal(t, ']', invalidLinkRefRune("a]b"))
	// The first offending rune wins.
	assert.Equal(t, '[', invalidLinkRefRune("x[y]"))
}

func TestLabelConflictIn(t *testing.T) {
	ps := parseSource([]byte("[a]: u1\n[Beta]: u2\n"))
	assert.Equal(t, "Beta", labelConflictIn(ps, "a", "beta"))
	assert.Equal(t, "", labelConflictIn(ps, "a", "gamma"))
	// Renaming a label to itself is not a conflict.
	assert.Equal(t, "", labelConflictIn(ps, "a", "a"))
	// A def-shaped line inside a fence is not a real definition.
	fenced := parseSource([]byte("[a]: u1\n\n```\n[beta]: u2\n```\n"))
	assert.Equal(t, "", labelConflictIn(fenced, "a", "beta"))
	// Front matter does not confuse the scan.
	fm := parseSource([]byte("---\ntitle: t\n---\n[a]: u1\n[beta]: u2\n"))
	assert.Equal(t, "beta", labelConflictIn(fm, "a", "beta"))
}

func TestLinkRefEditsIn(t *testing.T) {
	ps := parseSource([]byte("# T\n\nSee [spec], [the spec][spec], and [other][x].\n\n[spec]: u\n[x]: v\n"))
	edits := linkRefEditsIn(ps, "spec", "rfc")
	// One def edit plus a shortcut use and a full use.
	require.Len(t, edits, 3)
	for _, e := range edits {
		assert.Equal(t, "rfc", e.NewText)
	}
	// The def edit comes first and targets line index 4.
	assert.Equal(t, 4, edits[0].Range.Start.Line)
	// Uses: shortcut [spec] at columns 5-9, full [the spec][spec] label
	// at columns 23-27, both on line index 2.
	for i, want := range [][2]int{{5, 9}, {23, 27}} {
		e := edits[i+1]
		assert.Equal(t, 2, e.Range.Start.Line)
		assert.Equal(t, want[0], e.Range.Start.Character)
		assert.Equal(t, want[1], e.Range.End.Character)
	}
	assert.Empty(t, linkRefEditsIn(ps, "ghost", "rfc"))
}

func firstLink(t *testing.T, root ast.Node) *ast.Link {
	t.Helper()
	var found *ast.Link
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.Link); ok && entering && found == nil {
			found = l
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, found, "no link in body")
	return found
}

func TestRefUseEditsInBody(t *testing.T) {
	src := []byte("[a] and [b] and [t][A].\n\n[a]: u\n[b]: v\n")
	body, off := bodyAndFMOffset(src)
	root := parseBody(body)
	lines := splitLines(src)

	edits := refUseEditsInBody(root, body, lines, off, "a", "z")
	// Shortcut [a] and full [t][A]; [b] is ignored.
	require.Len(t, edits, 2)
	assert.Equal(t, 0, edits[0].Range.Start.Line)
	assert.Equal(t, 1, edits[0].Range.Start.Character)
	assert.Equal(t, 2, edits[0].Range.End.Character)
	assert.Equal(t, "z", edits[1].NewText)
	// [t][A]: the label A sits at columns 20-21 of line 0.
	assert.Equal(t, 0, edits[1].Range.Start.Line)
	assert.Equal(t, 20, edits[1].Range.Start.Character)
	assert.Equal(t, 21, edits[1].Range.End.Character)

	assert.Empty(t, refUseEditsInBody(root, body, lines, off, "ghost", "z"))
	// An inline link carries no Reference and is skipped.
	inline := []byte("[a](u)\n")
	ib, io := bodyAndFMOffset(inline)
	assert.Empty(t, refUseEditsInBody(parseBody(ib), ib, splitLines(inline), io, "a", "z"))
}

func TestRefUseEdit(t *testing.T) {
	src := []byte("---\ntitle: t\n---\nSee [text][Label].\n\n[label]: u\n")
	body, off := bodyAndFMOffset(src)
	root := parseBody(body)
	l := firstLink(t, root)

	e, ok := refUseEdit(l, l.Reference, body, splitLines(src), off, "new", newBodyLineIndex(body))
	require.True(t, ok)
	// Front matter shifts the line: "See ..." is file line index 3.
	assert.Equal(t, 3, e.Range.Start.Line)
	assert.Equal(t, 3, e.Range.End.Line)
	assert.Equal(t, 11, e.Range.Start.Character)
	assert.Equal(t, 16, e.Range.End.Character)
	assert.Equal(t, "new", e.NewText)

	// A node with no recorded source position cannot be anchored.
	bare := ast.NewLink()
	bare.Reference = l.Reference
	_, ok = refUseEdit(bare, bare.Reference, body, splitLines(src), off, "new", newBodyLineIndex(body))
	assert.False(t, ok)
}

func TestRefUseEdit_UTF16ColumnsAndMultiLine(t *testing.T) {
	// The emoji is 4 UTF-8 bytes but 2 UTF-16 units; é is 2 bytes, 1 unit.
	src := []byte("😀 é [t][docs]\n\n[docs]: u\n")
	body, off := bodyAndFMOffset(src)
	l := firstLink(t, parseBody(body))
	e, ok := refUseEdit(l, l.Reference, body, splitLines(src), off, "new", newBodyLineIndex(body))
	require.True(t, ok)
	assert.Equal(t, 0, e.Range.Start.Line)
	assert.Equal(t, 9, e.Range.Start.Character)
	assert.Equal(t, 13, e.Range.End.Character)

	// A label that wraps across lines ends on a later line.
	wrapped := []byte("[t][two\nwords]\n\n[two words]: u\n")
	wb, wo := bodyAndFMOffset(wrapped)
	wl := firstLink(t, parseBody(wb))
	we, ok := refUseEdit(wl, wl.Reference, wb, splitLines(wrapped), wo, "new", newBodyLineIndex(wb))
	require.True(t, ok)
	assert.Equal(t, 0, we.Range.Start.Line)
	assert.Equal(t, 1, we.Range.End.Line)
	assert.Equal(t, 5, we.Range.End.Character)
}

func TestLinkTextBounds(t *testing.T) {
	body := []byte("See [the spec][x] now.\n\n[x]: u\n")
	l := firstLink(t, parseBody(body))
	start, end := linkTextBounds(l, body)
	assert.Equal(t, "the spec", string(body[start:end]))

	// Inline markup in the text stays inside the bounds.
	marked := []byte("[**b** `]`][x]\n\n[x]: u\n")
	ml := firstLink(t, parseBody(marked))
	ms, me := linkTextBounds(ml, marked)
	assert.Equal(t, "**b** `]`", string(marked[ms:me]))

	// An empty-text link has an empty, anchored run.
	empty := []byte("[][x]\n\n[x]: u\n")
	el := firstLink(t, parseBody(empty))
	s, e := linkTextBounds(el, empty)
	assert.Equal(t, 1, s)
	assert.Equal(t, 1, e)

	// A node with no recorded position has no bounds.
	s, e = linkTextBounds(ast.NewLink(), body)
	assert.Equal(t, -1, s)
	assert.Equal(t, -1, e)

	// Bytes that no longer match the parsed link have no bounds: a
	// label that no longer matches the reference, a `[` that moved,
	// and a position past the end of the buffer.
	for _, other := range [][]byte{
		[]byte("See [the spec][y] now.\n"),
		[]byte("See (the spec][x] now.\n"),
		[]byte("See"),
	} {
		s, e = linkTextBounds(l, other)
		assert.Equal(t, -1, s, string(other))
		assert.Equal(t, -1, e, string(other))
	}
}

func TestBodyNewlineCount(t *testing.T) {
	assert.Equal(t, 0, bodyNewlineCount(nil))
	assert.Equal(t, 0, bodyNewlineCount([]byte("no newline")))
	assert.Equal(t, 1, bodyNewlineCount([]byte("a\n")))
	assert.Equal(t, 3, bodyNewlineCount([]byte("a\n\nb\nc")))
}

// TestLinkRef_RewritesUsesWithInlineMarkupInText covers reference
// uses whose display text is not plain text: emphasis, a code span
// (even one holding a `]`), a nested image, and an image reference.
// Each use must be rewritten along with the def, or the rename leaves
// a dangling label behind.
func TestLinkRef_RewritesUsesWithInlineMarkupInText(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"emphasis", "[**bold**][docs]\n\n[docs]: u\n", "[**bold**][ref]\n\n[ref]: u\n"},
		{"trailing emphasis", "[a *b*][docs]\n\n[docs]: u\n", "[a *b*][ref]\n\n[ref]: u\n"},
		{"code span", "[`code`][docs]\n\n[docs]: u\n", "[`code`][ref]\n\n[ref]: u\n"},
		{"code span with bracket", "[`a]`][docs]\n\n[docs]: u\n", "[`a]`][ref]\n\n[ref]: u\n"},
		{"nested image", "[![i](x.png)][docs]\n\n[docs]: u\n", "[![i](x.png)][ref]\n\n[ref]: u\n"},
		{"image full", "![alt][docs]\n\n[docs]: u\n", "![alt][ref]\n\n[ref]: u\n"},
		{"image shortcut", "![docs]\n\n[docs]: u\n", "![ref]\n\n[ref]: u\n"},
		{"empty text", "[][docs]\n\n[docs]: u\n", "[][ref]\n\n[ref]: u\n"},
		{"escaped bracket", "[a\\]b][docs]\n\n[docs]: u\n", "[a\\]b][ref]\n\n[ref]: u\n"},
		{
			"bracket in nested image destination",
			"[![i](x]y.png)][docs]\n\n[docs]: u\n", "[![i](x]y.png)][ref]\n\n[ref]: u\n",
		},
		{
			"bracket in raw HTML attribute",
			"[<b title=\"]\">x</b>][docs]\n\n[docs]: u\n", "[<b title=\"]\">x</b>][ref]\n\n[ref]: u\n",
		},
		{"nested image same label", "[![i][docs]][docs]\n\n[docs]: u\n", "[![i][ref]][ref]\n\n[ref]: u\n"},
		{"nested image collapsed", "[![docs][]][docs]\n\n[docs]: u\n", "[![ref][]][ref]\n\n[ref]: u\n"},
		{"nested image shortcut", "[![docs]][docs]\n\n[docs]: u\n", "[![ref]][ref]\n\n[ref]: u\n"},
		{
			"nested inline image with label-like destination",
			"[![i](x][docs)][docs]\n\n[docs]: u\n", "[![i](x][docs)][ref]\n\n[ref]: u\n",
		},
		{"bracket in autolink", "[<http://a]b>][docs]\n\n[docs]: u\n", "[<http://a]b>][ref]\n\n[ref]: u\n"},
		{"in blockquote", "> see [**b**][docs]\n\n[docs]: u\n", "> see [**b**][ref]\n\n[ref]: u\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edits, err := callLinkRef([]byte(tc.src), "docs", "ref")
			require.NoError(t, err)
			assert.Equal(t, tc.want, applyEditsToSource(t, tc.src, edits))
		})
	}
}

func TestInvalidLinkRefRune_TrailingBackslash(t *testing.T) {
	// A trailing unescaped backslash escapes the closing `]`, so the
	// def `[foo\]: u` no longer parses.
	assert.Equal(t, '\\', invalidLinkRefRune(`foo\`))
	assert.Equal(t, '\\', invalidLinkRefRune(`foo\\\`))
	// An escaped backslash (even run) and an inner one are fine.
	assert.Equal(t, rune(0), invalidLinkRefRune(`foo\\`))
	assert.Equal(t, rune(0), invalidLinkRefRune(`a\b`))
}

func TestLinkRef_TrailingBackslashRejected(t *testing.T) {
	_, err := callLinkRef([]byte("[a]\n\n[a]: u\n"), "a", `b\`)
	var runeErr InvalidLabelRuneError
	require.ErrorAs(t, err, &runeErr)
	assert.Equal(t, `label cannot end with an unescaped backslash`, err.Error())
}

func TestReferenceOf(t *testing.T) {
	src := []byte("[a][r] ![b][r] [c](u)\n\n[r]: u\n")
	root := parseBody(src)
	var refs []*ast.ReferenceLink
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			refs = append(refs, referenceOf(n))
		}
		return ast.WalkContinue, nil
	})
	var nonNil int
	for _, r := range refs {
		if r != nil {
			nonNil++
			assert.Equal(t, "r", string(r.Value))
		}
	}
	// The link and the image carry a reference; the inline link,
	// text, paragraph, and document do not.
	assert.Equal(t, 2, nonNil)
	assert.Nil(t, referenceOf(ast.NewText()))
}

func TestContentEnd(t *testing.T) {
	cases := []struct {
		src  string
		want string // the source prefix ending at contentEnd
	}{
		{"[a *b*][x]\n\n[x]: u\n", "[a *b"},
		{"[`]`][x]\n\n[x]: u\n", "[`]"},
		{"[<a title=\"]\">][x]\n\n[x]: u\n", "[<a title=\"]\">"},
		{"[<http://a]b>][x]\n\n[x]: u\n", "[<http://a]b>"},
		{"[][x]\n\n[x]: u\n", "["},
	}
	for _, tc := range cases {
		body := []byte(tc.src)
		l := firstLink(t, parseBody(body))
		assert.Equal(t, tc.want, string(body[:contentEnd(l, body, 1)]), tc.src)
	}
}

func TestClosingTextBracket(t *testing.T) {
	full := &ast.ReferenceLink{Type: ast.ReferenceLinkFull, Value: []byte("docs")}
	collapsed := &ast.ReferenceLink{Type: ast.ReferenceLinkCollapsed, Value: []byte("a b")}
	shortcut := &ast.ReferenceLink{Type: ast.ReferenceLinkShortcut, Value: []byte("a")}
	cases := []struct {
		name string
		body string
		from int
		ref  *ast.ReferenceLink
		want int
	}{
		{"full", "[t][docs]", 1, full, 2},
		{"full skips non-label bracket", "[x](y]z)][docs]", 1, full, 8},
		{"full skips non-matching label", "[a][x][docs]", 1, full, 5},
		{"full label case-folds", "[t][DOCS]", 1, full, 2},
		{"full unclosed label", "[t][docs", 1, full, -1},
		{"escaped bracket skipped", "[a\\]][docs]", 1, full, 4},
		{"collapsed", "[A  B][]", 1, collapsed, 5},
		{"collapsed needs []", "[a b] x", 1, collapsed, -1},
		{"shortcut", "[a] x", 1, shortcut, 2},
		{"blank line ends search", "[a\n\n]", 1, shortcut, -1},
		{"newline inside text", "[a\nb][docs]", 1, full, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, closingTextBracket([]byte(tc.body), 1, tc.from, tc.ref))
		})
	}
}

// firstImage returns the first image node in root.
func firstImage(t *testing.T, root ast.Node) *ast.Image {
	t.Helper()
	var found *ast.Image
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if img, ok := n.(*ast.Image); ok && entering && found == nil {
			found = img
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, found, "no image in body")
	return found
}

func TestImageEnd(t *testing.T) {
	cases := []struct {
		src  string
		want string // the source prefix ending at imageEnd
	}{
		{"![a](x(y)z) t\n", "![a](x(y)z)"},
		{"![a][docs] t\n\n[docs]: u\n", "![a][docs]"},
		{"![docs][] t\n\n[docs]: u\n", "![docs][]"},
		{"![docs] t\n\n[docs]: u\n", "![docs]"},
	}
	for _, tc := range cases {
		body := []byte(tc.src)
		img := firstImage(t, parseBody(body))
		assert.Equal(t, tc.want, string(body[:imageEnd(img, body)]), tc.src)
	}
	// Bytes that no longer match the parsed image have no end.
	body := []byte("![a][docs] t\n\n[docs]: u\n")
	img := firstImage(t, parseBody(body))
	assert.Equal(t, -1, imageEnd(img, []byte("(a][docs] t\n")))
	assert.Equal(t, -1, imageEnd(img, []byte("![a][docs")))
}

func TestParenEnd(t *testing.T) {
	assert.Equal(t, 3, parenEnd([]byte("(a)"), 0))
	assert.Equal(t, 7, parenEnd([]byte("(a(b)c)"), 0))
	assert.Equal(t, 5, parenEnd([]byte(`(a\))`), 0))
	assert.Equal(t, -1, parenEnd([]byte("(a"), 0))
}

func TestInlineTextBracket(t *testing.T) {
	assert.Equal(t, 2, inlineTextBracket([]byte("[a](u)"), 1))
	// A `]` not followed by `(` and an escaped `]` are skipped.
	assert.Equal(t, 5, inlineTextBracket([]byte("[a] b](u)"), 1))
	assert.Equal(t, 7, inlineTextBracket([]byte(`[a\](u)](v)`), 1))
	assert.Equal(t, -1, inlineTextBracket([]byte("[a\n\n](u)"), 1))
	assert.Equal(t, -1, inlineTextBracket([]byte("[a]"), 1))
}
