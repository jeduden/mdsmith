package lsp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandlePrepareRenameOnUnknownDocument verifies the
// out-of-workspace path returns null instead of crashing — the
// docTextOrFile gate refuses non-Markdown URIs.
func TestHandlePrepareRenameOnUnknownDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///nope.md"},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

// TestHandlePrepareRenameInvalidJSON exercises the malformed
// params arm so the handler's error branch participates in
// coverage. We call the handler directly with a non-JSON
// payload; the harness's request() path always marshals via
// json.Marshal and can't produce broken bytes.
func TestHandlePrepareRenameInvalidJSON(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.handlePrepareRename(&requestMessage{
		ID:     json.RawMessage(`1`),
		Params: json.RawMessage(`not json`),
	})
	assert.Contains(t, buf.String(), `"code":-32602`)
}

// TestHandleRenameInvalidJSON ensures the rename handler returns
// InvalidParams on malformed input. Same direct-call pattern as
// TestHandleCodeActionMalformedReturnsInvalidParams.
func TestHandleRenameInvalidJSON(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.handleRename(&requestMessage{
		ID:     json.RawMessage(`1`),
		Params: json.RawMessage(`not json`),
	})
	assert.Contains(t, buf.String(), `"code":-32602`)
}

// TestHandleRenameOnUnknownDocument ensures the missing-buffer
// path returns null rather than panicking.
func TestHandleRenameOnUnknownDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: "file:///nope.md"},
		Position:     Position{Line: 0, Character: 0},
		NewName:      "x",
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

// TestPrepareRenameOnAnchorLinkReturnsNull verifies the
// TokenAnchorLink arm — the cursor inside `[text](#anchor)`
// returns null because heading rename is the canonical surface.
func TestPrepareRenameOnAnchorLinkReturnsNull(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [s](#sec).\n\n## Sec\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 8},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

// TestRenameHeadingNoOpWhenSameText verifies that renaming a
// heading to its own current text returns an empty WorkspaceEdit
// rather than an error.
func TestRenameHeadingNoOpWhenSameText(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Setup\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
		NewName:      "Setup",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	assert.Empty(t, edit.Changes)
}

// TestRenameLinkRefEmptyName verifies the validation path that
// rejects a blank label.
func TestRenameLinkRefEmptyName(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[t][docs]\n\n[docs]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
		NewName:      "   ",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestRenameLinkRefNewlineRejected verifies invalid characters
// other than `[` / `]` (here, a newline) trigger the validation
// error.
func TestRenameLinkRefNewlineRejected(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[t][docs]\n\n[docs]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
		NewName:      "bad\nlabel",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestAtxHeadingTextByteRangeMore covers tab-prefixed spacing,
// trailing-hash forms with spaces, and a heading with only `#`
// and a tab so the tab branches participate in coverage.
func TestAtxHeadingTextByteRangeMore(t *testing.T) {
	t.Parallel()
	cases := []struct {
		row                string
		ok                 bool
		startWant, endWant int
	}{
		{"##\tWith tab", true, 3, 11},
		{"##  spaced", true, 4, 10},
		{"##  spaced  ", true, 4, 10},
		{"## foo ###", true, 3, 6},
		{"## foo###", true, 3, 9},
		{"##", true, 2, 2},
	}
	for _, tc := range cases {
		start, end, ok := atxHeadingTextByteRange([]byte(tc.row))
		assert.Equal(t, tc.ok, ok, "row=%q", tc.row)
		if !ok {
			continue
		}
		assert.Equal(t, tc.startWant, start, "start row=%q", tc.row)
		assert.Equal(t, tc.endWant, end, "end row=%q", tc.row)
	}
}

// TestRefDefBracketBytesEdgeCases covers the rejection paths:
// missing `[`, missing `]`, missing `:`, leading-space cap.
func TestRefDefBracketBytesEdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		row  string
		want []int
	}{
		{"[docs]: https://x", []int{1, 5}},
		{"  [docs]: https://x", []int{3, 7}},
		{"[empty]", nil},       // missing colon
		{"plain text", nil},    // no bracket
		{"[]: bad", nil},       // empty label
		{"    [over]: x", nil}, // 4 leading spaces is over the limit
	}
	for _, tc := range cases {
		got := refDefBracketBytes([]byte(tc.row))
		assert.Equal(t, tc.want, got, "row=%q", tc.row)
	}
}

// TestRefUseLabelBytesShapes covers full, shortcut, collapsed,
// and the cursor-inside-text-of-full case.
func TestRefUseLabelBytesShapes(t *testing.T) {
	t.Parallel()
	row := []byte("See [text][docs] and [docs] and [Docs API][]")
	start, end, ok := refUseLabelBytes(row, 11, "docs")
	require.True(t, ok)
	assert.Equal(t, "docs", string(row[start:end]))
	start, end, ok = refUseLabelBytes(row, 5, "docs")
	require.True(t, ok)
	assert.Equal(t, "docs", string(row[start:end]))
	start, end, ok = refUseLabelBytes(row, 22, "docs")
	require.True(t, ok)
	assert.Equal(t, "docs", string(row[start:end]))
	start, end, ok = refUseLabelBytes(row, 35, "docs api")
	require.True(t, ok)
	assert.Equal(t, "Docs API", string(row[start:end]))
}

// TestAssignSlugsDisambiguator exercises the slug-uniqueness
// pass directly: duplicate base slugs gain `-1`, `-2`, … suffixes
// in document order.
// TestAssignSlugsEmpty ensures empty-slug entries pass through
// as "" instead of stealing the first base.
// TestSlugRemapPairsSkipsUnchanged keeps the helper's contract
// in coverage — only entries whose slug changed appear.
// TestAnchorFragmentBytesNoHash covers the destination without a
// fragment — the helper returns false instead of misclassifying.
// TestAnchorFragmentBytesNoDestination covers the path where the
// `]` is not followed by `(` (e.g. a reference-style link), so
// destBounds returns false.
// TestDestBoundsHandlesEscapedParens verifies the CommonMark
// escape rule: `\(` and `\)` inside a destination don't shift
// the depth counter, so the outer `)` still closes the link.
// TestIsBackslashEscaped exercises the helper directly: even
// counts of backslashes mean the byte is unescaped.
// TestBracketPairsHandlesEscape verifies the bracket walker
// honors `\]`.
func TestBracketPairsHandlesEscape(t *testing.T) {
	t.Parallel()
	row := []byte(`[a\]b][c]`)
	pairs := bracketPairs(row)
	require.Len(t, pairs, 2)
	assert.Equal(t, "a\\]b", string(row[pairs[0].open+1:pairs[0].close]))
	assert.Equal(t, "c", string(row[pairs[1].open+1:pairs[1].close]))
}

// TestBodyAndFMOffsetWithFrontMatter exercises both branches of
// the helper.
// TestBodyLineIndexEdgeCases covers the boundaries: empty body,
// out-of-range line, last-line offsets.
// TestComputeSlugRemapTargetNotFound covers the path where the
// cursor's body line doesn't match any heading — the helper
// returns nil slices instead of indexing past the array.
// TestRenameHeadingOnUnknownLineReturnsError covers the
// renameHeading path where the cursor matches no heading
// (e.g. body trimmed away) — it should reject with InvalidParams
// rather than emit a bogus edit.
func TestRenameHeadingOnUnknownLineReturnsError(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 99, Character: 0},
		NewName:      "Other",
	})
	require.NotNil(t, errResp)
}

// TestServerRenameSameNormalizedNameStillEmits exercises the
// link-ref code path where newLabel normalizes equal to oldLabel
// but the user wanted a casing refresh — used as a regression
// against re-introducing the early-return optimization.
func TestServerRenameSameNormalizedNameStillEmits(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[t][Docs API]\n\n[Docs API]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
		NewName:      "DOCS API",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	for _, e := range edit.Changes[uri] {
		assert.Equal(t, "DOCS API", e.NewText)
	}
}

// TestPrepareRenameSetextHeading exercises headingPrepareRange's
// setext branch. The text line lacks `#` markers so
// atxHeadingTextByteRange returns false and the helper falls back
// to trimmedRange.
func TestPrepareRenameSetextHeading(t *testing.T) {
	t.Parallel()
	src := "Top\n===\n\nSetup\n-----\n\nbody\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 1},
	})
	require.Nil(t, errResp)
	var res prepareRenameResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.Equal(t, "Setup", res.Placeholder)
}

// TestHeadingTextEditOutOfRange verifies the defensive return
// when the source has fewer lines than `line` requests.
// TestHeadingPrepareRangeOutOfRange and friends cover the
// out-of-range branches of every prepareRange variant.
func TestHeadingPrepareRangeOutOfRange(t *testing.T) {
	t.Parallel()
	_, ok := headingPrepareRange([]byte("# Top\n"), 99, "x")
	assert.False(t, ok)
	_, ok = refDefPrepareRange([]byte("# Top\n"), 99, "x")
	assert.False(t, ok)
	_, ok = refUsePrepareRange([]byte("# Top\n"), 99, 1, "x")
	assert.False(t, ok)
}

// TestRefDefPrepareRangeMissingBracket covers the row-without-a-
// def-shape path.
func TestRefDefPrepareRangeMissingBracket(t *testing.T) {
	t.Parallel()
	_, ok := refDefPrepareRange([]byte("plain text\n"), 1, "x")
	assert.False(t, ok)
}

// TestRefUsePrepareRangeNoMatch covers the path where the cursor
// sits on a line with no bracket pair matching the label.
func TestRefUsePrepareRangeNoMatch(t *testing.T) {
	t.Parallel()
	_, ok := refUsePrepareRange([]byte("plain text\n"), 1, 1, "label")
	assert.False(t, ok)
}

// TestMatchLeadingAndTrailingPairMisses cover the helper's
// return-false branches: cursor in a bracket pair with no
// adjacent partner.
func TestMatchLeadingAndTrailingPairMisses(t *testing.T) {
	t.Parallel()
	pairs := []bracketPair{{open: 0, close: 5}}
	_, _, ok := matchLeadingPair([]byte(`[abc]`), pairs, 0, "abc")
	assert.False(t, ok)
	_, _, ok = matchTrailingPair([]byte(`[abc]`), pairs, 0, "abc")
	assert.False(t, ok)
}

// TestBracketPairsUnclosedBracket covers the path where `[`
// has no matching `]` on the line — the open bracket is
// silently dropped.
func TestBracketPairsUnclosedBracket(t *testing.T) {
	t.Parallel()
	pairs := bracketPairs([]byte(`[unclosed`))
	assert.Empty(t, pairs)
}

// TestAnchorFragmentBytesEdgeCases covers the defensive returns:
// negative textStart, textStart at end of row, fragment that
// starts past the destination's `)`.
// TestDestBoundsEdgeCases covers the unclosed-paren branch.
// TestFragmentMatchesSlugInvalidEscape verifies the helper falls
// back to the raw bytes when URL-unescaping fails (malformed
// percent escape).
// TestLineOfBodyOffsetEdges covers the negative-offset and
// past-EOF clamps.
// TestRenameLinkRefAfterDidChangeRewritesLiveBuffer verifies the
// rename uses the buffer the editor most recently sent rather
// than the on-disk content. After a didChange that keeps the
// `[label]: …` line intact, the rename still produces edits
// against the updated text.
func TestRenameLinkRefAfterDidChangeRewritesLiveBuffer(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[label]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	h.notify("textDocument/didChange", didChangeTextDocumentParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{{Text: "# T\n\n[label]: https://x\n"}},
	})
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 2},
		NewName:      "manual",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	assert.NotEmpty(t, edit.Changes[uri])
}

// TestRenameHeadingAllowsSameBaseSlugRefresh verifies the
// collision check skips renames whose new bare slug equals the
// target heading's existing bare slug. A casing/punctuation edit
// inside an existing duplicate-name group does not introduce a
// new collision and must not be rejected.
func TestRenameHeadingAllowsSameBaseSlugRefresh(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Setup\n\nfirst\n\n## Setup\n\nsecond\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
		NewName:      "Setup!",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	assert.Equal(t, "Setup!", edit.Changes[uri][0].NewText)
}

// TestRenameOnCodeBlockRefDefRejected mirrors
// TestPrepareRenameSkipsCodeBlockDefLookalike at the rename
// dispatch level: a client calling rename directly on a
// code-block lookalike must get InvalidParams rather than a
// silent no-op.
func TestRenameOnCodeBlockRefDefRejected(t *testing.T) {
	t.Parallel()
	src := "# T\n\n```\n[fake]: https://x\n```\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	_, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 2},
		NewName:      "real",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestRenameHeadingRejectsNewline verifies that a newline in
// newName turns into InvalidParams instead of a multi-line edit.
func TestRenameHeadingRejectsNewline(t *testing.T) {
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
		NewName:      "Bad\nName",
	})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestDestBoundsEscapedClosingInText verifies the `\]` escape
// inside link text is honored by destBounds.
// TestHandleRenameOnRefUseRewritesViaUse covers handleRename's
// TokenRefUse case — the cursor on `[text][label]` instead of
// the def line.
func TestHandleRenameOnRefUseRewritesViaUse(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [t][docs] here.\n\n[docs]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 12},
		NewName:      "manual",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	assert.Len(t, edit.Changes[uri], 2)
}

// TestResolveURIAndSourceMissingOnDisk hits the on-disk fallback
// path where the file doesn't exist.
func TestResolveURIAndSourceMissingOnDisk(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{})
	uri, src, ok := h.srv.resolveURIAndSource("missing.md")
	assert.False(t, ok)
	assert.Empty(t, uri)
	assert.Nil(t, src)
}

// TestAnchorEditForEdgeStalePaths covers anchorEditForEdge's
// defensive branches: unresolvable source file, SourceLine out
// of range, and a fragment that doesn't match.
// TestRefUseEditsInBodyIgnoresOtherLabels covers the
// `Reference.Value != oldLabel` continue branch.
func TestRefUseEditsInBodyIgnoresOtherLabels(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [a][docs] and [b][manual].\n\n[docs]: x\n[manual]: y\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
		NewName:      "renamed",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	assert.Len(t, edit.Changes[uri], 2)
}

// TestRefUseEditCollapsedReferenceLabel covers the
// ReferenceLinkCollapsed arm of refUseEdit.
func TestRefUseEditCollapsedReferenceLabel(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [docs][] inline.\n\n[docs]: x\n"
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
	assert.Len(t, edit.Changes[uri], 2)
}

// TestBracketPairsStrayClosingBracket covers the empty-stack
// branch — a `]` with no matching `[` is dropped silently.
func TestBracketPairsStrayClosingBracket(t *testing.T) {
	t.Parallel()
	pairs := bracketPairs([]byte(`stray ] then [ok]`))
	require.Len(t, pairs, 1)
}

// TestRefDefHelperShapes drives every branch of the ref-def
// destination helpers added for the heading-rename companion
// pass.
// TestRefDefDestEditForMatchSkipsBadInputs covers
// refDefDestEditForMatch's defensive returns: out-of-range
// fileLine, malformed bracket shape, empty dest, and the
// no-`#` fragment path.
// TestAppendRefDefDestEditsForHeadingSkipsUnresolvable covers
// the resolveURIAndSource fail branch by planting a synthetic
// index entry for a file the server can't read.
// TestValidRefDefMatchesSkipsRegexHitGoldmarkRefused covers the
// `!wanted[norm]` continue arm. The first `[unaccepted]: url`
// line is paragraph continuation in CommonMark, so the regex
// matches it but goldmark never registers a def for that label;
// the helper must drop it while still emitting the real
// `[accepted]: url` def.
// TestAppendAnchorEditsForHeadingDropsUnresolvableEdge plants a
// synthetic edge in the index that points at a file the server
// can't read, then verifies appendAnchorEditsForHeading skips it
// rather than producing a phantom edit. This drives the
// `if !ok { continue }` arm that the rename's natural flow
// rarely reaches.
// TestRenameLinkRefSkipsEmptyTextReference verifies that an
// empty-text reference link `[][docs]` doesn't panic the
// rename. linkTextBounds returns (-1, -1) for these — without
// the labelBoundsInBody guard, indexing body[-1] would crash.
// The use is silently skipped; the def + other uses still
// rewrite cleanly.
func TestRenameLinkRefSkipsEmptyTextReference(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [][docs] and [other][docs].\n\n[docs]: https://x\n"
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
	require.NotEmpty(t, edit.Changes[uri])
}

// TestRenameHeadingRewritesAngleBracketRefDef covers
// CommonMark's angle-bracketed reference-definition form:
// `[setup]: <./a.md#setup>`. refDefDestRange returns the
// bytes inside the angle brackets so the slug edit lands on
// just the fragment text.
func TestRenameHeadingRewritesAngleBracketRefDef(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n\n## Setup\n\nbody\n"
	srcB := "# Beta\n\n[setup]: <./a.md#setup>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
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
	assert.Equal(t, "configuration", edit.Changes[uriB][0].NewText)
}

// TestRefDefDestRangeAngleBracket exercises the new
// angle-bracket arm of refDefDestRange plus its
// fall-through on an unterminated `<…`.
// TestLabelBoundsInBodyEmptyText covers the negative-offset
// guard added for empty-text references.
// TestRenameHeadingSkipsRefDefInCodeBlock verifies that a
// `[label]: …#oldSlug` line inside a fenced code block is not
// rewritten when the heading is renamed. Without routing
// through validRefDefMatches the helper would treat any
// regex-matching line as a real def and corrupt code samples.
func TestRenameHeadingSkipsRefDefInCodeBlock(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n\n## Setup\n\nbody\n"
	srcB := "# Beta\n\nExample:\n\n```\n[setup]: ./a.md#setup\n```\n\n[real]: ./a.md#setup\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
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
	assert.NotEqual(t, 5, edit.Changes[uriB][0].Range.Start.Line)
}

// TestValidRefDefMatchesExcludesParagraphContinuation covers
// the parser-rejection path where a `[docs]: url`-shaped line
// appears as paragraph content (continuation, not a def) AND
// a real `[docs]: url` def appears later in the file. The
// label-only filter from before would accept both regex hits;
// the AST paragraph-line filter drops the continuation.
// TestRenameHeadingRewritesRefDefDestinations verifies the
// ref-def-destination companion pass. A `[setup]: ./a.md#setup`
// def in b.md isn't recorded as an edge in the index (refs are
// symbols, not edges) — without the dedicated walk, the def
// would still point at the old slug after renaming the
// heading.
func TestRenameHeadingRewritesRefDefDestinations(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n\n## Setup\n\nbody\n"
	srcB := "# Beta\n\n[setup]: ./a.md#setup\n[local]: #setup\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
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
	bEdits := edit.Changes[uriB]
	require.Len(t, bEdits, 1)
	assert.Equal(t, "configuration", bEdits[0].NewText)
}

// TestRenameLinkRefMultiLineUseRewritesLabel verifies that a
// `[wrap\ntext][docs]` reference whose text spans two lines
// still rewrites its label. labelBoundsInBody locates the
// trailing `[label]` from body offsets rather than line-local
// bracket pairs, so the multi-line text doesn't block the
// rewrite.
func TestRenameLinkRefMultiLineUseRewritesLabel(t *testing.T) {
	t.Parallel()
	src := "# T\n\nA [wrap\ntext][docs] B.\n\nC [docs] D.\n\n[docs]: https://x\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 7, Character: 2},
		NewName:      "manual",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 3)
	for _, e := range edit.Changes[uri] {
		assert.Equal(t, "manual", e.NewText)
	}
}

// TestLabelBoundsInBodyShapes covers every documented branch of
// the body-offset label resolver: full ref happy path, missing
// `]` after text, missing `[` after `]`, unterminated label,
// shortcut/collapsed with valid bracketing, and shortcut/
// collapsed where the leading `[` isn't right before textStart.
// TestBracketPairsHandlesNestedBrackets verifies that the
// bracket walker pairs the outer `]` with the outer `[` for
// CommonMark text containing balanced inner brackets (e.g.
// `[a [b]][label]`). A naive first-`]` matcher would close the
// outer pair on the inner `]`, which would cascade into wrong
// label ranges in refUseLabelBytes.
func TestBracketPairsHandlesNestedBrackets(t *testing.T) {
	t.Parallel()
	row := []byte(`[a [b]][label]`)
	pairs := bracketPairs(row)
	require.Len(t, pairs, 2)
	assert.Equal(t, "a [b]", string(row[pairs[0].open+1:pairs[0].close]))
	assert.Equal(t, "label", string(row[pairs[1].open+1:pairs[1].close]))
}

// TestRefUseLabelBytesNestedBrackets exercises refUseLabelBytes
// against a full reference link with balanced inner brackets in
// the text. The cursor on `b` (inside the inner pair) should
// resolve to the outer trailing label range.
func TestRefUseLabelBytesNestedBrackets(t *testing.T) {
	t.Parallel()
	row := []byte(`See [a [b]][label] inline`)
	start, end, ok := refUseLabelBytes(row, 8, "label")
	require.True(t, ok)
	assert.Equal(t, "label", string(row[start:end]))
}

// TestIsValidRefDefLineBodyLineUnderflow covers the
// bodyLine < 1 short-circuit. A cursor whose source line is
// inside (or before) the front matter would translate to a
// non-positive bodyLine after the offset subtraction.
func TestIsValidRefDefLineBodyLineUnderflow(t *testing.T) {
	t.Parallel()
	src := []byte("---\ntitle: T\n---\n[a]: x\n")
	assert.False(t, isValidRefDefLine(src, 1))
}

// TestResolveURIAndSourceWorkspaceURIEmpty exercises the
// workspaceURI=="" fallback. Without a configured root and with
// no open buffer matching the rel, resolveURIAndSource has no way
// to produce a URI and returns false.
func TestResolveURIAndSourceWorkspaceURIEmpty(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	_, _, ok := s.resolveURIAndSource("nope.md")
	assert.False(t, ok)
}

// TestPrepareRenameSkipsCodeBlockDefLookalike verifies that
// prepareRename returns null for `[label]: url` lines inside a
// fenced code block — those are content, not defs, and the
// rename surface must not offer to rewrite them.
func TestPrepareRenameSkipsCodeBlockDefLookalike(t *testing.T) {
	t.Parallel()
	src := "# T\n\n```\n[fake]: https://x\n```\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/prepareRename", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 2},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

// TestRenameLinkRefIgnoresCodeBlockLookalike verifies that an
// actual rename targeting a real def doesn't accidentally rewrite
// a `[label]: url`-looking line inside a fenced code block. The
// rewrite should hit the real def only.
func TestRenameLinkRefIgnoresCodeBlockLookalike(t *testing.T) {
	t.Parallel()
	src := "# T\n\nUse [t][docs].\n\n```\n[docs]: https://wrong\n```\n\n[docs]: https://right\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 8, Character: 2},
		NewName:      "manual",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 2)
	for _, e := range edit.Changes[uri] {
		assert.NotEqual(t, 5, e.Range.Start.Line, "edit must not target code-block line")
	}
}

// TestResolveURIAndSourceFallbackToDisk verifies the open-doc
// scan falls back to on-disk read when no buffer matches the
// requested rel. The closed file's URI is the canonical
// workspaceURI form.
func TestResolveURIAndSourceFallbackToDisk(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"open.md":   "# open\n",
		"closed.md": "# closed\n",
	}
	h, _, rootURI := rootedHarness(t, files)
	openURI := rootURI + "/open.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: openURI, LanguageID: "markdown", Version: 1, Text: files["open.md"]},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	uri, source, ok := h.srv.resolveURIAndSource("closed.md")
	require.True(t, ok)
	assert.Equal(t, "# closed\n", string(source))
	assert.Equal(t, rootURI+"/closed.md", uri)
}

// TestRefDefEditsInBodyIgnoresOtherLabels verifies the
// `normalizedLabel != oldLabel` continue branch — defs for other
// labels in the same buffer are skipped.
// TestAppendAnchorEditsForHeadingSkipsStaleEdge covers the
// continue branch in appendAnchorEditsForHeading: an edge whose
// link doesn't actually contain the old slug at the recorded
// column produces a nil edit and is silently skipped rather
// than failing the rename.
func TestAppendAnchorEditsForHeadingSkipsStaleEdge(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n[link](#elsewhere)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	_ = h.srv.ensureIndex()
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 3},
		NewName:      "Other",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 1)
}

// TestRenameHeadingSelfLinkSharesURI verifies that a self-anchor
// edit (a `[t](#slug)` link in the same file as the renamed
// heading) lands under the same WorkspaceEdit key as the heading-
// line edit. resolveURIAndSource preferring the open buffer's URI
// keeps the two edits from splitting across canonical and
// client-provided URI strings.
func TestRenameHeadingSelfLinkSharesURI(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Setup\n\nSee [self](#setup) for details.\n"
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
	require.Len(t, edit.Changes, 1, "expected one URI key, got %v", keys(edit.Changes))
	require.Contains(t, edit.Changes, uri)
	require.Len(t, edit.Changes[uri], 2)
}

func keys(m map[string][]textEdit) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestTrimmedRangeStripsLeadingAndTrailing exercises the
// trimmedRange helper directly. The setext heading rename path
// only invokes it on text without leading whitespace, so the
// trim loops need direct coverage to participate.
func TestTrimmedRangeStripsLeadingAndTrailing(t *testing.T) {
	t.Parallel()
	start, end := trimmedRange([]byte("  text\t"))
	assert.Equal(t, 2, start)
	assert.Equal(t, 6, end)
	start, end = trimmedRange([]byte("   "))
	assert.Equal(t, start, end)
}

// TestMatchLeadingPairAdjacentNoLabelMatch covers the return-false
// path where the cursor sits in a leading bracket pair, the next
// pair is flush, but neither pair's content matches the label.
func TestMatchLeadingPairAdjacentNoLabelMatch(t *testing.T) {
	t.Parallel()
	row := []byte(`[a][b]`)
	pairs := bracketPairs(row)
	require.Len(t, pairs, 2)
	_, _, ok := matchLeadingPair(row, pairs, 0, "missing")
	assert.False(t, ok)
}

// TestMatchTrailingPairAdjacentNoLabelMatch covers the symmetric
// trailing-pair return-false branch.
func TestMatchTrailingPairAdjacentNoLabelMatch(t *testing.T) {
	t.Parallel()
	row := []byte(`[a][b]`)
	pairs := bracketPairs(row)
	require.Len(t, pairs, 2)
	_, _, ok := matchTrailingPair(row, pairs, 1, "missing")
	assert.False(t, ok)
}

// TestRefUseLabelBytesReturnsFalseWhenCursorOutside ensures the
// outer-loop fallthrough returns false when the cursor lands on
// no bracket pair.
func TestRefUseLabelBytesReturnsFalseWhenCursorOutside(t *testing.T) {
	t.Parallel()
	row := []byte(`[a][b]`)
	_, _, ok := refUseLabelBytes(row, 99, "a")
	assert.False(t, ok)
}
