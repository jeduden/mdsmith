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
	assert.Equal(t, "Configuration", edit.Changes[uriA][0].NewText)
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
		Position:     Position{Line: 2, Character: 4},
		NewName:      "Configuration",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uriB)
	require.Len(t, edit.Changes[uriB], 1)
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
		Position:     Position{Line: 4, Character: 4},
		NewName:      "Foo",
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
		Position:     Position{Line: 4, Character: 2},
		NewName:      "manual",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
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
		Position:     Position{Line: 4, Character: 2},
		NewName:      "manual",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestRenameLinkRefRefreshesCasing verifies that a rename whose
// normalized label is unchanged but whose visible spelling differs
// (`docs api` → `Docs API`) still produces edits across the def
// and every use. Treating the normalized-equal case as a no-op
// would silently block users from updating just casing or
// whitespace of a label.
func TestRenameLinkRefRefreshesCasing(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [t][docs api] and [docs api] again.\n\n[docs api]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
		NewName:      "Docs API",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 3)
	for _, e := range edit.Changes[uri] {
		assert.Equal(t, "Docs API", e.NewText)
	}
}

// TestRenameLinkRefDetectsDuplicateDefCollision verifies that the
// collision check inspects the source directly, not the deduped
// symbol index. The index only stores the first def per normalized
// label, so a buffer that already carries two `[manual]: …` lines
// (one of which is unused) would otherwise pass the collision
// check silently.
func TestRenameLinkRefDetectsDuplicateDefCollision(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [t][docs].\n\n[docs]: https://x\n[manual]: https://y\n[manual]: https://z\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
		NewName:      "manual",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestRenameHeadingRejectsEmptySlugNewName verifies that a
// heading rename whose new text slugifies to "" (e.g.
// punctuation-only) is rejected. Allowing it would produce
// `#` placeholder anchors and break every incoming link
// instead of redirecting them.
func TestRenameHeadingRejectsEmptySlugNewName(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Setup\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
		NewName:      "!!!",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestRenameLinkRefRejectsBracketInNewName verifies that a
// new label containing `]` is rejected outright. Inserting it
// unescaped would close the bracket pair early, producing an
// unparsable `[label]:` line.
func TestRenameLinkRefRejectsBracketInNewName(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [t][docs].\n\n[docs]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
		NewName:      "bad]label",
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

// TestPrepareRenameOnRefUseReturnsLabelRange exercises
// prepareRename for the cursor on a `[text][label]` reference
// use, hitting the refUsePrepareRange path that the def-side
// tests don't cover.
func TestPrepareRenameOnRefUseReturnsLabelRange(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [look][docs] inline.\n\n[docs]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 12},
	})
	require.Nil(t, errResp)
	var res prepareRenameResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.Equal(t, "docs", res.Placeholder)
}

// TestRenameSetextHeading verifies the non-ATX heading rename
// path. Setext headings cover the whole text line; the rename
// edit replaces that line and leaves the underline intact.
func TestRenameSetextHeading(t *testing.T) {
	t.Parallel()
	src := "Top\n===\n\nSetup\n-----\n\nbody\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 2},
		NewName:      "Configuration",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 1)
	assert.Equal(t, "Configuration", edit.Changes[uri][0].NewText)
	assert.Equal(t, 3, edit.Changes[uri][0].Range.Start.Line)
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
		Position:     Position{Line: 4, Character: 2},
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
	src := "# !!!\n\n## Setup\n\nbody\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
		NewName:      "Configuration",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 1)
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
// TestAnchorFragmentBytesRejectsPrefixMatch guards against
// `#foo` rewriting `#foobar`. The destination ends at `)`, so
// the fragment boundary must agree.
// TestAnchorFragmentBytesNormalizesCase verifies that a raw
// fragment whose case differs from the slug still matches —
// the index keys edges by mdtext.Slugify(decoded), which is
// lowercase, so `#Setup` participates in a rename of the
// `setup` slug.
// TestAnchorFragmentBytesURLDecodesPercentEscape verifies that
// `#Docs%20API` (a real GitHub anchor when the heading is
// "Docs API") matches the indexed slug `docs-api`.
// TestAnchorFragmentBytesAngleBracketDestination verifies that
// a destination of the form `<#sec>` returns a fragment range
// that excludes the closing `>`. Without that boundary the
// rename would overwrite the `>` and corrupt the link.
// TestBodyLineIndexLookups exercises the precomputed line-offset
// table. The fast path replaced an O(n) scan that ran per
// reference-use edit; correctness is the bar this test enforces,
// not throughput.
// TestRefUseLabelBytesCollapsedTrailingEmptyBrackets verifies
// that the cursor on the trailing `[]` of a collapsed reference
// resolves to the leading bracket pair, not nil. The Locator
// already tags the position as TokenRefUse for collapsed links,
// so prepareRename must surface a range there or rename is
// effectively unreachable on `[label][]`.
func TestRefUseLabelBytesCollapsedTrailingEmptyBrackets(t *testing.T) {
	t.Parallel()
	row := []byte(`See [Docs API][] elsewhere`)
	start, end, ok := refUseLabelBytes(row, 14, "docs api")
	require.True(t, ok, "expected match for cursor inside trailing []")
	assert.Equal(t, "Docs API", string(row[start:end]))
}

// TestIsValidRefDefLine drives isValidRefDefLine with a real ref-def
// line (returns true), a heading line (returns false), and a blank
// line (returns false).
func TestIsValidRefDefLine(t *testing.T) {
	t.Parallel()
	src := []byte("# T\n\n[docs]: https://example.com\n")
	assert.True(t, isValidRefDefLine(src, 3))
	assert.False(t, isValidRefDefLine(src, 1))
	assert.False(t, isValidRefDefLine(src, 2))
}

// TestHeadingPrepareRangeATX drives headingPrepareRange with a simple
// ATX heading and verifies the range covers the text only.
func TestHeadingPrepareRangeATX(t *testing.T) {
	t.Parallel()
	src := []byte("# Hello\n")
	res, ok := headingPrepareRange(src, 1, "Hello")
	require.True(t, ok)
	assert.Equal(t, "Hello", res.Placeholder)
	assert.Equal(t, 0, res.Range.Start.Line)
	assert.Equal(t, 2, res.Range.Start.Character)
	assert.Equal(t, 7, res.Range.End.Character)
}

// TestAtxHeadingTextStart drives atxHeadingTextStart directly with
// ATX headings, a no-space header, a too-deep header, and plain text.
func TestAtxHeadingTextStart(t *testing.T) {
	t.Parallel()
	cases := []struct {
		row    string
		wantI  int
		wantOK bool
	}{
		{"# Hello", 2, true},
		{"## Hi", 3, true},
		{"###### Six", 7, true},
		{"   ## Indented", 6, true},
		{"##\tTab", 3, true},
		{"#NoSpace", 0, false},
		{"####### TooMany", 0, false},
		{"plain text", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		i, ok := atxHeadingTextStart([]byte(tc.row))
		assert.Equal(t, tc.wantOK, ok, "row=%q", tc.row)
		if ok {
			assert.Equal(t, tc.wantI, i, "row=%q", tc.row)
		}
	}
}

// TestTrimTrailingHashRun drives trimTrailingHashRun with a closing
// hash run preceded by space (stripped), a hash run without space
// (kept), and a row with no trailing hash (unchanged).
func TestTrimTrailingHashRun(t *testing.T) {
	t.Parallel()
	cases := []struct {
		row        string
		start, end int
		want       int
	}{
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

// TestSkipLeadingSpaces drives skipLeadingSpaces with no leading
// spaces, fewer than max spaces, and more than max spaces.
func TestSkipLeadingSpaces(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, skipLeadingSpaces([]byte("abc"), 3))
	assert.Equal(t, 2, skipLeadingSpaces([]byte("  abc"), 3))
	assert.Equal(t, 3, skipLeadingSpaces([]byte("   abc"), 3))
	assert.Equal(t, 3, skipLeadingSpaces([]byte("    abc"), 3))
	assert.Equal(t, 0, skipLeadingSpaces([]byte(""), 3))
}

// TestTrimRightSpace drives trimRightSpace with trailing spaces,
// trailing tab, no trailing whitespace, and all-whitespace input.
func TestTrimRightSpace(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5, trimRightSpace([]byte("hello  "), 0, 7))
	assert.Equal(t, 5, trimRightSpace([]byte("hello\t "), 0, 7))
	assert.Equal(t, 5, trimRightSpace([]byte("hello"), 0, 5))
	assert.Equal(t, 0, trimRightSpace([]byte("   "), 0, 3))
}

// TestTrimmedRange drives trimmedRange with leading-and-trailing
// whitespace, no whitespace, and an all-whitespace row.
func TestTrimmedRange(t *testing.T) {
	t.Parallel()
	start, end := trimmedRange([]byte("  hello  "))
	assert.Equal(t, 2, start)
	assert.Equal(t, 7, end)
	start, end = trimmedRange([]byte("nospace"))
	assert.Equal(t, 0, start)
	assert.Equal(t, 7, end)
	start, end = trimmedRange([]byte("   "))
	assert.Equal(t, start, end)
}

// TestRefDefPrepareRangeHappy drives refDefPrepareRange with a
// well-formed `[label]: url` line and verifies the returned
// placeholder and range.
func TestRefDefPrepareRangeHappy(t *testing.T) {
	t.Parallel()
	src := []byte("[docs]: https://example.com\n")
	res, ok := refDefPrepareRange(src, 1, "")
	require.True(t, ok)
	assert.Equal(t, "docs", res.Placeholder)
	assert.Equal(t, 0, res.Range.Start.Line)
	assert.Equal(t, 1, res.Range.Start.Character)
	assert.Equal(t, 5, res.Range.End.Character)
}

// TestRefUsePrepareRangeHappy drives refUsePrepareRange with the
// cursor inside the trailing `[label]` of a full reference link.
func TestRefUsePrepareRangeHappy(t *testing.T) {
	t.Parallel()
	src := []byte("See [text][docs] here.\n")
	res, ok := refUsePrepareRange(src, 1, 12, "docs")
	require.True(t, ok)
	assert.Equal(t, "docs", res.Placeholder)
}

// TestRefUseLabelBytesAllForms drives refUseLabelBytes across the
// four reference forms (full cursor-in-text, full cursor-in-label,
// shortcut, collapsed) and the no-match case.
func TestRefUseLabelBytesAllForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		row       string
		cursor    int
		label     string
		wantOK    bool
		wantLabel string
	}{
		{`[text][docs]`, 2, "docs", true, "docs"},  // full: cursor in text
		{`[text][docs]`, 8, "docs", true, "docs"},  // full: cursor in label
		{`[docs]`, 2, "docs", true, "docs"},        // shortcut
		{`[docs][]`, 2, "docs", true, "docs"},      // collapsed: cursor in leading
		{`[docs][]`, 7, "docs", true, "docs"},      // collapsed: cursor in trailing
		{`[other]`, 2, "docs", false, ""},          // wrong label
		{`[docs]`, 99, "docs", false, ""},          // cursor outside
	}
	for _, tc := range cases {
		row := []byte(tc.row)
		start, end, ok := refUseLabelBytes(row, tc.cursor, tc.label)
		assert.Equal(t, tc.wantOK, ok, "row=%q cursor=%d", tc.row, tc.cursor)
		if ok {
			assert.Equal(t, tc.wantLabel, string(row[start:end]), "row=%q cursor=%d", tc.row, tc.cursor)
		}
	}
}

// TestMatchLeadingPairHappy drives matchLeadingPair for the full
// `[text][label]` case (returns trailing label) and the collapsed
// `[label][]` case (returns leading pair content).
func TestMatchLeadingPairHappy(t *testing.T) {
	t.Parallel()
	// full: [text][label] — leading pair, returns trailing label
	row := []byte(`[text][label]`)
	pairs := bracketPairs(row)
	require.Len(t, pairs, 2)
	start, end, ok := matchLeadingPair(row, pairs, 0, "label")
	require.True(t, ok)
	assert.Equal(t, "label", string(row[start:end]))

	// collapsed: [label][] — leading pair, returns leading content
	row2 := []byte(`[label][]`)
	pairs2 := bracketPairs(row2)
	require.Len(t, pairs2, 2)
	start2, end2, ok2 := matchLeadingPair(row2, pairs2, 0, "label")
	require.True(t, ok2)
	assert.Equal(t, "label", string(row2[start2:end2]))
}

// TestMatchTrailingPairHappy drives matchTrailingPair for the full
// `[text][label]` case (returns trailing label) and the collapsed
// `[label][]` case (returns leading pair content).
func TestMatchTrailingPairHappy(t *testing.T) {
	t.Parallel()
	// full: [text][label] — trailing pair, returns label
	row := []byte(`[text][label]`)
	pairs := bracketPairs(row)
	require.Len(t, pairs, 2)
	start, end, ok := matchTrailingPair(row, pairs, 1, "label")
	require.True(t, ok)
	assert.Equal(t, "label", string(row[start:end]))

	// collapsed: [label][] — trailing empty pair, returns leading content
	row2 := []byte(`[label][]`)
	pairs2 := bracketPairs(row2)
	require.Len(t, pairs2, 2)
	start2, end2, ok2 := matchTrailingPair(row2, pairs2, 1, "label")
	require.True(t, ok2)
	assert.Equal(t, "label", string(row2[start2:end2]))
}

// TestBracketPairsBasic drives bracketPairs with a single pair, two
// adjacent pairs, and an empty row.
func TestBracketPairsBasic(t *testing.T) {
	t.Parallel()
	// single pair
	pairs := bracketPairs([]byte(`[a]`))
	require.Len(t, pairs, 1)
	assert.Equal(t, 0, pairs[0].open)
	assert.Equal(t, 2, pairs[0].close)

	// two adjacent pairs
	row := []byte(`[a][b]`)
	pairs = bracketPairs(row)
	require.Len(t, pairs, 2)
	assert.Equal(t, "a", string(row[pairs[0].open+1:pairs[0].close]))
	assert.Equal(t, "b", string(row[pairs[1].open+1:pairs[1].close]))

	// empty row
	pairs = bracketPairs([]byte(``))
	assert.Empty(t, pairs)
}
