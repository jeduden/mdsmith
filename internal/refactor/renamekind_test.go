package refactor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRenameKind(t *testing.T) {
	for _, s := range []string{"", "heading", "label"} {
		k, err := ParseRenameKind(s)
		require.NoError(t, err, s)
		assert.Equal(t, RenameKind(s), k)
	}
	_, err := ParseRenameKind("bogus")
	var invalid InvalidRenameKindError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "bogus", invalid.Kind)
	assert.Contains(t, err.Error(), `"bogus"`)
}

// Every listed kind parses, so the list RenameKindList renders and the
// set ParseRenameKind accepts cannot drift apart.
func TestRenameKindsParse(t *testing.T) {
	assert.Equal(t, []RenameKind{KindHeading, KindLabel}, renameKinds)
	for _, k := range renameKinds {
		got, err := ParseRenameKind(string(k))
		require.NoError(t, err, k)
		assert.Equal(t, k, got)
	}
}

func TestEditsLeaveUnchanged(t *testing.T) {
	src := []byte("# Setup\r\n\r\nbody\n")
	at := func(line, from, to int, text string) Edit {
		return Edit{Range: Range{Start: Position{line, from}, End: Position{line, to}}, NewText: text}
	}
	assert.True(t, editsLeaveUnchanged(src, nil), "no edits")
	assert.True(t, editsLeaveUnchanged(src, []Edit{at(0, 2, 7, "Setup")}), "same text, CRLF kept")
	assert.False(t, editsLeaveUnchanged(src, []Edit{at(0, 2, 7, "Install")}), "real edit")
	assert.False(t, editsLeaveUnchanged(src, []Edit{at(9, 0, 0, "x")}), "rejected plan is not a no-op")
	assert.False(t, editsLeaveUnchanged(src, []Edit{at(0, 2, 5, "Set"), at(0, 4, 7, "tup")}),
		"overlapping edits are rejected by ApplyEdits, so not a no-op")
	assert.False(t, editsLeaveUnchanged(src, []Edit{at(0, 3, 2, "")}), "inverted range")
	assert.False(t, editsLeaveUnchanged(src, []Edit{at(0, 2, 99, "Setup")}), "past the row")
	assert.False(t, editsLeaveUnchanged(src, []Edit{
		{Range: Range{Start: Position{0, 2}, End: Position{2, 0}}, NewText: "x"},
	}), "multi-line edit")
	assert.True(t, editsLeaveUnchanged(src, []Edit{at(2, 0, 4, "body"), at(0, 2, 7, "Setup")}),
		"several same-text edits in any order")

	// The check compares each edit with the bytes it replaces rather
	// than splicing a copy of the whole file.
	big := []byte(strings.Repeat("filler line\n", 2000) + "# Setup\n")
	edits := []Edit{at(2000, 2, 7, "Setup")}
	allocs := testing.AllocsPerRun(10, func() { editsLeaveUnchanged(big, edits) })
	assert.LessOrEqual(t, allocs, 2.0)
}

func TestRowAt(t *testing.T) {
	src := []byte("ab\r\ncd\nef")
	assert.Equal(t, "ab", string(rowAt(src, 0)), "CR dropped")
	assert.Equal(t, "cd", string(rowAt(src, 4)))
	assert.Equal(t, "ef", string(rowAt(src, 7)), "last line has no newline")
	assert.Empty(t, rowAt([]byte("x\n"), 2), "empty final row")
}

func TestEditKeepsRow(t *testing.T) {
	row := []byte("a😀b") // the emoji is two UTF-16 units
	at := func(from, to int, text string) Edit {
		return Edit{Range: Range{Start: Position{0, from}, End: Position{0, to}}, NewText: text}
	}
	assert.True(t, editKeepsRow(row, at(3, 4, "b")), "offsets count UTF-16 units")
	assert.True(t, editKeepsRow(row, at(1, 3, "😀")))
	assert.False(t, editKeepsRow(row, at(3, 4, "c")), "different text")
	assert.False(t, editKeepsRow(row, at(2, 3, "")), "splits a surrogate pair")
	assert.False(t, editKeepsRow(row, at(0, 9, "")), "past the row")
}

func TestOnlyUnchangedSelf(t *testing.T) {
	src := []byte("# Setup\n")
	same := Edit{Range: Range{End: Position{0, 7}}, NewText: "# Setup"}
	other := Edit{Range: Range{}, NewText: "x"}
	assert.True(t, onlyUnchangedSelf(Plan{Edits: map[string][]Edit{"a.md": {same}}}, "a.md", src))
	assert.False(t, onlyUnchangedSelf(Plan{Edits: map[string][]Edit{"b.md": {other}}}, "a.md", src),
		"an edit to another file only is a real rename")
	assert.False(t, onlyUnchangedSelf(Plan{Edits: map[string][]Edit{"a.md": {same}, "b.md": {other}}}, "a.md", src),
		"an incoming-link edit makes it real")
}

func TestRenameKindList(t *testing.T) {
	assert.Equal(t, "heading or label", RenameKindList("%s"))
	assert.Equal(t, `"heading" or "label"`, RenameKindList("%q"))
	assert.Equal(t, "--as heading or --as label", RenameKindList("--as %s"))
}

func TestRenameSymbolList(t *testing.T) {
	assert.Equal(t, "a heading and a link-ref label", RenameSymbolList("and", true))
	assert.Equal(t, "heading or link-ref label", RenameSymbolList("or", false))
}

func TestRenameKindNouns(t *testing.T) {
	assert.Equal(t, "heading", KindHeading.symbolNoun())
	assert.Equal(t, "link-ref label", KindLabel.symbolNoun())
	assert.Equal(t, "heading", KindHeading.missingNoun())
	assert.Equal(t, "link reference", KindLabel.missingNoun())
	assert.Equal(t, "widget", RenameKind("widget").symbolNoun(), "an unlisted kind names itself")
	assert.Equal(t, "widget", RenameKind("widget").missingNoun())
	for _, k := range renameKinds {
		assert.Contains(t, kindNouns, k, "every listed kind declares its nouns")
	}
}

func TestSentinelTextDerivesFromKinds(t *testing.T) {
	assert.Equal(t, "name matches both a heading and a link-ref label", ErrAmbiguousRename.Error())
	assert.Equal(t, "no heading or link-ref label matches the name", ErrNoRenameTarget.Error())
}

func TestJoinList(t *testing.T) {
	assert.Equal(t, "", joinList(nil, "and"))
	assert.Equal(t, "a", joinList([]string{"a"}, "and"))
	assert.Equal(t, "a and b", joinList([]string{"a", "b"}, "and"))
	assert.Equal(t, "a, b, and c", joinList([]string{"a", "b", "c"}, "and"))
}

func TestJoinOr(t *testing.T) {
	assert.Equal(t, "", joinOr(nil))
	assert.Equal(t, "a", joinOr([]string{"a"}))
	assert.Equal(t, "a or b", joinOr([]string{"a", "b"}))
	assert.Equal(t, "a, b, or c", joinOr([]string{"a", "b", "c"}))
}

func TestDetectRenameKind(t *testing.T) {
	tests := []struct {
		name string
		src  string
		old  string
		kind RenameKind
		line int
		err  error
	}{
		{"heading only", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "Setup", KindHeading, 1, nil},
		{"heading line is reported", "Intro.\n\n## Setup\n", "Setup", KindHeading, 3, nil},
		{"label only", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "docs", KindLabel, 0, nil},
		{"both match", "# docs\n\nSee [docs].\n\n[docs]: u\n", "docs", "", 0, ErrAmbiguousRename},
		{"neither match", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "ghost", "", 0, ErrNoRenameTarget},
		{"label match is case-insensitive", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "DOCS", KindLabel, 0, nil},
		{"fenced def is not a label", "# Setup\n\n```\n[docs]: u\n```\n", "docs", "", 0, ErrNoRenameTarget},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, line, err := detectRenameKind([]byte(tt.src), tt.old)
			assert.Equal(t, tt.kind, kind)
			assert.Equal(t, tt.line, line)
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

const dispatchSrc = "# Setup\n\nSee [docs].\n\n[docs]: u\n"

func newDispatchWorkspace() *memWorkspace {
	return newMemWorkspace(map[string]string{
		"a.md": dispatchSrc,
		"b.md": "See [go](a.md#setup).\n",
	})
}

func TestRename_DispatchSuccess(t *testing.T) {
	ws := newDispatchWorkspace()
	src := []byte(dispatchSrc)

	t.Run("auto heading rewrites anchors", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, "", "Setup", "Install")
		require.NoError(t, err)
		assert.Contains(t, p.Edits, "a.md")
		assert.Contains(t, p.Edits, "b.md")
	})
	t.Run("auto label", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, "", "docs", "rfc")
		require.NoError(t, err)
		assert.Len(t, p.Edits["a.md"], 2)
	})
	t.Run("explicit heading", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, KindHeading, "Setup", "Install")
		require.NoError(t, err)
		assert.Contains(t, p.Edits, "b.md")
	})
	t.Run("explicit label", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, KindLabel, "docs", "rfc")
		require.NoError(t, err)
		assert.Len(t, p.Edits["a.md"], 2)
	})
}

func TestRename_DispatchErrors(t *testing.T) {
	ws := newDispatchWorkspace()
	src := []byte(dispatchSrc)
	t.Run("ambiguous", func(t *testing.T) {
		both := []byte("# docs\n\nSee [docs].\n\n[docs]: u\n")
		_, err := Rename(ws, "a.md", both, "", "docs", "x")
		assert.ErrorIs(t, err, ErrAmbiguousRename)
	})
	t.Run("neither", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, "", "ghost", "x")
		assert.ErrorIs(t, err, ErrNoRenameTarget)
	})
	t.Run("explicit heading missing", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, KindHeading, "Ghost", "X")
		var missing MissingSymbolError
		require.ErrorAs(t, err, &missing)
		assert.Equal(t, KindHeading, missing.Kind)
		assert.Equal(t, `no heading "Ghost"`, err.Error())
	})
	t.Run("explicit label missing", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, KindLabel, "ghost", "x")
		var missing MissingSymbolError
		require.ErrorAs(t, err, &missing)
		assert.Equal(t, KindLabel, missing.Kind)
		assert.Equal(t, `no link reference "ghost"`, err.Error())
	})
	t.Run("same-name heading is nothing to rename", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, "", "Setup", "Setup")
		assert.ErrorIs(t, err, ErrNothingToRename)
		assert.Equal(t, `nothing to rename for heading "Setup"`, err.Error())
	})
	t.Run("same-name label is nothing to rename", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, "", "docs", "docs")
		assert.ErrorIs(t, err, ErrNothingToRename)
		assert.Equal(t, `nothing to rename for label "docs"`, err.Error())
		_, err = Rename(ws, "a.md", src, KindLabel, "docs", "docs")
		assert.ErrorIs(t, err, ErrNothingToRename)
	})
	t.Run("engine error passes through", func(t *testing.T) {
		two := []byte("# Setup\n\n# Other\n")
		_, err := Rename(ws, "a.md", two, KindHeading, "Setup", "Other")
		var collision HeadingCollisionError
		assert.ErrorAs(t, err, &collision)
		_, err = Rename(ws, "a.md", src, KindLabel, "docs", "bad]name")
		var invalidRune InvalidLabelRuneError
		assert.ErrorAs(t, err, &invalidRune)
	})
	t.Run("invalid kind", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, "bogus", "Setup", "X")
		var invalid InvalidRenameKindError
		assert.ErrorAs(t, err, &invalid)
	})
}

// TestRename_ReportsLikeTheExitTable pins two outcomes that must match
// the heading/label symmetry the hosts' exit tables promise.
func TestRename_ReportsLikeTheExitTable(t *testing.T) {
	ws := newDispatchWorkspace()
	t.Run("explicit label missing is reported before new-name checks", func(t *testing.T) {
		// Like the heading branch, a missing label is reported as
		// missing — not as a collision with, or an invalid spelling
		// of, a name it was never going to be renamed to.
		for _, neu := range []string{"docs", " ", "bad]name"} {
			_, err := Rename(ws, "a.md", []byte(dispatchSrc), KindLabel, "ghost", neu)
			assert.Equal(t, MissingSymbolError{Kind: KindLabel, Name: "ghost"}, err, neu)
		}
	})
	t.Run("same-bytes heading is nothing to rename", func(t *testing.T) {
		// `# *Setup*` has visible text Setup; renaming it to its own
		// source spelling rewrites the heading line with the bytes it
		// already has and shifts no slug.
		emph := []byte("# *Setup*\n")
		_, err := Rename(ws, "a.md", emph, "", "Setup", "*Setup*")
		assert.Equal(t, NothingToRenameError{Kind: KindHeading, Name: "Setup"}, err)
	})
}

func TestInvalidRenameKindError_Error(t *testing.T) {
	err := InvalidRenameKindError{Kind: "file"}
	assert.Equal(t, `rename kind must be "heading" or "label", got "file"`, err.Error())
}

func TestNothingToRenameError(t *testing.T) {
	h := NothingToRenameError{Kind: KindHeading, Name: "Setup"}
	assert.Equal(t, `nothing to rename for heading "Setup"`, h.Error())
	assert.ErrorIs(t, h, ErrNothingToRename)
	l := NothingToRenameError{Kind: KindLabel, Name: "docs"}
	assert.Equal(t, `nothing to rename for label "docs"`, l.Error())
	assert.ErrorIs(t, l, ErrNothingToRename)
	assert.False(t, l.Is(ErrNoRenameTarget))
}

func TestMissingSymbolError_Error(t *testing.T) {
	assert.Equal(t, `no heading "Setup"`,
		MissingSymbolError{Kind: KindHeading, Name: "Setup"}.Error())
	assert.Equal(t, `no link reference "docs"`,
		MissingSymbolError{Kind: KindLabel, Name: "docs"}.Error())
	assert.Equal(t, `no widget "w"`,
		MissingSymbolError{Kind: "widget", Name: "w"}.Error(),
		"an unlisted kind is not mislabeled a link reference")
}

func TestRenameHeadingAt(t *testing.T) {
	ws := newDispatchWorkspace()
	src := []byte(dispatchSrc)

	p, err := renameHeadingAt(ws, "a.md", src, 1, "Setup", "Install")
	require.NoError(t, err)
	assert.Contains(t, p.Edits, "a.md")
	assert.Contains(t, p.Edits, "b.md", "the incoming anchor is rewritten")

	_, err = renameHeadingAt(ws, "a.md", src, 1, "Setup", " Setup ")
	assert.Equal(t, NothingToRenameError{Kind: KindHeading, Name: "Setup"}, err,
		"a same-text rename has no edits")

	_, err = renameHeadingAt(ws, "a.md", src, 1, "Setup", "!!!")
	assert.ErrorIs(t, err, ErrEmptyHeadingSlug, "engine errors pass through")

	emph := []byte("# *Setup*\n")
	_, err = renameHeadingAt(ws, "a.md", emph, 1, "Setup", "*Setup*")
	assert.Equal(t, NothingToRenameError{Kind: KindHeading, Name: "Setup"}, err,
		"a heading edit that leaves the file byte-identical has nothing to do")

	p, err = renameHeadingAt(ws, "a.md", emph, 1, "Setup", "Setup")
	assert.Equal(t, NothingToRenameError{Kind: KindHeading, Name: "Setup"}, err,
		"dropping the emphasis has the same visible text, so Heading plans nothing")
	assert.Empty(t, p.Edits)

	p, err = renameHeadingAt(ws, "a.md", emph, 1, "Setup", "**Setup**")
	require.NoError(t, err, "a respelled heading is a real edit")
	assert.Len(t, p.Edits["a.md"], 1)
}

func TestRenameLabel(t *testing.T) {
	src := []byte(dispatchSrc)

	p, err := renameLabel("a.md", src, "docs", "rfc")
	require.NoError(t, err)
	assert.Len(t, p.Edits["a.md"], 2, "the def and the shortcut use")

	_, err = renameLabel("a.md", src, "docs", " ")
	assert.ErrorIs(t, err, ErrEmptyLabel, "engine errors pass through")

	_, err = renameLabel("a.md", src, "docs", "docs")
	assert.Equal(t, NothingToRenameError{Kind: KindLabel, Name: "docs"}, err,
		"a rename that leaves every occurrence byte-identical has nothing to do")

	mixed := []byte("See [docs] and [x][DOCS].\n\n[docs]: u\n")
	p, err = renameLabel("a.md", mixed, "docs", "docs")
	require.NoError(t, err, "respelling [DOCS] to [docs] is a real edit")
	assert.NotEmpty(t, p.Edits["a.md"])
}
