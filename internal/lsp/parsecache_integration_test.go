package lsp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseCache_DidChangeReflectsNewText pins the integration
// boundary the parse cache must hold: a didOpen → runLint →
// didChange → runLint sequence must surface diagnostics for the
// *post-edit* text. A regression where didChange forgot to bump the
// version, forgot to invalidate, or where the cache served the
// old-version *lint.File would publish stale diagnostics here.
//
// The fixture pairs a clean buffer (no findings) with an edit that
// introduces trailing whitespace — MDS006 — so the test gates on a
// rule whose presence vs absence is unambiguous.
func TestParseCache_DidChangeReflectsNewText(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/parsecache.md"
	clean := "# Hi\n\nclean line\n"
	dirty := "# Hi\n\ndirty line   \n"

	// didOpen: warm the cache with the clean buffer at version 1.
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: clean,
		},
	})
	first := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p1 publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(first, &p1))
	assert.Empty(t, p1.Diagnostics, "clean buffer should produce no diagnostics on the first lint")

	// Sanity: the server populated the parse cache. The relative
	// path the cache keys off is the absolute path itself when no
	// workspace root is configured (workspaceRelative returns the
	// input). Either way, an entry must exist for version 1.
	_, ok := h.srv.parseCache.Get("/workspace/parsecache.md", 1)
	require.True(t, ok, "parse cache should hold the version-1 entry after the first lint")

	// didChange at version 2: dirty buffer with trailing spaces.
	// The cache must invalidate, force a reparse, and MDS006 must
	// appear in the published diagnostics.
	h.notify("textDocument/didChange", didChangeTextDocumentParams{
		TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{
			{Text: dirty},
		},
	})
	second := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p2 publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(second, &p2))

	var saw006 bool
	for _, d := range p2.Diagnostics {
		if d.Code == "MDS006" {
			saw006 = true
			break
		}
	}
	assert.True(t, saw006, "expected MDS006 after didChange; got %+v", p2.Diagnostics)

	// The version-1 entry must be gone: didChange invalidated it.
	// The version-2 entry must exist: the post-edit runLint stored
	// the fresh parse.
	_, ok = h.srv.parseCache.Get("/workspace/parsecache.md", 1)
	assert.False(t, ok, "didChange must drop the version-1 entry so a stale parse cannot resurface")
	_, ok = h.srv.parseCache.Get("/workspace/parsecache.md", 2)
	assert.True(t, ok, "the post-edit lint must store a fresh entry at the new version")
}

// TestParseCache_DidCloseDropsEntry pins didClose's invalidation:
// once the buffer is closed the cache entry must be gone so a
// reopen (which restarts at version 1) cannot accidentally serve a
// stale *File parsed against the previous session's content.
func TestParseCache_DidCloseDropsEntry(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/close.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\nclean\n",
		},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, ok := h.srv.parseCache.Get("/workspace/close.md", 1)
	require.True(t, ok, "the open lint must populate the cache")

	h.notify("textDocument/didClose", didCloseTextDocumentParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	_, ok = h.srv.parseCache.Get("/workspace/close.md", 1)
	assert.False(t, ok, "didClose must drop the parse cache entry")
}
