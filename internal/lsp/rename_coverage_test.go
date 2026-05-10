package lsp

import (
	"context"
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
	old, new, conflict := computeSlugRemap(source, 99, "Top", "Other")
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

// silence unused-import warnings if context drops out later.
var _ = context.Background
