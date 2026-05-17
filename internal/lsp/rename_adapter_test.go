package lsp

import (
	"encoding/json"
	"testing"

	"github.com/jeduden/mdsmith/internal/rename"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToTextEdits(t *testing.T) {
	t.Parallel()
	got := toTextEdits([]rename.Edit{
		{Range: rename.Range{
			Start: rename.Position{Line: 1, Character: 2},
			End:   rename.Position{Line: 1, Character: 7},
		}, NewText: "x"},
	})
	require.Len(t, got, 1)
	assert.Equal(t, textEdit{
		Range:   Range{Start: Position{Line: 1, Character: 2}, End: Position{Line: 1, Character: 7}},
		NewText: "x",
	}, got[0])
}

func TestToLSPChanges(t *testing.T) {
	t.Parallel()
	// An empty engine map still yields a non-nil map so the reply
	// serializes as `"changes": {}` rather than `null`.
	got := toLSPChanges(map[string][]rename.Edit{})
	assert.NotNil(t, got)
	assert.Empty(t, got)

	got = toLSPChanges(map[string][]rename.Edit{
		"file:///a.md": {{NewText: "n"}},
	})
	require.Len(t, got["file:///a.md"], 1)
	assert.Equal(t, "n", got["file:///a.md"][0].NewText)
}

func TestSortTextEditsBottomUp(t *testing.T) {
	t.Parallel()
	edits := []textEdit{
		{Range: Range{Start: Position{Line: 1, Character: 0}}},
		{Range: Range{Start: Position{Line: 5, Character: 1}}},
		{Range: Range{Start: Position{Line: 5, Character: 4}}},
	}
	sortTextEditsBottomUp(edits)
	assert.Equal(t, 5, edits[0].Range.Start.Line)
	assert.Equal(t, 4, edits[0].Range.Start.Character)
	assert.Equal(t, 5, edits[1].Range.Start.Line)
	assert.Equal(t, 1, edits[2].Range.Start.Line)
}

func TestServer_writeRenameError(t *testing.T) {
	t.Parallel()

	t.Run("heading collision carries conflict data", func(t *testing.T) {
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		s.writeRenameError(json.RawMessage(`1`), rename.HeadingCollisionError{Conflict: "Intro"})
		out := buf.String()
		assert.Contains(t, out, `"code":-32602`)
		assert.Contains(t, out, `rename would collide with heading Intro`)
		assert.Contains(t, out, `"conflict":"Intro"`)
	})

	t.Run("label collision carries conflict data", func(t *testing.T) {
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		s.writeRenameError(json.RawMessage(`1`), rename.LabelConflictError{Conflict: "Docs"})
		out := buf.String()
		assert.Contains(t, out, `"code":-32602`)
		assert.Contains(t, out, `rename would collide with link reference [Docs]`)
		assert.Contains(t, out, `"conflict":"Docs"`)
	})

	t.Run("plain typed error surfaces its message", func(t *testing.T) {
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		s.writeRenameError(json.RawMessage(`1`), rename.ErrEmptyHeadingSlug)
		out := buf.String()
		assert.Contains(t, out, `"code":-32602`)
		assert.Contains(t, out, rename.ErrEmptyHeadingSlug.Error())
	})
}
