package refactor

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (w *memWorkspace) IncomingPathEdges(file string) []index.Edge {
	return w.idx.IncomingPathEdges(file)
}

func (w *memWorkspace) IncomingWikilinkEdges(stem string) []index.Edge {
	return w.idx.IncomingWikilinkEdges(stem)
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
	changes, err := callHeading(ws, "a.md", "a.md", src, 1, "Setup", "Install")
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

func TestHeading_SlugsRenderedTextNotRawMarkup(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n\n[ref]: x.md\n",
		"b.md": "See [setup](a.md#setup).\n",
	})
	// The renamed heading renders as "Install", so its anchor is
	// #install, not the raw-markup slug #installxmd.
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "[Install](x.md)")
	require.NoError(t, err)
	require.Len(t, changes["b.md"], 1)
	assert.Equal(t, "install", changes["b.md"][0].NewText)

	// A full reference link renders its text only when the label is
	// defined in the file, so the slug follows the file's ref-defs.
	changes, err = callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "[Install][ref]")
	require.NoError(t, err)
	assert.Equal(t, "install", changes["b.md"][0].NewText)
}

func TestHeading_EmptyRenderedSlugRejected(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	_, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "[](x.md)")
	assert.ErrorIs(t, err, ErrEmptyHeadingSlug)
}

func TestRenderedHeadingText(t *testing.T) {
	src := []byte("---\nk: v\n---\n# Setup\n")
	assert.Equal(t, "Install", renderedHeadingText(src, 4, "**Install**"))
	// A line that is not a heading falls back to the raw text.
	assert.Equal(t, "raw", renderedHeadingText([]byte("prose\n"), 1, "raw"))
	assert.Equal(t, "raw", renderedHeadingText([]byte("# A\n"), 9, "raw"))
}

func TestHeading_PlanCarriesNoFileOp(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n\nBody.\n",
	})
	plan, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	// A heading rename edits symbols in place; it never relocates a file.
	assert.Nil(t, plan.FileOp)
	assert.NotEmpty(t, plan.Edits["a.md"])
}

func TestHeading_SameFileAnchorSharesKey(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Top\n\nJump to [here](#top).\n",
	})
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Top", "Start")
	require.NoError(t, err)
	// Heading edit + same-file anchor edit both land under "a.md".
	require.Len(t, changes, 1)
	require.Len(t, changes["a.md"], 2)
}

func TestHeading_NoOpWhenUnchanged(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "  Title  ")
	require.NoError(t, err)
	assert.Empty(t, changes)
	assert.NotNil(t, changes)
}

func TestHeading_ControlRuneRejected(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	_, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "two\nlines")
	var ire InvalidHeadingRuneError
	require.True(t, errors.As(err, &ire))
	assert.Equal(t, '\n', ire.Rune)
	assert.Equal(t, `heading text cannot contain '\n'`, ire.Error())
}

func TestHeading_EmptySlugRejected(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	_, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "...")
	assert.ErrorIs(t, err, ErrEmptyHeadingSlug)
}

func TestHeading_CollisionNamesConflict(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Alpha\n\n## Beta\n",
	})
	_, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Alpha", "Beta")
	var hce HeadingCollisionError
	require.True(t, errors.As(err, &hce))
	assert.Equal(t, "Beta", hce.Conflict)
	assert.Equal(t, "rename would collide with heading Beta", hce.Error())
}

func TestHeading_SameBaseSlugRefreshAllowed(t *testing.T) {
	// "Title" → "title" keeps the same bare slug; the collision check
	// is skipped and the heading edit still emits.
	ws := newMemWorkspace(map[string]string{"a.md": "# Title\n"})
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "title")
	require.NoError(t, err)
	require.Len(t, changes["a.md"], 1)
}

func TestHeading_SetextHeadingLine(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "Title\n=====\n"})
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Title", "Renamed")
	require.NoError(t, err)
	require.Len(t, changes["a.md"], 1)
	assert.Equal(t, "Renamed", changes["a.md"][0].NewText)
}

func TestHeading_TargetLineNotAHeading(t *testing.T) {
	// computeSlugRemap returns no remap when the line isn't a heading;
	// only the (degenerate) heading-line edit is produced.
	ws := newMemWorkspace(map[string]string{"a.md": "# Real\n\nprose\n"})
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 3, "prose", "other")
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
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Dup", "Unique")
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
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	require.Len(t, changes["a.md"], 1)
	assert.Empty(t, changes["b.md"])
}

func TestHeading_AngleBracketRefDefDestination(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "[ref]: <a.md#setup>\n",
	})
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
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
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	// Heading line + the local ref-def destination.
	require.Len(t, changes["a.md"], 2)
}

func TestHeading_RefDefInCodeBlockNotRewritten(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "```\n[ref]: a.md#setup\n```\n",
	})
	changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], 1, "Setup", "Install")
	require.NoError(t, err)
	assert.Empty(t, changes["b.md"])
}

// --- direct helper coverage for branches Heading can't easily drive --

func TestFindHeadingLineIn(t *testing.T) {
	ps := parseSource([]byte("---\ntitle: x\n---\n# Intro\n\n## Setup\n"))
	line, ok := findHeadingLineIn(ps, "Setup")
	require.True(t, ok)
	assert.Equal(t, 6, line) // 3 front-matter lines + body line 3

	_, ok = findHeadingLineIn(ps, "Missing")
	assert.False(t, ok)
}

func TestFirstControlRune(t *testing.T) {
	assert.Equal(t, '\r', firstControlRune("a\rb"))
	assert.Equal(t, rune(0), firstControlRune("clean"))
}

func TestHeadingTextEdit_OutOfRange(t *testing.T) {
	_, ok := headingTextEdit([]byte("# H\n"), 9, "X")
	assert.False(t, ok)
}

// TestAtxHeadingTextByteRange covers the heading-line parsing that
// HeadingTextRange (shared by refactor's rename engine and the LSP's
// prepareRename range in internal/lsp/rename.go) falls through to for
// an ATX line. These cases drive both surfaces' rename popups so they
// need to stay tight against the documented behavior.
func TestAtxHeadingTextByteRange(t *testing.T) {
	cases := []struct {
		row                string
		wantOK             bool
		wantStart, wantEnd int
	}{
		{"# Hello", true, 2, 7},
		{"## Hi there", true, 3, 11},
		{"### Setup ###", true, 4, 9}, // trailing " ###" stripped
		{"###### Six", true, 7, 10},
		{"   ## Indented", true, 6, 14},
		{"#NoSpace", false, 0, 0},
		{"####### TooMany", false, 0, 0},
		{"plain text", false, 0, 0},
		{"## ", true, 3, 3}, // empty heading: zero-width range
		{"##\tWith tab", true, 3, 11},
		{"##  spaced", true, 4, 10},
		{"##  spaced  ", true, 4, 10},
		{"## foo ###", true, 3, 6},
		{"## foo###", true, 3, 9}, // no preceding space — hashes kept as text
		{"##", true, 2, 2},
	}
	for _, tc := range cases {
		start, end, ok := atxHeadingTextByteRange([]byte(tc.row))
		assert.Equal(t, tc.wantOK, ok, "row=%q", tc.row)
		if !ok {
			continue
		}
		assert.Equal(t, tc.wantStart, start, "start row=%q", tc.row)
		assert.Equal(t, tc.wantEnd, end, "end row=%q", tc.row)
	}
}

func TestAtxHeadingTextStart(t *testing.T) {
	cases := []struct {
		row    string
		wantI  int
		wantOK bool
	}{
		{"    # x", 0, false},   // >3 leading spaces
		{"####### x", 0, false}, // level 7
		{"##foo", 0, false},     // no space after markers
		{"#\tx", 2, true},       // tab separator
		{"no hash", 0, false},
		{"", 0, false},
		{"# Hello", 2, true},
		{"## Hi", 3, true},
		{"###### Six", 7, true},
		{"   ## Indented", 6, true},
		{"##\tTab", 3, true},
	}
	for _, tc := range cases {
		i, ok := atxHeadingTextStart([]byte(tc.row))
		assert.Equal(t, tc.wantOK, ok, "row=%q", tc.row)
		assert.Equal(t, tc.wantI, i, "row=%q", tc.row)
	}
}

func TestTrimTrailingHashRun(t *testing.T) {
	cases := []struct {
		row        string
		start, end int
		want       int
	}{
		{"", 0, 0, 0},              // end<=start
		{"# abc", 2, 5, 5},         // no trailing #
		{"# a#", 2, 4, 4},          // # not preceded by space
		{"#  ###", 1, 6, 1},        // k<=start after run
		{"## Setup ###", 3, 12, 8}, // trailing " ###" stripped; text ends at 8
		{"## Setup ##", 3, 11, 8},  // trailing " ##" stripped
		{"## Setup #", 3, 10, 8},   // trailing " #" stripped
		{"## Setup#", 3, 9, 9},     // no preceding space — kept
		{"## Setup", 3, 8, 8},      // no trailing hash — unchanged
		{"## Setup   ", 3, 11, 11}, // trailing spaces only, no hash — unchanged
	}
	for _, tc := range cases {
		got := trimTrailingHashRun([]byte(tc.row), tc.start, tc.end)
		assert.Equal(t, tc.want, got, "row=%q", tc.row)
	}
}

// TestHeadingTextRange covers HeadingTextRange's own two branches: the
// ATX byte range it delegates to (already exercised in detail by
// TestAtxHeadingTextByteRange above) and the non-ATX fallback to the
// full trimmed line, e.g. a setext heading's text line.
func TestHeadingTextRange(t *testing.T) {
	start, end := HeadingTextRange([]byte("## Hi there"))
	assert.Equal(t, 3, start)
	assert.Equal(t, 11, end)

	start, end = HeadingTextRange([]byte("  Setext Title  "))
	assert.Equal(t, 2, start)
	assert.Equal(t, 14, end)
}

func TestTrimmedRange(t *testing.T) {
	cases := []struct {
		row                string
		wantStart, wantEnd int
	}{
		{"  hello  ", 2, 7},
		{"nospace", 0, 7},
		{"   ", 3, 3},
		{"  text\t", 2, 6},
	}
	for _, tc := range cases {
		start, end := trimmedRange([]byte(tc.row))
		assert.Equal(t, tc.wantStart, start, "start row=%q", tc.row)
		assert.Equal(t, tc.wantEnd, end, "end row=%q", tc.row)
	}
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
	cases := []struct {
		row   string
		max   int
		wantI int
	}{
		{"  x", 3, 2},    // fewer than max
		{"     x", 3, 3}, // more than max — capped
		{"x", 3, 0},      // none
		{"", 3, 0},       // empty
		{"   abc", 3, 3}, // exactly max
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantI, skipLeadingSpaces([]byte(tc.row), tc.max), "row=%q", tc.row)
	}
}

func TestTrimRightSpace(t *testing.T) {
	cases := []struct {
		row        string
		start, end int
		want       int
	}{
		{"ab \t ", 0, 5, 2},   // trailing space+tab+space
		{"   ", 0, 3, 0},      // all whitespace
		{"hello  ", 0, 7, 5},  // trailing spaces
		{"hello\t ", 0, 7, 5}, // trailing tab+space
		{"hello", 0, 5, 5},    // no trailing whitespace — unchanged
	}
	for _, tc := range cases {
		got := trimRightSpace([]byte(tc.row), tc.start, tc.end)
		assert.Equal(t, tc.want, got, "row=%q", tc.row)
	}
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

func TestHeading_ImageInLinkAnchor(t *testing.T) {
	// [![icon](icon.png)](a.md#setup) — an image-wrapped link.
	// firstTextOffset finds the alt-text node "icon" inside the image,
	// which sits before the outer ](a.md#setup). anchorFragmentBytes
	// must scan past the image destination and locate the outer fragment.
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n\nBody.\n",
		"b.md": "[![icon](icon.png)](a.md#setup)\n",
	})
	src := ws.files["a.md"]
	changes, err := callHeading(ws, "a.md", "a.md", src, 1, "Setup", "Install")
	require.NoError(t, err)
	bEdits := changes["b.md"]
	require.Len(t, bEdits, 1)
	assert.Equal(t, "install", bEdits[0].NewText)
}

func TestAnchorFragmentBytes_ImageInLink(t *testing.T) {
	// destBounds first finds ](icon.png) with no hash; the scanner must
	// advance past it and find ](a.md#setup).
	row := []byte("[![icon](icon.png)](a.md#setup)")
	s, e, ok := anchorFragmentBytes(row, 3, "setup")
	require.True(t, ok)
	assert.Equal(t, "setup", string(row[s:e]))
}

func TestAnchorFragmentBytes_ImageWithFragInLink(t *testing.T) {
	// Image destination itself has a fragment that doesn't match;
	// the scanner must advance past it and find the correct one.
	row := []byte("[![icon](icon.png#badge)](a.md#setup)")
	s, e, ok := anchorFragmentBytes(row, 3, "setup")
	require.True(t, ok)
	assert.Equal(t, "setup", string(row[s:e]))
}

// TestHeading_DropsAnchorEditsInsideHeadingText pins that a link on
// the renamed heading's own line that points back at the heading gets
// no edit of its own. The heading-text edit already replaces those
// bytes, so a fragment edit inside them would claim the same bytes
// twice: ApplyEdits rejects such a plan, and LSP forbids it. Links
// outside the heading text still get their fragment edit.
func TestHeading_DropsAnchorEditsInsideHeadingText(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		line    int
		oldName string
		want    map[string][]Edit
	}{
		{
			name:    "the whole heading text is a self-link",
			files:   map[string]string{"a.md": "# [Setup](#setup)\n"},
			line:    1,
			oldName: "Setup",
			want:    map[string][]Edit{"a.md": {mkEdit(0, 2, 17, "Install")}},
		},
		{
			name:    "a self-link heading and a same-file link below it",
			files:   map[string]string{"a.md": "# Top\n\n## [Setup](#setup)\n\nSee [s](#setup).\n"},
			line:    3,
			oldName: "Setup",
			want: map[string][]Edit{"a.md": {
				mkEdit(4, 9, 14, "install"),
				mkEdit(2, 3, 18, "Install"),
			}},
		},
		{
			name: "a self-link inside the heading text and an incoming link",
			files: map[string]string{
				"a.md": "## Setup [top](#setup-top)\n",
				"b.md": "[x](a.md#setup-top)\n",
			},
			line:    1,
			oldName: "Setup top",
			want: map[string][]Edit{
				"a.md": {mkEdit(0, 3, 26, "Install")},
				"b.md": {mkEdit(0, 9, 18, "install")},
			},
		},
		{
			name:    "a setext heading whose text is a self-link",
			files:   map[string]string{"a.md": "[Setup](#setup)\n=====\n"},
			line:    1,
			oldName: "Setup",
			want:    map[string][]Edit{"a.md": {mkEdit(0, 0, 15, "Install")}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := newMemWorkspace(tt.files)
			changes, err := callHeading(ws, "a.md", "a.md", ws.files["a.md"], tt.line, tt.oldName, "Install")
			require.NoError(t, err)
			assert.Equal(t, tt.want, changes)
		})
	}
}

// TestHeading_SelfLinkRenameAppliesCleanly runs a heading rename whose
// heading links to itself end to end: plan, then splice every file
// through ApplyEdits, pinning the bytes written. Before the planner
// dropped the inner fragment edit, the plan's two overlapping edits
// wrote `# Installl)`; with the overlap check they failed the rename.
// The new text replaces the whole heading text, link markup included,
// as it does for any inline markup (`# **Setup**` becomes `# Install`).
func TestHeading_SelfLinkRenameAppliesCleanly(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		line    int
		oldName string
		want    map[string]string
	}{
		{
			name:    "the whole heading text is a self-link",
			files:   map[string]string{"a.md": "# [Setup](#setup)\n"},
			line:    1,
			oldName: "Setup",
			want:    map[string]string{"a.md": "# Install\n"},
		},
		{
			name:    "a self-link heading and a same-file link below it",
			files:   map[string]string{"a.md": "# Top\n\n## [Setup](#setup)\n\nSee [s](#setup).\n"},
			line:    3,
			oldName: "Setup",
			want:    map[string]string{"a.md": "# Top\n\n## Install\n\nSee [s](#install).\n"},
		},
		{
			name: "a self-link inside the heading text and an incoming link",
			files: map[string]string{
				"a.md": "## Setup [top](#setup-top)\n",
				"b.md": "[x](a.md#setup-top)\n",
			},
			line:    1,
			oldName: "Setup top",
			want: map[string]string{
				"a.md": "## Install\n",
				"b.md": "[x](a.md#install)\n",
			},
		},
		{
			name:    "a self-link heading below front matter",
			files:   map[string]string{"a.md": "---\ntitle: T\n---\n# [Setup](#setup)\n\nSee [s](#setup).\n"},
			line:    4,
			oldName: "Setup",
			want:    map[string]string{"a.md": "---\ntitle: T\n---\n# Install\n\nSee [s](#install).\n"},
		},
		{
			name:    "a CRLF setext heading whose text is a self-link",
			files:   map[string]string{"a.md": "[Setup](#setup)\r\n=====\r\n\r\nSee [s](#setup).\r\n"},
			line:    1,
			oldName: "Setup",
			want:    map[string]string{"a.md": "Install\r\n=====\r\n\r\nSee [s](#install).\r\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, renameHeadingAndApply(t, tt.files, tt.line, tt.oldName, "Install"))
		})
	}
}

// renameHeadingAndApply renames the heading on line of a.md in a
// workspace holding files, splices each file's planned edits through
// ApplyEdits, and returns the rewritten bytes of every file the plan
// edits.
func renameHeadingAndApply(t *testing.T, files map[string]string, line int, oldName, newName string) map[string]string {
	t.Helper()
	ws := newMemWorkspace(files)
	plan, err := Heading(ws, "a.md", "a.md", ws.files["a.md"], line, oldName, newName)
	require.NoError(t, err)
	got := make(map[string]string, len(plan.Edits))
	for key, edits := range plan.Edits {
		got[key] = applyEditsToSource(t, files[key], edits)
	}
	return got
}

// TestDropEditsInside pins which edits the heading-text edit swallows:
// those on its line that claim a byte of its range, or insert strictly
// inside it. An edit that only touches the range — an insert at either
// end — and an edit on another line are kept, in input order. A
// partial overlap is kept too, so ApplyEdits still reports it.
func TestDropEditsInside(t *testing.T) {
	outer := mkEdit(2, 3, 18, "Install").Range
	tests := []struct {
		name string
		in   []Edit
		want []Edit
	}{
		{"no edits", nil, nil},
		{"an edit inside is dropped", []Edit{mkEdit(2, 12, 17, "x")}, []Edit{}},
		{"an edit over the same range is dropped", []Edit{mkEdit(2, 3, 18, "x")}, []Edit{}},
		{"an insert strictly inside is dropped", []Edit{mkEdit(2, 5, 5, "x")}, []Edit{}},
		{"an insert at the start is kept", []Edit{mkEdit(2, 3, 3, "x")}, []Edit{mkEdit(2, 3, 3, "x")}},
		{"an insert at the end is kept", []Edit{mkEdit(2, 18, 18, "x")}, []Edit{mkEdit(2, 18, 18, "x")}},
		{"a partial overlap is kept", []Edit{mkEdit(2, 15, 20, "x")}, []Edit{mkEdit(2, 15, 20, "x")}},
		{"the same columns on another line are kept", []Edit{mkEdit(4, 12, 17, "x")}, []Edit{mkEdit(4, 12, 17, "x")}},
		{
			"kept edits stay in input order",
			[]Edit{mkEdit(4, 9, 14, "a"), mkEdit(2, 12, 17, "b"), mkEdit(0, 1, 2, "c")},
			[]Edit{mkEdit(4, 9, 14, "a"), mkEdit(0, 1, 2, "c")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dropEditsInside(tt.in, outer))
		})
	}
	t.Run("an empty heading range keeps an insert at its offset", func(t *testing.T) {
		empty := mkEdit(0, 4, 4, "Install").Range
		in := []Edit{mkEdit(0, 4, 4, "x")}
		assert.Equal(t, []Edit{mkEdit(0, 4, 4, "x")}, dropEditsInside(in, empty))
	})
}
