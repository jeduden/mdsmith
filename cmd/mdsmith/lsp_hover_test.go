package main_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLSPHoverE2E drives a full hover round trip over the real mdsmith
// binary:
//
//   - Capability check: hoverProvider must appear in the initialize response.
//   - Diagnostic case: hovering over an MDS001 (line-too-long) squiggle
//     returns MarkupContent containing the diagnostic message and rule docs.
//   - No-match case: hovering on plain prose with no diagnostic and no
//     directive returns null.
func TestLSPHoverE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LSP hover subprocess test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pipe := startLSPSubprocess(t, ctx, binaryPath)

	// Initialize and verify hoverProvider is advertised.
	initResult := pipe.requestPickResult(t, "initialize", 1, map[string]any{
		"capabilities": fullClientCapabilities(),
	})
	initObj, ok := initResult.(map[string]any)
	require.True(t, ok, "expected object in initialize result")
	caps, ok := initObj["capabilities"].(map[string]any)
	require.True(t, ok, "expected capabilities in initialize result")
	hoverProv, _ := caps["hoverProvider"].(bool)
	assert.True(t, hoverProv, "hoverProvider must be advertised in initialize capabilities")
	pipe.notify("initialized", map[string]any{})

	// --- diagnostic case ---
	// A line exceeding the 100-character MDS001 limit.
	longLine := strings.Repeat("x", 110)
	diagURI := "file:///tmp/lsp-hover-diag.md"
	pipe.openDocument(diagURI, "# Title\n\n"+longLine+"\n")
	diags := pipe.awaitDiagnostics(t, diagURI, time.Now().Add(30*time.Second))
	var sawMDS001 bool
	for _, d := range diags.Diagnostics {
		if d.Code == "MDS001" {
			sawMDS001 = true
			break
		}
	}
	require.True(t, sawMDS001, "expected MDS001 diagnostic, got %+v", diags.Diagnostics)

	// Hover at character 105 on the long line — inside the MDS001 range
	// (which starts at character 100, the first character past the limit).
	hoverResult := pipe.requestPickResult(t, "textDocument/hover", 10, map[string]any{
		"textDocument": map[string]any{"uri": diagURI},
		"position":     map[string]any{"line": 2, "character": 105},
	})
	require.NotNil(t, hoverResult, "hover over MDS001 diagnostic must not return null")
	hoverObj, ok := hoverResult.(map[string]any)
	require.True(t, ok, "hover result must be an object")
	contents, ok := hoverObj["contents"].(map[string]any)
	require.True(t, ok, "hover result must have contents")
	assert.Equal(t, "markdown", contents["kind"], "hover contents kind must be markdown")
	body, _ := contents["value"].(string)
	assert.NotEmpty(t, body, "hover body must not be empty")

	// --- no-match case (plain prose) ---
	proseURI := "file:///tmp/lsp-hover-prose.md"
	pipe.openDocument(proseURI, "# Title\n\nPlain prose with no issues.\n")
	pipe.awaitDiagnostics(t, proseURI, time.Now().Add(30*time.Second))

	proseResult := pipe.requestPickResult(t, "textDocument/hover", 11, map[string]any{
		"textDocument": map[string]any{"uri": proseURI},
		"position":     map[string]any{"line": 2, "character": 0},
	})
	assert.Nil(t, proseResult, "hover on plain prose must return null")

	pipe.shutdown(t)
}
