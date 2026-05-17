package rename

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yuin/goldmark/ast"
)

func TestInvalidLabelRuneError_Error(t *testing.T) {
	assert.Equal(t, `label cannot contain ']'`, InvalidLabelRuneError{Rune: ']'}.Error())
}

func TestLabelConflictError_Error(t *testing.T) {
	assert.Equal(t,
		"rename would collide with link reference [Beta]",
		LabelConflictError{Conflict: "Beta"}.Error())
}

func TestRefDefBracketBytes(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want []int
	}{
		{"no opening bracket", "no bracket here", nil},
		{"unclosed bracket", "[unclosed: u", nil},
		{"empty label", "[]: u", nil},
		{"no colon", "[a] not a def", nil},
		{"valid", "[a]: u", []int{1, 2}},
		{"indented valid", "  [lbl]: u", []int{3, 6}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, refDefBracketBytes([]byte(tc.row)))
		})
	}
}

func TestLabelBoundsInBody(t *testing.T) {
	t.Run("negative text bounds", func(t *testing.T) {
		_, _, ok := labelBoundsInBody([]byte("x"), -1, -1, ast.ReferenceLinkShortcut)
		assert.False(t, ok)
	})
	t.Run("full ref text not closed by bracket", func(t *testing.T) {
		// textEnd points past end / not at ']'
		_, _, ok := labelBoundsInBody([]byte("ab"), 0, 2, ast.ReferenceLinkFull)
		assert.False(t, ok)
	})
	t.Run("full ref missing label opener", func(t *testing.T) {
		body := []byte("t]x")
		_, _, ok := labelBoundsInBody(body, 0, 1, ast.ReferenceLinkFull)
		assert.False(t, ok)
	})
	t.Run("full ref unterminated label", func(t *testing.T) {
		body := []byte("t][lbl")
		_, _, ok := labelBoundsInBody(body, 0, 1, ast.ReferenceLinkFull)
		assert.False(t, ok)
	})
	t.Run("full ref with escaped bracket then close", func(t *testing.T) {
		body := []byte(`t][a\]b]`)
		s, e, ok := labelBoundsInBody(body, 0, 1, ast.ReferenceLinkFull)
		assert.True(t, ok)
		assert.Equal(t, `a\]b`, string(body[s:e]))
	})
	t.Run("full ref success", func(t *testing.T) {
		body := []byte("t][lbl]")
		s, e, ok := labelBoundsInBody(body, 0, 1, ast.ReferenceLinkFull)
		assert.True(t, ok)
		assert.Equal(t, "lbl", string(body[s:e]))
	})
	t.Run("shortcut not opened by bracket", func(t *testing.T) {
		_, _, ok := labelBoundsInBody([]byte("lbl]"), 0, 3, ast.ReferenceLinkShortcut)
		assert.False(t, ok)
	})
	t.Run("shortcut not closed by bracket", func(t *testing.T) {
		_, _, ok := labelBoundsInBody([]byte("[lblx"), 1, 5, ast.ReferenceLinkShortcut)
		assert.False(t, ok)
	})
	t.Run("shortcut success", func(t *testing.T) {
		body := []byte("[lbl]")
		s, e, ok := labelBoundsInBody(body, 1, 4, ast.ReferenceLinkShortcut)
		assert.True(t, ok)
		assert.Equal(t, "lbl", string(body[s:e]))
	})
}

func TestLineOfBodyOffset(t *testing.T) {
	body := []byte("a\nbb\nc")
	assert.Equal(t, 1, lineOfBodyOffset(body, -5), "negative clamps to line 1")
	assert.Equal(t, 1, lineOfBodyOffset(body, 0))
	assert.Equal(t, 2, lineOfBodyOffset(body, 2))
	assert.Equal(t, 3, lineOfBodyOffset(body, 99), "past end clamps to last line")
}

func TestBodyLineIndex(t *testing.T) {
	idx := newBodyLineIndex([]byte("a\nbb\nccc"))
	assert.Equal(t, 1, idx.lineOfOffset(-1), "negative clamps to line 1")
	assert.Equal(t, 1, idx.lineOfOffset(0))
	assert.Equal(t, 2, idx.lineOfOffset(2))
	assert.Equal(t, 3, idx.lineOfOffset(7))
	assert.Equal(t, 0, idx.lineStart(1))
	assert.Equal(t, -1, idx.lineStart(0), "line 0 is out of range")
	assert.Equal(t, -1, idx.lineStart(99), "past end is out of range")
}

func TestSplitLines(t *testing.T) {
	assert.Equal(t, [][]byte{nil}, splitLines(nil), "empty input is one empty line")
	got := splitLines([]byte("a\r\nb\n"))
	assert.Equal(t, "a", string(got[0]), "CR stripped")
	assert.Equal(t, "b", string(got[1]))
}

func TestUTF16FromByteOffset(t *testing.T) {
	assert.Equal(t, 0, utf16FromByteOffset([]byte("abc"), 0), "zero offset")
	assert.Equal(t, 0, utf16FromByteOffset([]byte("abc"), -3), "negative clamps to 0")
	assert.Equal(t, 3, utf16FromByteOffset([]byte("abc"), 99), "past end clamps to length")
	// "𝄞" (U+1D11E) is one rune, two UTF-16 code units, four bytes.
	assert.Equal(t, 2, utf16FromByteOffset([]byte("𝄞"), 4))
}

func TestNonNegativeUTF16RuneLen(t *testing.T) {
	assert.Equal(t, 1, nonNegativeUTF16RuneLen('a'))
	assert.Equal(t, 2, nonNegativeUTF16RuneLen('𝄞'))
	// A lone surrogate is an invalid scalar: utf16.RuneLen returns
	// -1, and the guard maps it to one unit.
	assert.Equal(t, 1, nonNegativeUTF16RuneLen(rune(0xD800)))
}

func TestRefDefEditsInBody_SkipsNonMatchingAndOutOfRange(t *testing.T) {
	// One real def for "a" and one for "other"; renaming "a" only
	// touches the "a" def. Exercises the normLabel-mismatch skip.
	body := []byte("[a]: u\n[other]: v\n")
	lines := splitLines(body)
	edits := refDefEditsInBody(body, lines, 0, "a", "z")
	assert.Len(t, edits, 1)
}

func TestRefDefEditsInBody_DefLinePastLineTable(t *testing.T) {
	// The def is real (validRefDefMatches finds it on body line 1),
	// but the caller hands in an empty line table, so the
	// out-of-range guard skips it instead of indexing past the end.
	body := []byte("[a]: u\n")
	edits := refDefEditsInBody(body, [][]byte{}, 0, "a", "z")
	assert.Empty(t, edits)
}

func TestLinkRef_EmptyTextReferenceUseSkipped(t *testing.T) {
	// `[][spec]` is a full reference with empty display text:
	// linkTextBounds can't anchor it, so refUseEdit skips that use
	// while the def and the normal `[spec]` use are still rewritten.
	src := []byte("Empty [][spec] and normal [spec].\n\n[spec]: u\n")
	edits, err := LinkRef(src, "spec", "rfc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// def + the one anchorable use; the empty-text use is dropped.
	assert.Len(t, edits, 2)
}
