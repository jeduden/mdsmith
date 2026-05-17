package rename

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkRef_DefAndShortcutUse(t *testing.T) {
	src := []byte("# T\n\nSee [spec].\n\n[spec]: https://x.example\n")
	edits, err := LinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 2)
	// Both edits replace the label text with the new name.
	for _, e := range edits {
		assert.Equal(t, "rfc", e.NewText)
		assert.Equal(t, e.Range.Start.Line, e.Range.End.Line)
	}
}

func TestLinkRef_DefAndFullUse(t *testing.T) {
	src := []byte("# T\n\nSee [the spec][spec] here.\n\n[spec]: u\n")
	edits, err := LinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 2)
}

func TestLinkRef_EmptyLabel(t *testing.T) {
	_, err := LinkRef([]byte("[a]: u\n"), "a", "   ")
	assert.ErrorIs(t, err, ErrEmptyLabel)
}

func TestLinkRef_InvalidRune(t *testing.T) {
	_, err := LinkRef([]byte("[a]: u\n"), "a", "bad]label")
	var ire InvalidLabelRuneError
	require.True(t, errors.As(err, &ire))
	assert.Equal(t, ']', ire.Rune)
}

func TestLinkRef_NewlineRuneRejected(t *testing.T) {
	_, err := LinkRef([]byte("[a]: u\n"), "a", "two\nlines")
	var ire InvalidLabelRuneError
	require.True(t, errors.As(err, &ire))
	assert.Equal(t, '\n', ire.Rune)
}

func TestLinkRef_LabelConflict(t *testing.T) {
	src := []byte("[a]: u1\n[Beta]: u2\n")
	_, err := LinkRef(src, "a", "beta")
	var lce LabelConflictError
	require.True(t, errors.As(err, &lce))
	assert.Equal(t, "Beta", lce.Conflict)
}

func TestLinkRef_SameNormalizedFormRefreshesCasing(t *testing.T) {
	// "a b" and "A  B" normalize identically; the rename is allowed
	// and must not self-collide.
	src := []byte("Use [a b].\n\n[a b]: u\n")
	edits, err := LinkRef(src, "a b", "A  B")
	require.NoError(t, err)
	require.Len(t, edits, 2)
}

func TestLinkRef_CodeFenceDefNotRewritten(t *testing.T) {
	src := []byte("Use [spec].\n\n```\n[spec]: fake\n```\n\n[spec]: real\n")
	edits, err := LinkRef(src, "spec", "rfc")
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
	edits, err := LinkRef(src, "s", "t")
	require.NoError(t, err)
	require.Len(t, edits, 2)
	// Edits land on body lines shifted by the 3-line front matter.
	for _, e := range edits {
		assert.GreaterOrEqual(t, e.Range.Start.Line, 3)
	}
}

func TestValidRefDefBodyLines(t *testing.T) {
	body := []byte("para\n\n[a]: u\n\n```\n[b]: v\n```\n")
	got := ValidRefDefBodyLines(body)
	assert.True(t, got[3], "real def on body line 3")
	assert.False(t, got[6], "fenced def-shaped line is not a def")
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
	edits, err := LinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 2, "only the spec def + spec use")
}

func TestLinkRef_NoMatchingLabel(t *testing.T) {
	// Renaming a label with no def and no use yields no edits and
	// no error (the caller decides whether that is meaningful).
	edits, err := LinkRef([]byte("# Just a heading\n"), "ghost", "spirit")
	require.NoError(t, err)
	assert.Empty(t, edits)
}
