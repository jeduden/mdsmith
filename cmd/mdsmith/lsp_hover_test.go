package main_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLSPHoverE2E spawns the shared mdsmith binary and drives a
// full initialize → didOpen → hover round-trip for the three cases
// the plan-133 acceptance criteria specify:
//
//   - Hovering over an MDS006 diagnostic returns a markdown body that
//     contains the rule help text.
//   - Hovering inside a <?catalog?> directive (no diagnostic) returns
//     the catalog directive docs.
//   - Hovering on plain prose (no diagnostic, no directive) returns null.
func TestLSPHoverE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LSP hover subprocess test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pipe := startLSPSubprocess(t, ctx, binaryPath)
	assertHoverAdvertised(t, pipe)
	pipe.notify("initialized", map[string]any{})

	// Document layout (0-based lines):
	//   0: "# Title"
	//   1: ""
	//   2: "trailing   "   ← MDS006 (trailing spaces start at char 8)
	//   3: ""
	//   4: "<?catalog"     ← catalog directive start
	//   5: "glob: \"*.md\""
	//   6: "?>"
	uri := "file:///tmp/lsp-hover-e2e.md"
	src := "# Title\n\ntrailing   \n\n<?catalog\nglob: \"*.md\"\n?>\n"
	pipe.openDocument(uri, src)
	pipe.awaitDiagnostics(t, uri, time.Now().Add(30*time.Second))

	assertHoverDiagnostic(t, pipe, uri)
	assertHoverDirective(t, pipe, uri)
	assertHoverNull(t, pipe, uri)

	pipe.shutdown(t)
}

func assertHoverAdvertised(t *testing.T, pipe *lspPipe) {
	t.Helper()
	resp := pipe.request("initialize", 1, map[string]any{
		"capabilities": fullClientCapabilities(),
	})
	require.Equal(t, float64(1), resp["id"])
	res, ok := resp["result"].(map[string]any)
	require.True(t, ok)
	caps, ok := res["capabilities"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, caps["hoverProvider"], "hoverProvider must be advertised in initialize")
}

func assertHoverDiagnostic(t *testing.T, pipe *lspPipe, uri string) {
	t.Helper()
	result := pipe.requestPickResult(t, "textDocument/hover", 10, map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 2, "character": 9},
	})
	require.NotNil(t, result, "hover over MDS006 diagnostic must return content")
	obj, ok := result.(map[string]any)
	require.True(t, ok)
	contents, ok := obj["contents"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "markdown", contents["kind"])
	val, _ := contents["value"].(string)
	assert.Contains(t, val, "MDS006", "hover body must contain the rule ID")
	assert.NotNil(t, obj["range"], "hover over diagnostic must include range")
}

func assertHoverDirective(t *testing.T, pipe *lspPipe, uri string) {
	t.Helper()
	result := pipe.requestPickResult(t, "textDocument/hover", 11, map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 4, "character": 3},
	})
	require.NotNil(t, result, "hover inside catalog directive must return content")
	obj, ok := result.(map[string]any)
	require.True(t, ok)
	contents, ok := obj["contents"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "markdown", contents["kind"])
	val, _ := contents["value"].(string)
	assert.NotEmpty(t, val, "directive hover body must be non-empty")
	assert.NotNil(t, obj["range"], "hover inside directive must include range")
}

func assertHoverNull(t *testing.T, pipe *lspPipe, uri string) {
	t.Helper()
	result := pipe.requestPickResult(t, "textDocument/hover", 12, map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 0, "character": 3},
	})
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Equal(t, "null", string(raw), "hover on plain prose must return null")
}
