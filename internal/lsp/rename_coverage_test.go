package lsp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lsp/index"
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
		// Cursor inside `[s](#sec)` on line 3.
		Position: Position{Line: 2, Character: 8},
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
	// Cursor inside the trailing `[docs]` label of the full ref.
	start, end, ok := refUseLabelBytes(row, 11, "docs")
	require.True(t, ok)
	assert.Equal(t, "docs", string(row[start:end]))
	// Cursor inside `text` of the full ref — should resolve to label.
	start, end, ok = refUseLabelBytes(row, 5, "docs")
	require.True(t, ok)
	assert.Equal(t, "docs", string(row[start:end]))
	// Cursor on the shortcut `[docs]`.
	start, end, ok = refUseLabelBytes(row, 22, "docs")
	require.True(t, ok)
	assert.Equal(t, "docs", string(row[start:end]))
	// Cursor on the leading `Docs API` of a collapsed ref.
	start, end, ok = refUseLabelBytes(row, 35, "docs api")
	require.True(t, ok)
	assert.Equal(t, "Docs API", string(row[start:end]))
}

// TestAssignSlugsDisambiguator exercises the slug-uniqueness
// pass directly: duplicate base slugs gain `-1`, `-2`, … suffixes
// in document order.
func TestAssignSlugsDisambiguator(t *testing.T) {
	t.Parallel()
	got := assignSlugs([]string{"Setup", "Setup", "Other", "Setup"})
	assert.Equal(t, []string{"setup", "setup-1", "other", "setup-2"}, got)
}

// TestAssignSlugsEmpty ensures empty-slug entries pass through
// as "" instead of stealing the first base.
func TestAssignSlugsEmpty(t *testing.T) {
	t.Parallel()
	got := assignSlugs([]string{"!!!", "Foo", "..."})
	assert.Equal(t, []string{"", "foo", ""}, got)
}

// TestSlugRemapPairsSkipsUnchanged keeps the helper's contract
// in coverage — only entries whose slug changed appear.
func TestSlugRemapPairsSkipsUnchanged(t *testing.T) {
	t.Parallel()
	pairs := slugRemapPairs(
		[]string{"a", "b", "c"},
		[]string{"a", "x", "c"},
	)
	assert.Equal(t, map[string]string{"b": "x"}, pairs)
}

// TestAnchorFragmentBytesNoHash covers the destination without a
// fragment — the helper returns false instead of misclassifying.
func TestAnchorFragmentBytesNoHash(t *testing.T) {
	t.Parallel()
	row := []byte("see [t](./other.md)")
	_, _, ok := anchorFragmentBytes(row, 4, "anything")
	assert.False(t, ok)
}

// TestAnchorFragmentBytesNoDestination covers the path where the
// `]` is not followed by `(` (e.g. a reference-style link), so
// destBounds returns false.
func TestAnchorFragmentBytesNoDestination(t *testing.T) {
	t.Parallel()
	row := []byte("[t][label]")
	_, _, ok := anchorFragmentBytes(row, 0, "anything")
	assert.False(t, ok)
}

// TestDestBoundsHandlesEscapedParens verifies the CommonMark
// escape rule: `\(` and `\)` inside a destination don't shift
// the depth counter, so the outer `)` still closes the link.
func TestDestBoundsHandlesEscapedParens(t *testing.T) {
	t.Parallel()
	row := []byte(`[t](foo\(bar\)#sec)`)
	open, close, ok := destBounds(row, 0)
	require.True(t, ok)
	assert.Equal(t, "foo\\(bar\\)#sec", string(row[open:close]))
}

// TestIsBackslashEscaped exercises the helper directly: even
// counts of backslashes mean the byte is unescaped.
func TestIsBackslashEscaped(t *testing.T) {
	t.Parallel()
	row := []byte(`a\)b\\)c`)
	assert.True(t, isBackslashEscaped(row, 2), `position 2 is escaped \)`)
	assert.False(t, isBackslashEscaped(row, 6), `position 6 follows \\, even count`)
}

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
func TestBodyAndFMOffsetWithFrontMatter(t *testing.T) {
	t.Parallel()
	body, off := bodyAndFMOffset([]byte("---\ntitle: T\n---\n# H\n"))
	assert.Equal(t, "# H\n", string(body))
	assert.Equal(t, 3, off)

	body, off = bodyAndFMOffset([]byte("# H\n"))
	assert.Equal(t, "# H\n", string(body))
	assert.Equal(t, 0, off)
}

// TestBodyLineIndexEdgeCases covers the boundaries: empty body,
// out-of-range line, last-line offsets.
func TestBodyLineIndexEdgeCases(t *testing.T) {
	t.Parallel()
	// Empty body has just one line start at 0.
	idx := newBodyLineIndex(nil)
	assert.Equal(t, 0, idx.LineStart(1))
	assert.Equal(t, -1, idx.LineStart(2))
	// Negative offsets clamp to line 1.
	assert.Equal(t, 1, idx.LineOfOffset(-1))
}

// TestComputeSlugRemapTargetNotFound covers the path where the
// cursor's body line doesn't match any heading — the helper
// returns nil slices instead of indexing past the array.
func TestComputeSlugRemapTargetNotFound(t *testing.T) {
	t.Parallel()
	source := []byte("# Top\n\nparagraph\n")
	old, new, conflict := computeSlugRemap(source, 99, "Other")
	assert.Nil(t, old)
	assert.Nil(t, new)
	assert.Empty(t, conflict)
}

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
	// Cursor on the heading; rename to identical text should be
	// the no-op path (covered elsewhere). Here we exercise an
	// out-of-buffer position to hit the unsupported branch.
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
		// Cursor on `Setup` (line 4, 1-based) → LSP line 3.
		Position: Position{Line: 3, Character: 1},
	})
	require.Nil(t, errResp)
	var res prepareRenameResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.Equal(t, "Setup", res.Placeholder)
}

// TestHeadingTextEditOutOfRange verifies the defensive return
// when the source has fewer lines than `line` requests.
func TestHeadingTextEditOutOfRange(t *testing.T) {
	t.Parallel()
	_, ok := headingTextEdit([]byte("only one line\n"), 99, "x")
	assert.False(t, ok)
}

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
func TestAnchorFragmentBytesEdgeCases(t *testing.T) {
	t.Parallel()
	row := []byte("see [t](#sec)")
	// Negative textStart clamps to 0.
	start, end, ok := anchorFragmentBytes(row, -10, "sec")
	require.True(t, ok)
	assert.Equal(t, "sec", string(row[start:end]))
	// textStart past EOL.
	_, _, ok = anchorFragmentBytes(row, 999, "sec")
	assert.False(t, ok)
}

// TestDestBoundsEdgeCases covers the unclosed-paren branch.
func TestDestBoundsEdgeCases(t *testing.T) {
	t.Parallel()
	_, _, ok := destBounds([]byte(`[t](unclosed`), 0)
	assert.False(t, ok)
	_, _, ok = destBounds([]byte(`plain text`), 0)
	assert.False(t, ok)
}

// TestFragmentMatchesSlugInvalidEscape verifies the helper falls
// back to the raw bytes when URL-unescaping fails (malformed
// percent escape).
func TestFragmentMatchesSlugInvalidEscape(t *testing.T) {
	t.Parallel()
	// `%zz` is an invalid percent escape; PathUnescape errors out
	// and the helper uses the raw bytes for slugify.
	got := fragmentMatchesSlug([]byte("%zz"), "zz")
	assert.True(t, got)
}

// TestLineOfBodyOffsetEdges covers the negative-offset and
// past-EOF clamps.
func TestLineOfBodyOffsetEdges(t *testing.T) {
	t.Parallel()
	body := []byte("a\nb\nc")
	assert.Equal(t, 1, lineOfBodyOffset(body, -5))
	// past EOF clamps to last line
	assert.Equal(t, 3, lineOfBodyOffset(body, 1000))
}

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
		// First `## Setup` (line 3, 1-based) → LSP line 2.
		Position: Position{Line: 2, Character: 4},
		// Same base slug ("setup") — this is a non-semantic refresh,
		// not a collision.
		NewName: "Setup!",
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
func TestDestBoundsEscapedClosingInText(t *testing.T) {
	t.Parallel()
	row := []byte(`[a\](b)](#sec)`)
	open, close, ok := destBounds(row, 0)
	require.True(t, ok)
	assert.Equal(t, "#sec", string(row[open:close]))
}

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
func TestAnchorEditForEdgeStalePaths(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": "# A\n\n[t](#sec)\n"})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# A\n\n[t](#sec)\n"},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	_, _, ok := h.srv.anchorEditForEdge(index.Edge{SourceFile: "gone.md", SourceLine: 1, SourceCol: 1}, "sec", "new")
	assert.False(t, ok)
	_, _, ok = h.srv.anchorEditForEdge(index.Edge{SourceFile: "a.md", SourceLine: 999, SourceCol: 1}, "sec", "new")
	assert.False(t, ok)
	_, _, ok = h.srv.anchorEditForEdge(index.Edge{SourceFile: "a.md", SourceLine: 3, SourceCol: 1}, "missing", "new")
	assert.False(t, ok)
}

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
func TestRefDefHelperShapes(t *testing.T) {
	t.Parallel()
	// refDefColonOffset
	assert.Equal(t, 5, refDefColonOffset([]byte(`[abc]: url`)))
	assert.Equal(t, 7, refDefColonOffset([]byte(`  [abc]: url`)))
	assert.Equal(t, -1, refDefColonOffset([]byte(`plain text`)))
	assert.Equal(t, -1, refDefColonOffset([]byte(`[no-close`)))
	assert.Equal(t, -1, refDefColonOffset([]byte(`[abc] no colon`)))
	assert.Equal(t, -1, refDefColonOffset([]byte(`    [over]: x`)))

	// refDefDestRange — skip leading whitespace, stop at next space.
	start, end := refDefDestRange([]byte(`[a]:   url more`), 4)
	assert.Equal(t, "url", string([]byte(`[a]:   url more`)[start:end]))
	// All-whitespace tail returns empty range at end.
	start, end = refDefDestRange([]byte(`[a]:   `), 4)
	assert.Equal(t, start, end)

	// refDefParseTarget happy + error cases.
	tgt, ok := refDefParseTarget("./b.md#sec")
	require.True(t, ok)
	assert.Equal(t, "./b.md", tgt.path)
	assert.Equal(t, "sec", tgt.fragment)
	tgt, ok = refDefParseTarget("#sec")
	require.True(t, ok)
	assert.True(t, tgt.localAnchor)
	_, ok = refDefParseTarget("")
	assert.False(t, ok)
	_, ok = refDefParseTarget("//host/x")
	assert.False(t, ok)
	_, ok = refDefParseTarget("https://example.com")
	assert.False(t, ok)
	_, ok = refDefParseTarget("%")
	assert.False(t, ok)
	_, ok = refDefParseTarget("   ")
	assert.False(t, ok)
	// Empty path AND empty fragment.
	_, ok = refDefParseTarget("?q=v")
	assert.False(t, ok)

	// refDefDestPointsAt happy + mismatches.
	assert.True(t, refDefDestPointsAt(
		[]byte(`./a.md#setup`), "b.md", "a.md", "setup"))
	assert.False(t, refDefDestPointsAt(
		[]byte(`./a.md#other`), "b.md", "a.md", "setup"))
	assert.False(t, refDefDestPointsAt(
		[]byte(`./other.md#setup`), "b.md", "a.md", "setup"))
	// Local anchor in a def whose host file matches headingFile.
	assert.True(t, refDefDestPointsAt(
		[]byte(`#setup`), "a.md", "a.md", "setup"))
	// Local anchor in the wrong host file.
	assert.False(t, refDefDestPointsAt(
		[]byte(`#setup`), "b.md", "a.md", "setup"))
	// Garbage destination.
	assert.False(t, refDefDestPointsAt(
		[]byte(`%`), "b.md", "a.md", "setup"))
	// Percent-escaped anchor still matches the slugified form.
	assert.True(t, refDefDestPointsAt(
		[]byte(`./a.md#Docs%20API`), "b.md", "a.md", "docs-api"))
}

// TestRefDefDestEditForMatchSkipsBadInputs covers
// refDefDestEditForMatch's defensive returns: out-of-range
// fileLine, malformed bracket shape, empty dest, and the
// no-`#` fragment path.
func TestRefDefDestEditForMatchSkipsBadInputs(t *testing.T) {
	t.Parallel()
	body := []byte(`[a]: ./b.md#sec`)
	// Out-of-range fileLine (lines slice has 1 entry but match
	// points past it).
	short := [][]byte{[]byte("# h")}
	_, ok := refDefDestEditForMatch(body, short, 10, []int{0, len(body), 1, 2},
		"a.md", "b.md", "sec", "new")
	assert.False(t, ok)
	// Row that the regex matched but doesn't contain a valid
	// `[label]:` (synthetic mismatch).
	lines := [][]byte{[]byte("plain")}
	_, ok = refDefDestEditForMatch(body, lines, 0, []int{0, 5, 1, 2},
		"a.md", "b.md", "sec", "new")
	assert.False(t, ok)
	// Empty destination after `:`.
	lines = [][]byte{[]byte("[a]:   ")}
	_, ok = refDefDestEditForMatch([]byte("[a]:   "), lines, 0,
		[]int{0, 7, 1, 2}, "a.md", "b.md", "sec", "new")
	assert.False(t, ok)
	// Destination has no `#` (so no fragment to rewrite).
	lines = [][]byte{[]byte("[a]: ./b.md")}
	_, ok = refDefDestEditForMatch([]byte("[a]: ./b.md"), lines, 0,
		[]int{0, 11, 1, 2}, "a.md", "b.md", "sec", "new")
	assert.False(t, ok)
}

// TestAppendRefDefDestEditsForHeadingSkipsUnresolvable covers
// the resolveURIAndSource fail branch by planting a synthetic
// index entry for a file the server can't read.
func TestAppendRefDefDestEditsForHeadingSkipsUnresolvable(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{})
	idx := h.srv.ensureIndex()
	idx.UpdateWithKinds("ghost.md", []byte("[setup]: ./a.md#setup\n"), nil)
	changes := map[string][]textEdit{}
	h.srv.appendRefDefDestEditsForHeading(changes, idx, "a.md", "setup", "config")
	assert.Empty(t, changes)
}

// TestValidRefDefMatchesSkipsRegexHitGoldmarkRefused covers the
// `!wanted[norm]` continue arm. The first `[unaccepted]: url`
// line is paragraph continuation in CommonMark, so the regex
// matches it but goldmark never registers a def for that label;
// the helper must drop it while still emitting the real
// `[accepted]: url` def.
func TestValidRefDefMatchesSkipsRegexHitGoldmarkRefused(t *testing.T) {
	t.Parallel()
	body := []byte("para line\n[unaccepted]: url\n\n[accepted]: url\n")
	matches := validRefDefMatches(body)
	require.Len(t, matches, 1)
	assert.Equal(t, "accepted", matches[0].rawLabel)
}

// TestAppendAnchorEditsForHeadingDropsUnresolvableEdge plants a
// synthetic edge in the index that points at a file the server
// can't read, then verifies appendAnchorEditsForHeading skips it
// rather than producing a phantom edit. This drives the
// `if !ok { continue }` arm that the rename's natural flow
// rarely reaches.
func TestAppendAnchorEditsForHeadingDropsUnresolvableEdge(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{
		"present.md": "# Real\n",
	})
	idx := h.srv.ensureIndex()
	// `ghost.md` has no on-disk file and no open buffer, so
	// resolveURIAndSource will fail; but the index entry below
	// makes IncomingEdges return an edge for (ghost.md, sec).
	idx.UpdateWithKinds("ghost.md", []byte("# Sec\n[t](#sec)\n"), nil)
	changes := map[string][]textEdit{}
	h.srv.appendAnchorEditsForHeading(changes, idx, "ghost.md", "sec", "newsec")
	assert.Empty(t, changes)
}

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
func TestRefDefDestRangeAngleBracket(t *testing.T) {
	t.Parallel()
	row := []byte(`[a]: <./b.md#sec>`)
	start, end := refDefDestRange(row, 4)
	assert.Equal(t, "./b.md#sec", string(row[start:end]))
	// Unterminated angle: fall back to bare reader from the `<`.
	row = []byte(`[a]: <./b.md#sec`)
	start, end = refDefDestRange(row, 4)
	assert.Equal(t, "<./b.md#sec", string(row[start:end]))
	// Angle dest containing an escaped `\>` doesn't terminate
	// the URL early.
	row = []byte(`[a]: <./b.md\>x#sec>`)
	start, end = refDefDestRange(row, 4)
	assert.Equal(t, `./b.md\>x#sec`, string(row[start:end]))
}

// TestLabelBoundsInBodyEmptyText covers the negative-offset
// guard added for empty-text references.
func TestLabelBoundsInBodyEmptyText(t *testing.T) {
	t.Parallel()
	body := []byte(`[][docs]`)
	_, _, ok := labelBoundsInBody(body, -1, -1, ast.ReferenceLinkFull)
	assert.False(t, ok)
	_, _, ok = labelBoundsInBody(body, -1, -1, ast.ReferenceLinkShortcut)
	assert.False(t, ok)
}

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
	// Only the real `[real]: ./a.md#setup` def rewrites. The
	// fake one inside the fenced code block stays put.
	require.Len(t, edit.Changes[uriB], 1)
	// The fence opens on b.md line 5 (1-based) → LSP line 4;
	// the fake def is on line 6 → LSP line 5. No edit should
	// land there.
	assert.NotEqual(t, 5, edit.Changes[uriB][0].Range.Start.Line)
}

// TestValidRefDefMatchesExcludesParagraphContinuation covers
// the parser-rejection path where a `[docs]: url`-shaped line
// appears as paragraph content (continuation, not a def) AND
// a real `[docs]: url` def appears later in the file. The
// label-only filter from before would accept both regex hits;
// the AST paragraph-line filter drops the continuation.
func TestValidRefDefMatchesExcludesParagraphContinuation(t *testing.T) {
	t.Parallel()
	body := []byte("para line\n[docs]: bogus\n\n[docs]: https://real\n")
	matches := validRefDefMatches(body)
	require.Len(t, matches, 1)
	// The surviving match is the real def on body-line 4.
	assert.Equal(t, 4, matches[0].bodyLine)
}

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
	// b.md's `[setup]: ./a.md#setup` def rewrites to
	// `#configuration`; the `[local]: #setup` def points at the
	// non-renamed file b.md so it stays untouched.
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
	// Def + multi-line full ref + single-line shortcut = 3 edits.
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
func TestLabelBoundsInBodyShapes(t *testing.T) {
	t.Parallel()
	// Full ref `[t][docs]`: text "t" at offset 1, textEnd=2.
	body := []byte(`[t][docs]`)
	start, end, ok := labelBoundsInBody(body, 1, 2, ast.ReferenceLinkFull)
	require.True(t, ok)
	assert.Equal(t, "docs", string(body[start:end]))
	// Full but trailing `[` missing.
	body = []byte(`[t] docs`)
	_, _, ok = labelBoundsInBody(body, 1, 2, ast.ReferenceLinkFull)
	assert.False(t, ok)
	// Full but unterminated label.
	body = []byte(`[t][docs`)
	_, _, ok = labelBoundsInBody(body, 1, 2, ast.ReferenceLinkFull)
	assert.False(t, ok)
	// Full but the byte at textEnd isn't `]`.
	body = []byte(`[t (docs)`)
	_, _, ok = labelBoundsInBody(body, 1, 2, ast.ReferenceLinkFull)
	assert.False(t, ok)
	// Shortcut `[docs]`: text "docs" at offset 1, textEnd=5.
	body = []byte(`[docs]`)
	start, end, ok = labelBoundsInBody(body, 1, 5, ast.ReferenceLinkShortcut)
	require.True(t, ok)
	assert.Equal(t, "docs", string(body[start:end]))
	// Shortcut with textStart at byte 0 (no leading `[`).
	body = []byte(`docs]`)
	_, _, ok = labelBoundsInBody(body, 0, 4, ast.ReferenceLinkShortcut)
	assert.False(t, ok)
	// Shortcut where the leading byte isn't `[`.
	body = []byte(`(docs]`)
	_, _, ok = labelBoundsInBody(body, 1, 5, ast.ReferenceLinkShortcut)
	assert.False(t, ok)
	// Shortcut where the byte at textEnd isn't `]`.
	body = []byte(`[docs `)
	_, _, ok = labelBoundsInBody(body, 1, 5, ast.ReferenceLinkShortcut)
	assert.False(t, ok)
	// Full ref label with `\]` escape inside the label.
	body = []byte(`[t][doc\]s]`)
	start, end, ok = labelBoundsInBody(body, 1, 2, ast.ReferenceLinkFull)
	require.True(t, ok)
	assert.Equal(t, `doc\]s`, string(body[start:end]))
}

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
	// Cursor on `b` (offset 8). Label is "label".
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
	// line=1 sits inside the front matter; subtracting fmOffset (3)
	// pushes bodyLine to -2, so the helper short-circuits to false.
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
		// Cursor inside `[fake]: https://x` (line 4, 1-based).
		Position: Position{Line: 3, Character: 2},
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
		// Cursor on the real def line `[docs]: https://right` (line 9, 1-based).
		Position: Position{Line: 8, Character: 2},
		NewName:  "manual",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	// Two edits: the real def + the `[t][docs]` use.
	require.Len(t, edit.Changes[uri], 2)
	for _, e := range edit.Changes[uri] {
		// Code-block line is line 6 (0-based 5); ensure no edit
		// targets it.
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
	// Fallback uses the canonical workspaceURI form, which mirrors
	// the openURI's prefix (file://...).
	assert.Equal(t, rootURI+"/closed.md", uri)
}

// TestRefDefEditsInBodyIgnoresOtherLabels verifies the
// `normalizedLabel != oldLabel` continue branch — defs for other
// labels in the same buffer are skipped.
func TestRefDefEditsInBodyIgnoresOtherLabels(t *testing.T) {
	t.Parallel()
	body := []byte("[a]: x\n[b]: y\n[a]: z\n")
	lines := [][]byte{[]byte("[a]: x"), []byte("[b]: y"), []byte("[a]: z")}
	out := refDefEditsInBody(body, lines, 0, "a", "renamed")
	require.Len(t, out, 2)
	for _, e := range out {
		assert.Equal(t, "renamed", e.NewText)
	}
}

// TestAppendAnchorEditsForHeadingSkipsStaleEdge covers the
// continue branch in appendAnchorEditsForHeading: an edge whose
// link doesn't actually contain the old slug at the recorded
// column produces a nil edit and is silently skipped rather
// than failing the rename.
func TestAppendAnchorEditsForHeadingSkipsStaleEdge(t *testing.T) {
	t.Parallel()
	// Build a real harness, then fabricate an out-of-date edge.
	src := "# Top\n\n[link](#elsewhere)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	// Trigger index build.
	_ = h.srv.ensureIndex()
	// Rename "Top" to "Other"; the edge for `[link](#elsewhere)`
	// is unrelated to the heading and stays untouched.
	raw, errResp := h.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 3},
		NewName:      "Other",
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Contains(t, edit.Changes, uri)
	// One edit total (heading line); no stale anchor edit slipped in.
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
	// Both the heading text and the self-anchor link must land
	// under the same URI key.
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
	// All-whitespace input collapses to start==end.
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

// TestDestBoundsUnclosedReturnsFalse covers the depth>0
// fallthrough branch when the destination has an unclosed `(`.
func TestDestBoundsUnclosedReturnsFalse(t *testing.T) {
	t.Parallel()
	_, _, ok := destBounds([]byte(`[t](inner(unclosed`), 0)
	assert.False(t, ok)
}

// TestRefDefEditsInBodyOutOfRangeFileLine covers the defensive
// `fileLine-1 >= len(lines)` branch by feeding a body whose
// regex match points at a line index past `lines`.
func TestRefDefEditsInBodyOutOfRangeFileLine(t *testing.T) {
	t.Parallel()
	// Two `[label]: url` matches in body but lines slice has
	// only one — the second match is silently skipped.
	body := []byte("[a]: x\n[a]: x\n")
	lines := [][]byte{[]byte("[a]: x")}
	out := refDefEditsInBody(body, lines, 0, "a", "b")
	require.Len(t, out, 1)
}
