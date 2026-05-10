package lsp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// TestInitializeAdvertisesRenameProvider checks that the
// `renameProvider` capability flips on with `prepareProvider: true`,
// matching the contract documented in
// docs/reference/cli/lsp.md#rename.
func TestInitializeAdvertisesRenameProvider(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	resultRaw, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	require.NotNil(t, res.Capabilities.RenameProvider)
	assert.True(t, res.Capabilities.RenameProvider.PrepareProvider)
}

// TestPrepareRenameHeadingReturnsTextRange verifies that the
// returned range covers just the heading text, not the leading
// `#` markers.
func TestPrepareRenameHeadingReturnsTextRange(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Setup\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	var res prepareRenameResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.Equal(t, "Setup", res.Placeholder)
	// "## Setup" — `S` is at byte column 3 (UTF-16 col 3), end col 8.
	assert.Equal(t, Position{Line: 2, Character: 3}, res.Range.Start)
	assert.Equal(t, Position{Line: 2, Character: 8}, res.Range.End)
}

// TestPrepareRenameOnProseReturnsNull verifies that a cursor on a
// plain paragraph yields a null result so the editor doesn't open
// the rename popup.
func TestPrepareRenameOnProseReturnsNull(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nparagraph text\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

// TestRenameHeadingRewritesCrossFileAnchors verifies the headline
// acceptance criterion: a heading rename in a.md updates the
// anchor links in b.md and c.md as part of one WorkspaceEdit.
func TestRenameHeadingRewritesCrossFileAnchors(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n\n## Setup\n\nbody\n"
	srcB := "# Beta\n\n[s](./a.md#setup)\n"
	srcC := "# Gamma\n\n[also](./a.md#setup)\n[same](./a.md#setup)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "c.md": srcC,
	})
	uriA := rootURI + "/a.md"
	uriB := rootURI + "/b.md"
	uriC := rootURI + "/c.md"
	openAll := []struct{ uri, src string }{{uriA, srcA}, {uriB, srcB}, {uriC, srcC}}
	for _, d := range openAll {
		h.notify("textDocument/didOpen", didOpenTextDocumentParams{
			TextDocument: textDocumentItem{URI: d.uri, LanguageID: "markdown", Version: 1, Text: d.src},
		})
		_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	}

	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 2, Character: 4},
		NewName:      "Configuration",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uriA)
	require.Contains(t, edit.Changes, uriB)
	require.Contains(t, edit.Changes, uriC)
	assert.Len(t, edit.Changes[uriA], 1, "heading line edit")
	assert.Len(t, edit.Changes[uriB], 1, "one anchor link in b.md")
	assert.Len(t, edit.Changes[uriC], 2, "two anchor links in c.md")
	// The replacement on a.md is the new heading text.
	assert.Equal(t, "Configuration", edit.Changes[uriA][0].NewText)
	// Anchor edits replace just the slug portion (no leading `#`).
	for _, e := range edit.Changes[uriB] {
		assert.Equal(t, "configuration", e.NewText)
	}
}

// TestRenameHeadingShiftDetection verifies that when a
// duplicate-name pair causes the disambiguator to shift, anchors
// pointing at the now-shifted slug also update.
//
// File `a.md`:
//
//	## Setup       <- slug "setup"
//	## Setup       <- slug "setup-1"
//
// File `b.md` links to `#setup-1`. Renaming the *first* heading to
// "Configuration" means the second heading's slug becomes "setup",
// and `b.md`'s `#setup-1` link must follow it to `#setup`.
func TestRenameHeadingShiftDetection(t *testing.T) {
	t.Parallel()
	srcA := "# Top\n\n## Setup\n\nfirst\n\n## Setup\n\nsecond\n"
	srcB := "# B\n\n[link](./a.md#setup-1)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB,
	})
	uriA := rootURI + "/a.md"
	uriB := rootURI + "/b.md"
	for _, d := range []struct{ uri, src string }{{uriA, srcA}, {uriB, srcB}} {
		h.notify("textDocument/didOpen", didOpenTextDocumentParams{
			TextDocument: textDocumentItem{URI: d.uri, LanguageID: "markdown", Version: 1, Text: d.src},
		})
		_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	}
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		// Position on the FIRST `## Setup` (line 3, 1-based) → LSP line 2.
		Position: Position{Line: 2, Character: 4},
		NewName:  "Configuration",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uriB)
	require.Len(t, edit.Changes[uriB], 1)
	// `setup-1` shifted to `setup`; b.md's link must now point at
	// `#setup`.
	assert.Equal(t, "setup", edit.Changes[uriB][0].NewText)
}

// TestRenameHeadingCollisionReturnsInvalidParams verifies that a
// rename whose new bare slug duplicates another heading returns an
// LSP error with the colliding heading name in `data`.
func TestRenameHeadingCollisionReturnsInvalidParams(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Foo\n\n## Bar\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor on `## Bar` (line 5, 1-based) → LSP line 4.
		Position: Position{Line: 4, Character: 4},
		NewName:  "Foo",
	})
	require.NotNil(t, errResp, "expected InvalidParams")
	assert.Equal(t, codeInvalidParams, errResp.Code)
	require.NotNil(t, errResp.Data)
	dataMap, ok := errResp.Data.(map[string]any)
	require.True(t, ok, "expected map data, got %T", errResp.Data)
	assert.Equal(t, "Foo", dataMap["conflict"])
}

// TestRenameLinkRefLabel verifies that renaming a link-reference
// label updates the def and every full / shortcut use in the same
// file.
func TestRenameLinkRefLabel(t *testing.T) {
	t.Parallel()
	src := "# T\n\nUse [text][docs] and [docs] again.\n\n[docs]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor on `[docs]: …` (line 5, 1-based) → LSP line 4.
		Position: Position{Line: 4, Character: 2},
		NewName:  "manual",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	// One def + two uses (full + shortcut) = 3 edits.
	assert.Len(t, edit.Changes[uri], 3)
	for _, e := range edit.Changes[uri] {
		assert.Equal(t, "manual", e.NewText)
	}
}

// TestRenameLinkRefLabelCollision verifies that renaming a label to
// match another existing definition fails loud rather than silently
// breaking references.
func TestRenameLinkRefLabelCollision(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [docs][docs].\n\n[docs]: https://x\n[manual]: https://y\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor on `[docs]: …` (line 5, 1-based).
		Position: Position{Line: 4, Character: 2},
		NewName:  "manual",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestRenameOnPlainProseReturnsError checks that a rename request
// at a position with no renameable symbol surfaces InvalidParams
// rather than silently returning an empty WorkspaceEdit (which an
// LSP client would apply as "no change" without any user feedback).
func TestRenameOnPlainProseReturnsError(t *testing.T) {
	t.Parallel()
	src := "# T\n\nplain text\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 3},
		NewName:      "anything",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestPrepareRenameLabelPlaceholderPreservesCase verifies that
// prepareRename returns the document's raw label text in
// `placeholder`, not the lowercased / whitespace-collapsed form
// the index uses for cross-link matching. Without this the editor
// would pre-fill the rename popup with `docs api` for a label
// written `Docs API`, surprising the user mid-rename.
func TestPrepareRenameLabelPlaceholderPreservesCase(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [text][Docs API].\n\n[Docs API]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor on `[Docs API]:` (line 5, 1-based) → LSP line 4.
		Position: Position{Line: 4, Character: 2},
	})
	require.Nil(t, errResp)
	var res prepareRenameResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.Equal(t, "Docs API", res.Placeholder)
}

// TestRenameHeadingHandlesEmptySlugSibling verifies that
// computeSlugRemap stays in sync when an earlier heading has an
// empty slug. mdtext.CollectTOCItems would skip that heading; the
// rename walk must include it so the index lookup matches the
// renamed heading on its actual line.
func TestRenameHeadingHandlesEmptySlugSibling(t *testing.T) {
	t.Parallel()
	// First heading slugifies to "" (only punctuation), so a naive
	// implementation would mis-identify which heading the cursor
	// lands on.
	src := "# !!!\n\n## Setup\n\nbody\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor on `## Setup` (line 3, 1-based) → LSP line 2.
		Position: Position{Line: 2, Character: 4},
		NewName:  "Configuration",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 1)
	// The edit must target the `## Setup` line (LSP line 2), not
	// the `# !!!` line (LSP line 0).
	assert.Equal(t, 2, edit.Changes[uri][0].Range.Start.Line)
	assert.Equal(t, "Configuration", edit.Changes[uri][0].NewText)
}

// TestAtxHeadingTextByteRange covers the heading-line parsing used
// for prepareRename. These cases drive the rename popup's range so
// they need to stay tight against the documented behavior.
func TestAtxHeadingTextByteRange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		row                string
		wantOK             bool
		wantStart, wantEnd int
	}{
		{"# Hello", true, 2, 7},
		{"## Hi there", true, 3, 11},
		{"### Setup ###", true, 4, 9},
		{"###### Six", true, 7, 10},
		{"   ## Indented", true, 6, 14},
		{"#NoSpace", false, 0, 0},
		{"####### TooMany", false, 0, 0},
		{"plain text", false, 0, 0},
		{"## ", true, 3, 3},
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

// TestAnchorFragmentBytes verifies the helper that finds the slug
// portion inside a link destination on a line. The returned range
// is what the rename's TextEdit uses to swap in the new slug.
func TestAnchorFragmentBytes(t *testing.T) {
	t.Parallel()
	row := []byte("text [a](./b.md#sec) more [c](#sec) end")
	// First link: `[a](./b.md#sec)` — text starts at byte 5 (`[`).
	startA, endA, ok := anchorFragmentBytes(row, 6, "sec")
	require.True(t, ok)
	assert.Equal(t, "sec", string(row[startA:endA]))
	// Second link: `[c](#sec)` — text starts at byte 26 (`[`).
	startB, endB, ok := anchorFragmentBytes(row, 27, "sec")
	require.True(t, ok)
	assert.Equal(t, "sec", string(row[startB:endB]))
	// Slug not present on this link.
	_, _, ok = anchorFragmentBytes(row, 6, "missing")
	assert.False(t, ok)
}

// TestAnchorFragmentBytesRejectsPrefixMatch guards against
// `#foo` rewriting `#foobar`. The destination ends at `)`, so
// the fragment boundary must agree.
func TestAnchorFragmentBytesRejectsPrefixMatch(t *testing.T) {
	t.Parallel()
	row := []byte("see [t](#foobar)")
	_, _, ok := anchorFragmentBytes(row, 4, "foo")
	assert.False(t, ok)
}
