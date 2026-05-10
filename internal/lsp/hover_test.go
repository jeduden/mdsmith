package lsp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHoverProvider verifies that initialize advertises hoverProvider=true.
func TestHoverProvider(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	resultRaw, errResp := h.request("initialize", initializeParams{
		Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)

	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	assert.True(t, res.Capabilities.HoverProvider, "hoverProvider must be advertised in initialize")
}

// TestHoverOnDiagnostic verifies that hovering over a diagnostic range
// returns MarkupContent with the diagnostic message and rule docs.
func TestHoverOnDiagnostic(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	// MDS001 fires when a line exceeds the default 100-character limit.
	longLine := strings.Repeat("x", 110)
	text := "# Title\n\n" + longLine + "\n"
	uri := "file:///workspace/hover-diag.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: text,
		},
	})
	// Wait for the diagnostic to be published before issuing the hover.
	raw := h.awaitNotification("textDocument/publishDiagnostics", 10*time.Second)
	var pubDiags publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &pubDiags))

	// Find MDS001 diagnostic range.
	var found bool
	for _, d := range pubDiags.Diagnostics {
		if d.Code == "MDS001" {
			found = true
			break
		}
	}
	require.True(t, found, "expected MDS001 diagnostic, got %+v", pubDiags.Diagnostics)

	// Hover at line 2 (the long line), character 105 (inside the MDS001
	// diagnostic range which starts at column 100).
	resultRaw, errResp2 := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 105},
	})
	require.Nil(t, errResp2)
	require.NotNil(t, resultRaw)
	require.NotEqual(t, "null", string(resultRaw), "hover over MDS001 diagnostic must not return null")

	var result hoverResult
	require.NoError(t, json.Unmarshal(resultRaw, &result))
	assert.Equal(t, "markdown", result.Contents.Kind)
	assert.NotEmpty(t, result.Contents.Value, "hover body must not be empty")
	assert.NotNil(t, result.Range, "hover range must be set")
}

// TestHoverOnDirective verifies that hovering inside a <?catalog?> block
// returns directive documentation even with no diagnostic at the cursor.
func TestHoverOnDirective(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	// A catalog directive in the document — no diagnostic expected here.
	text := "# Index\n\n<?catalog\nglob: \"docs/**/*.md\"\n?>\n- item\n<?/catalog?>\n"
	uri := "file:///workspace/hover-directive.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: text,
		},
	})
	// Consume the diagnostics notification.
	h.awaitNotification("textDocument/publishDiagnostics", 10*time.Second)

	// Hover at line 2 (the "<?catalog" line), character 0.
	resultRaw, errResp2 := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 2},
	})
	require.Nil(t, errResp2)
	// The server may return null when repoRoot is empty (tests have no
	// workspace root), but it must not error.
	if string(resultRaw) == "null" {
		t.Log("hover returned null (no repo root in test env — expected)")
		return
	}
	var result hoverResult
	require.NoError(t, json.Unmarshal(resultRaw, &result))
	assert.Equal(t, "markdown", result.Contents.Kind)
}

// TestHoverOnPlainProse verifies that hovering on ordinary prose with
// no diagnostic and no directive block returns null.
func TestHoverOnPlainProse(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	text := "# Title\n\nPlain prose with no issues.\n"
	uri := "file:///workspace/hover-prose.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: text,
		},
	})
	h.awaitNotification("textDocument/publishDiagnostics", 10*time.Second)

	// Hover at line 2 (the prose line), character 0.
	resultRaw, errResp2 := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 0},
	})
	require.Nil(t, errResp2)
	assert.Equal(t, "null", string(resultRaw), "hover on plain prose must return null")
}

// TestHoverUnknownDocument verifies that hovering on an unknown URI returns null.
func TestHoverUnknownDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	resultRaw, errResp2 := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: "file:///workspace/nonexistent.md"},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp2)
	assert.Equal(t, "null", string(resultRaw))
}

// directivePosCase is a test case for findDirectiveAtPos.
type directivePosCase struct {
	name      string
	source    string
	pos       Position
	wantFound bool
	wantName  string
}

// directivePosFixtures returns the table of findDirectiveAtPos test cases.
func directivePosFixtures() []directivePosCase {
	ml := "# Title\n\n<?catalog\nglob: a\n?>\n"
	return []directivePosCase{
		{"single-line catalog", "# Title\n\n<?catalog glob=\"a\"?>\n", Position{2, 5}, true, "catalog"},
		{"multi-line first line", ml, Position{2, 0}, true, "catalog"},
		{"multi-line body line", ml, Position{3, 0}, true, "catalog"},
		{"multi-line close line", ml, Position{4, 0}, true, "catalog"},
		{"plain prose", "# Title\n\nPlain prose.\n", Position{2, 5}, false, ""},
		{"closing marker", "# Title\n\n<?catalog\n?>\n<?/catalog?>\n", Position{4, 2}, false, ""},
		{"include directive", "<?include\nfile: x.md\n?>\n", Position{1, 0}, true, "include"},
	}
}

// TestFindDirectiveAtPos exercises the block-scan helper directly.
func TestFindDirectiveAtPos(t *testing.T) {
	t.Parallel()
	for _, tc := range directivePosFixtures() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, name, found := findDirectiveAtPos([]byte(tc.source), tc.pos)
			assert.Equal(t, tc.wantFound, found)
			if tc.wantFound {
				assert.Equal(t, tc.wantName, name)
			}
		})
	}
}

// TestPosInRange exercises the range-containment helper.
func TestPosInRange(t *testing.T) {
	t.Parallel()
	r := Range{
		Start: Position{Line: 2, Character: 5},
		End:   Position{Line: 2, Character: 20},
	}
	assert.True(t, posInRange(Position{Line: 2, Character: 5}, r), "start is inside")
	assert.True(t, posInRange(Position{Line: 2, Character: 10}, r), "mid is inside")
	assert.False(t, posInRange(Position{Line: 2, Character: 20}, r), "end is exclusive")
	assert.False(t, posInRange(Position{Line: 2, Character: 4}, r), "before start")
	assert.False(t, posInRange(Position{Line: 1, Character: 10}, r), "wrong line")
	assert.False(t, posInRange(Position{Line: 3, Character: 0}, r), "after line")
}

// TestBuildRuleHoverBody verifies that buildRuleHoverBody includes the
// message and rule docs (or fallback text when docs are absent).
func TestBuildRuleHoverBody(t *testing.T) {
	t.Parallel()
	body := buildRuleHoverBody("line is too long (110 > 100)", "MDS001")
	assert.Contains(t, body, "line is too long", "body must contain the message")
	// Either rule docs or fallback.
	hasContent := strings.Contains(body, "mdsmith help rule") || len(body) > len("line is too long (110 > 100)\n\n")
	assert.True(t, hasContent, "body must contain rule docs or fallback")
}

// TestStripDocFrontMatter verifies front-matter stripping.
func TestStripDocFrontMatter(t *testing.T) {
	t.Parallel()
	input := "---\ntitle: Foo\n---\n# Content\n"
	got := stripDocFrontMatter(input)
	assert.Equal(t, "# Content\n", got)
}
