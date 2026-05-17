package rename

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// memWorkspace is a concrete Workspace backed by a real index over an
// in-memory file set. It is not a mock — the edge graph is the
// production index.New + BuildSerial, the same path the LSP server and
// the CLI use; only the byte source is a map instead of disk.
type memWorkspace struct {
	idx   *index.Index
	files map[string][]byte
}

func newMemWorkspace(files map[string]string) *memWorkspace {
	bytesMap := make(map[string][]byte, len(files))
	rels := make([]string, 0, len(files))
	for rel, body := range files {
		n := index.NormalizePath(rel)
		bytesMap[n] = []byte(body)
		rels = append(rels, n)
	}
	idx := index.New(".")
	idx.BuildSerial(rels, func(rel string) ([]byte, error) {
		return bytesMap[rel], nil
	})
	return &memWorkspace{idx: idx, files: bytesMap}
}

func (w *memWorkspace) IncomingAnchorEdges(file, slug string) []index.Edge {
	return w.idx.IncomingEdges(file, slug)
}

func (w *memWorkspace) Files() []string { return w.idx.Files() }

func (w *memWorkspace) Resolve(file string) (string, []byte, bool) {
	n := index.NormalizePath(file)
	b, ok := w.files[n]
	return n, b, ok
}

func TestHeading_RewritesCrossFileAnchorsAndRefDef(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n\nBody.\n\n## Config\n\ntext\n",
		"b.md": "See [setup](a.md#setup) and [cfg](a.md#config).\n\n[ref]: a.md#setup\n",
	})
	src := ws.files["a.md"]
	changes, err := Heading(ws, "a.md", "a.md", src, 1, "Setup", "Install")
	require.NoError(t, err)

	// a.md: just the heading-text edit.
	aEdits := changes["a.md"]
	require.Len(t, aEdits, 1)
	assert.Equal(t, "Install", aEdits[0].NewText)
	assert.Equal(t, 0, aEdits[0].Range.Start.Line)

	// b.md: the anchor link fragment + the ref-def destination, both
	// rewritten setup → install.
	bEdits := changes["b.md"]
	require.Len(t, bEdits, 2)
	for _, e := range bEdits {
		assert.Equal(t, "install", e.NewText)
	}
}

func TestHeading_SameFileAnchorSharesKey(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Top\n\nJump to [here](#top).\n",
	})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Top", "Start")
	require.NoError(t, err)
	// Heading edit + same-file anchor edit both land under "a.md".
	require.Len(t, changes, 1)
	require.Len(t, changes["a.md"], 2)
}

func TestHeading_NoOpWhenUnchanged(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "  Title  ")
	require.NoError(t, err)
	assert.Empty(t, changes)
	assert.NotNil(t, changes)
}

func TestHeading_ControlRuneRejected(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	_, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "two\nlines")
	var ire InvalidHeadingRuneError
	require.True(t, errors.As(err, &ire))
	assert.Equal(t, '\n', ire.Rune)
	assert.Equal(t, `heading text cannot contain '\n'`, ire.Error())
}

func TestHeading_EmptySlugRejected(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	_, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "...")
	assert.ErrorIs(t, err, ErrEmptyHeadingSlug)
}

func TestHeading_CollisionNamesConflict(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Alpha\n\n## Beta\n",
	})
	_, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Alpha", "Beta")
	var hce HeadingCollisionError
	require.True(t, errors.As(err, &hce))
	assert.Equal(t, "Beta", hce.Conflict)
	assert.Equal(t, "rename would collide with heading Beta", hce.Error())
}

func TestHeading_SameBaseSlugRefreshAllowed(t *testing.T) {
	// "Title" → "title" keeps the same bare slug; the collision check
	// is skipped and the heading edit still emits.
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "title")
	require.NoError(t, err)
	require.Len(t, changes["a.md"], 1)
}

func TestHeading_SetextHeadingLine(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "Title\n=====\n"})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "Renamed")
	require.NoError(t, err)
	require.Len(t, changes["a.md"], 1)
	assert.Equal(t, "Renamed", changes["a.md"][0].NewText)
}

func TestHeading_TargetLineNotAHeading(t *testing.T) {
	// computeSlugRemap returns no remap when the line isn't a heading;
	// only the (degenerate) heading-line edit is produced.
	ws := newMemWorkspace(map[string]string{"a.md": "# Real\n\nprose\n"})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 3, "prose", "other")
	require.NoError(t, err)
	require.Len(t, changes, 1)
}

func TestHeading_DisambiguatorShift(t *testing.T) {
	// Two "Dup" headings; renaming the first away frees the bare slug
	// so the second's `dup-2` collapses to `dup`. An incoming link to
	// `dup-2` is rewritten to `dup`.
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Dup\n\n## Dup\n",
		"b.md": "[x](a.md#dup-1)\n",
	})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Dup", "Unique")
	require.NoError(t, err)
	require.Len(t, changes["b.md"], 1)
	assert.Equal(t, "dup", changes["b.md"][0].NewText)
}

func TestHeading_StaleEdgeSkipped(t *testing.T) {
	// An incoming edge whose source file the workspace can't resolve
	// is skipped rather than failing the whole rename.
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "[x](a.md#setup)\n",
	})
	delete(ws.files, "b.md") // edge survives in idx; bytes vanish
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	require.Len(t, changes["a.md"], 1)
	assert.Empty(t, changes["b.md"])
}

func TestHeading_AngleBracketRefDefDestination(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "[ref]: <a.md#setup>\n",
	})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	require.Len(t, changes["b.md"], 1)
	e := changes["b.md"][0]
	assert.Equal(t, "install", e.NewText)
	// The edit stays inside the angle brackets.
	assert.Equal(t, e.Range.Start.Line, e.Range.End.Line)
}

func TestHeading_LocalAnchorRefDefDestination(t *testing.T) {
	// `[ref]: #setup` is an anchor-only def in the heading's own file.
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n\n[ref]: #setup\n",
	})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	// Heading line + the local ref-def destination.
	require.Len(t, changes["a.md"], 2)
}

func TestHeading_RefDefInCodeBlockNotRewritten(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "```\n[ref]: a.md#setup\n```\n",
	})
	changes, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	assert.Empty(t, changes["b.md"])
}

// --- direct helper coverage for branches Heading can't easily drive --

func TestFirstControlRune(t *testing.T) {
	assert.Equal(t, '\r', firstControlRune("a\rb"))
	assert.Equal(t, rune(0), firstControlRune("clean"))
}

func TestHeadingTextEdit_OutOfRange(t *testing.T) {
	_, ok := headingTextEdit([]byte("# H\n"), 9, "X")
	assert.False(t, ok)
}

func TestAtxHeadingTextByteRange(t *testing.T) {
	t.Run("trailing hash run", func(t *testing.T) {
		s, e, ok := atxHeadingTextByteRange([]byte("## Title ##"))
		require.True(t, ok)
		assert.Equal(t, "Title", string([]byte("## Title ##")[s:e]))
	})
	t.Run("not atx", func(t *testing.T) {
		_, _, ok := atxHeadingTextByteRange([]byte("plain text"))
		assert.False(t, ok)
	})
	t.Run("empty heading", func(t *testing.T) {
		s, e, ok := atxHeadingTextByteRange([]byte("### "))
		require.True(t, ok)
		assert.Equal(t, s, e)
	})
	t.Run("hash#text not closing", func(t *testing.T) {
		s, e, ok := atxHeadingTextByteRange([]byte("# foo#bar"))
		require.True(t, ok)
		assert.Equal(t, "foo#bar", string([]byte("# foo#bar")[s:e]))
	})
}

func TestAtxHeadingTextStart(t *testing.T) {
	_, ok := atxHeadingTextStart([]byte("    # x")) // >3 leading spaces
	assert.False(t, ok)
	_, ok = atxHeadingTextStart([]byte("####### x")) // level 7
	assert.False(t, ok)
	_, ok = atxHeadingTextStart([]byte("##foo")) // no space after markers
	assert.False(t, ok)
	i, ok := atxHeadingTextStart([]byte("#\tx")) // tab separator
	require.True(t, ok)
	assert.Equal(t, 2, i)
	_, ok = atxHeadingTextStart([]byte("no hash"))
	assert.False(t, ok)
}

func TestTrimTrailingHashRun(t *testing.T) {
	assert.Equal(t, 0, trimTrailingHashRun([]byte(""), 0, 0)) // end<=start
	row := []byte("# abc")
	assert.Equal(t, 5, trimTrailingHashRun(row, 2, 5)) // no trailing #
	row = []byte("# a#")
	assert.Equal(t, 4, trimTrailingHashRun(row, 2, 4)) // # not preceded by space
	row = []byte("#  ###")
	assert.Equal(t, 1, trimTrailingHashRun(row, 1, 6)) // k<=start after run
}

func TestTrimmedRange(t *testing.T) {
	s, e := trimmedRange([]byte("  hi  "))
	assert.Equal(t, "hi", string([]byte("  hi  ")[s:e]))
}

func TestSlugRemapPairs(t *testing.T) {
	got := slugRemapPairs(
		[]string{"a", "", "b", "b"},
		[]string{"a", "x", "c", "z"},
	)
	// "a"→"a" unchanged skipped; ""→"x" skipped; "b" first-wins → "c".
	assert.Equal(t, map[string]string{"b": "c"}, got)
}

func TestAssignSlugs(t *testing.T) {
	got := assignSlugs([]string{"Dup", "...", "Dup", "Dup"})
	assert.Equal(t, []string{"dup", "", "dup-1", "dup-2"}, got)
}

func TestDestBounds(t *testing.T) {
	t.Run("escaped parens", func(t *testing.T) {
		row := []byte(`[t](foo\(bar\)#sec)`)
		o, c, ok := destBounds(row, 0)
		require.True(t, ok)
		assert.Equal(t, `foo\(bar\)#sec`, string(row[o:c]))
	})
	t.Run("no destination", func(t *testing.T) {
		_, _, ok := destBounds([]byte("[t] no paren"), 0)
		assert.False(t, ok)
	})
	t.Run("unclosed", func(t *testing.T) {
		_, _, ok := destBounds([]byte("[t](unclosed"), 0)
		assert.False(t, ok)
	})
	t.Run("escaped bracket in text", func(t *testing.T) {
		row := []byte(`[a\]b](u#s)`)
		o, c, ok := destBounds(row, 0)
		require.True(t, ok)
		assert.Equal(t, "u#s", string(row[o:c]))
	})
}

func TestIsBackslashEscaped(t *testing.T) {
	row := []byte(`a\)b`)
	assert.True(t, isBackslashEscaped(row, 2))
	row = []byte(`a\\)b`)
	assert.False(t, isBackslashEscaped(row, 3))
}

func TestIndexOfHash(t *testing.T) {
	assert.Equal(t, 3, indexOfHash([]byte("abc#frag"), 0, 8))
	assert.Equal(t, -1, indexOfHash([]byte("nohash"), 0, 6))
}

func TestFragmentEnd(t *testing.T) {
	row := []byte("abc#frag>x")
	assert.Equal(t, 8, fragmentEnd(row, 4, len(row)))     // stops at '>'
	assert.Equal(t, 2, fragmentEnd([]byte("ab x"), 0, 4)) // stops at space
	assert.Equal(t, 3, fragmentEnd([]byte("abc"), 0, 3))  // runs to close
}

func TestComputeSlugRemap(t *testing.T) {
	old, neu, conflict := computeSlugRemap([]byte("# A\n\n## B\n"), 1, "C")
	assert.Empty(t, conflict)
	assert.Equal(t, []string{"a", "b"}, old)
	assert.Equal(t, []string{"c", "b"}, neu)

	_, _, conflict = computeSlugRemap([]byte("# A\n\n## B\n"), 1, "B")
	assert.Equal(t, "B", conflict)

	old, neu, conflict = computeSlugRemap([]byte("# A\n"), 99, "X")
	assert.Nil(t, old)
	assert.Nil(t, neu)
	assert.Empty(t, conflict)
}

func TestWalkAllHeadings(t *testing.T) {
	body := []byte("# A\n\nprose\n\n## B\n")
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	hs := walkAllHeadings(root, body)
	require.Len(t, hs, 2)
	assert.Equal(t, "A", hs[0].text)
	assert.Equal(t, "B", hs[1].text)
}

func TestSlicesOfText(t *testing.T) {
	got := slicesOfText([]headingWalk{{text: "x"}, {text: "y"}})
	assert.Equal(t, []string{"x", "y"}, got)
}

func TestSkipLeadingSpaces(t *testing.T) {
	assert.Equal(t, 2, skipLeadingSpaces([]byte("  x"), 3))
	assert.Equal(t, 3, skipLeadingSpaces([]byte("     x"), 3)) // capped at max
	assert.Equal(t, 0, skipLeadingSpaces([]byte("x"), 3))
}

func TestTrimRightSpace(t *testing.T) {
	row := []byte("ab \t ")
	assert.Equal(t, 2, trimRightSpace(row, 0, len(row)))
	assert.Equal(t, 0, trimRightSpace([]byte("   "), 0, 3))
}

func TestInvalidHeadingRuneError_Error(t *testing.T) {
	assert.Equal(t, `heading text cannot contain '\n'`,
		InvalidHeadingRuneError{Rune: '\n'}.Error())
}

func TestHeadingCollisionError_Error(t *testing.T) {
	assert.Equal(t, "rename would collide with heading Intro",
		HeadingCollisionError{Conflict: "Intro"}.Error())
}

func TestAppendAnchorEditsForHeading(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "[x](a.md#setup)\n",
	})
	changes := map[string][]Edit{}
	appendAnchorEditsForHeading(changes, ws, "a.md", "setup", "install")
	require.Len(t, changes["b.md"], 1)
	assert.Equal(t, "install", changes["b.md"][0].NewText)

	// An edge whose source can't be resolved is skipped.
	delete(ws.files, "b.md")
	changes = map[string][]Edit{}
	appendAnchorEditsForHeading(changes, ws, "a.md", "setup", "install")
	assert.Empty(t, changes)
}

func TestFragmentMatchesSlug(t *testing.T) {
	assert.True(t, fragmentMatchesSlug([]byte("Docs%20API"), "docs-api"))
	assert.True(t, fragmentMatchesSlug([]byte("bad%zz"), "badzz")) // invalid escape → raw
	assert.False(t, fragmentMatchesSlug([]byte("other"), "setup"))
}

func TestAnchorFragmentBytes(t *testing.T) {
	t.Run("negative textStart clamps", func(t *testing.T) {
		row := []byte("[t](#setup)")
		s, e, ok := anchorFragmentBytes(row, -5, "setup")
		require.True(t, ok)
		assert.Equal(t, "setup", string(row[s:e]))
	})
	t.Run("textStart past row", func(t *testing.T) {
		_, _, ok := anchorFragmentBytes([]byte("[t](#s)"), 99, "s")
		assert.False(t, ok)
	})
	t.Run("no hash", func(t *testing.T) {
		_, _, ok := anchorFragmentBytes([]byte("[t](file.md)"), 0, "x")
		assert.False(t, ok)
	})
	t.Run("slug mismatch", func(t *testing.T) {
		_, _, ok := anchorFragmentBytes([]byte("[t](#other)"), 0, "setup")
		assert.False(t, ok)
	})
}

func TestRefDefColonOffset(t *testing.T) {
	assert.Equal(t, 4, refDefColonOffset([]byte("[ab]: u")))
	assert.Equal(t, 6, refDefColonOffset([]byte("  [ab]: u"))) // ≤3 leading spaces
	assert.Equal(t, -1, refDefColonOffset([]byte("no bracket")))
	assert.Equal(t, -1, refDefColonOffset([]byte("[ab no close")))
	assert.Equal(t, -1, refDefColonOffset([]byte("[ab] no colon")))
	assert.Equal(t, -1, refDefColonOffset([]byte("[ab]"))) // `]` is last byte
}

func TestAnchorEditForEdge_SkipPaths(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"b.md": "[x](a.md#setup)\n"})
	// Source file the workspace can't resolve.
	_, _, ok := anchorEditForEdge(ws, index.Edge{SourceFile: "gone.md", SourceLine: 1, SourceCol: 1}, "setup", "x")
	assert.False(t, ok)
	// SourceLine past EOF.
	_, _, ok = anchorEditForEdge(ws, index.Edge{SourceFile: "b.md", SourceLine: 99, SourceCol: 1}, "setup", "x")
	assert.False(t, ok)
	// Fragment can't be located on the line.
	ws2 := newMemWorkspace(map[string]string{"b.md": "no link here\n"})
	_, _, ok = anchorEditForEdge(ws2, index.Edge{SourceFile: "b.md", SourceLine: 1, SourceCol: 1}, "setup", "x")
	assert.False(t, ok)
}

func TestAnchorFragmentBytes_NoDestination(t *testing.T) {
	_, _, ok := anchorFragmentBytes([]byte("plain text no link"), 0, "x")
	assert.False(t, ok)
}

func TestDestBounds_NestedUnescapedParens(t *testing.T) {
	row := []byte("[t](a(b)#s)")
	o, c, ok := destBounds(row, 0)
	require.True(t, ok)
	assert.Equal(t, "a(b)#s", string(row[o:c]))
}

func TestAppendRefDefDestEditsForHeading_SkipsUnresolvable(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "[ref]: a.md#setup\n",
	})
	delete(ws.files, "b.md") // still in idx.Files(), bytes gone
	changes := map[string][]Edit{}
	appendRefDefDestEditsForHeading(changes, ws, "a.md", "setup", "install")
	assert.Empty(t, changes)
}

func TestRefDefDestEditForMatch_ColonAndEmptyDest(t *testing.T) {
	// m is only read as m[2] (a body offset → line). A zero offset
	// maps to the single fixture line, isolating the colon / empty-
	// destination guards from regex-shape concerns.
	m := []int{0, 0, 0, 0}

	// No `[label]:` colon on the line.
	_, ok := refDefDestEditForMatch(
		[]byte("plain text"), [][]byte{[]byte("plain text")},
		0, m, "b.md", "a.md", "setup", "x")
	assert.False(t, ok)

	// Colon present but nothing after it — empty destination.
	_, ok = refDefDestEditForMatch(
		[]byte("[a]:"), [][]byte{[]byte("[a]:")},
		0, m, "b.md", "a.md", "setup", "x")
	assert.False(t, ok)
}

func TestAppendRefDefDestEditsForHeading_NonMatchingDefSkipped(t *testing.T) {
	// b.md has a real ref-def, but its destination points at a
	// different anchor, so refDefDestEditForMatch returns false and
	// the inner skip fires.
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "[ref]: a.md#other\n",
	})
	changes := map[string][]Edit{}
	appendRefDefDestEditsForHeading(changes, ws, "a.md", "setup", "install")
	assert.Empty(t, changes)
}

func TestRefDefDestRange(t *testing.T) {
	t.Run("bare", func(t *testing.T) {
		row := []byte(`[a]: url "title"`)
		s, e := refDefDestRange(row, 4)
		assert.Equal(t, "url", string(row[s:e]))
	})
	t.Run("angle bracket", func(t *testing.T) {
		row := []byte(`[a]: <u#s>`)
		s, e := refDefDestRange(row, 4)
		assert.Equal(t, "u#s", string(row[s:e]))
	})
	t.Run("escaped inside angle", func(t *testing.T) {
		row := []byte(`[a]: <u\>v>`)
		s, e := refDefDestRange(row, 4)
		assert.Equal(t, `u\>v`, string(row[s:e]))
	})
	t.Run("unterminated angle falls back to bare", func(t *testing.T) {
		row := []byte(`[a]: <unterminated`)
		s, e := refDefDestRange(row, 4)
		assert.Equal(t, "<unterminated", string(row[s:e]))
	})
}

func TestRefDefParseTarget(t *testing.T) {
	_, ok := refDefParseTarget("")
	assert.False(t, ok)
	_, ok = refDefParseTarget("//proto-relative")
	assert.False(t, ok)
	_, ok = refDefParseTarget("ht\ttp://x\n")
	assert.False(t, ok) // url.Parse error on control char
	_, ok = refDefParseTarget("https://example.com")
	assert.False(t, ok) // scheme/host set
	tg, ok := refDefParseTarget("#frag")
	require.True(t, ok)
	assert.True(t, tg.localAnchor)
	_, ok = refDefParseTarget("?onlyquery")
	assert.False(t, ok) // empty path, no fragment
	tg, ok = refDefParseTarget("a.md#s")
	require.True(t, ok)
	assert.Equal(t, "a.md", tg.path)
}

func TestRefDefDestPointsAt(t *testing.T) {
	assert.False(t, refDefDestPointsAt([]byte("https://x"), "b.md", "a.md", "s"))
	assert.False(t, refDefDestPointsAt([]byte("a.md#other"), "b.md", "a.md", "setup"))
	assert.True(t, refDefDestPointsAt([]byte("#setup"), "a.md", "a.md", "setup"))
	assert.False(t, refDefDestPointsAt([]byte("#setup"), "b.md", "a.md", "setup"))
}

func TestStableSortEdits(t *testing.T) {
	changes := map[string][]Edit{
		"f": {
			{Range: Range{Start: Position{Line: 1, Character: 0}}},
			{Range: Range{Start: Position{Line: 5, Character: 2}}},
			{Range: Range{Start: Position{Line: 5, Character: 0}}},
		},
	}
	stableSortEdits(changes)
	got := changes["f"]
	assert.Equal(t, 5, got[0].Range.Start.Line)
	assert.Equal(t, 2, got[0].Range.Start.Character)
	assert.Equal(t, 5, got[1].Range.Start.Line)
	assert.Equal(t, 1, got[2].Range.Start.Line)
}

func TestRefDefDestEditForMatch_BadInputs(t *testing.T) {
	body := []byte("[a]: a.md#setup\n")
	lines := splitLines(body)
	matches := index.RefDefRegexpMatches(body)
	require.NotEmpty(t, matches)
	m := matches[0]

	// fileLine past the line table.
	_, ok := refDefDestEditForMatch(body, [][]byte{}, 0, m, "b.md", "a.md", "setup", "x")
	assert.False(t, ok)

	// Destination doesn't point at the heading.
	_, ok = refDefDestEditForMatch(body, lines, 0, m, "b.md", "z.md", "setup", "x")
	assert.False(t, ok)

	// Happy path.
	e, ok := refDefDestEditForMatch(body, lines, 0, m, "b.md", "a.md", "setup", "install")
	require.True(t, ok)
	assert.Equal(t, "install", e.NewText)
}
