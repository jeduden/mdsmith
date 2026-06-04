package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	// didChange at version 2: dirty buffer with trailing spaces. The
	// session's version-keyed parse cache must miss at the new version,
	// force a reparse, and MDS006 must appear in the published
	// diagnostics. (The cache mechanics themselves are unit-tested at the
	// session layer in pkg/mdsmith; here we pin the editor-visible
	// outcome.)
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
}

// TestParseCache_DidCloseReopenReflectsNewText pins didClose's
// invalidation behaviourally: once a buffer is closed and reopened
// (which restarts at version 1) with different content, the republished
// diagnostics reflect the reopened text — a stale version-1 parse from
// the first session must not resurface.
func TestParseCache_DidCloseReopenReflectsNewText(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/close.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\nclean line\n",
		},
	})
	first := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p1 publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(first, &p1))
	assert.Empty(t, p1.Diagnostics, "clean buffer should produce no diagnostics")

	h.notify("textDocument/didClose", didCloseTextDocumentParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Reopen at version 1 with dirty content. If a stale version-1 parse
	// survived didClose, MDS006 would be missed.
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\ndirty line   \n",
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
	assert.True(t, saw006, "reopen at version 1 must reflect the new dirty text, got %+v", p2.Diagnostics)
}

// TestParseCache_ReloadRebuildsSessionOnRootChange pins that a config
// reload to a different .mdsmith.yml rebuilds the per-workspace session.
// Every cache (the version-keyed parse cache, the cross-file read cache)
// is owned by the session and is rooted at the config's directory, so a
// rebuild on a moved config path gives fresh caches keyed against the
// new root — no stale entry from the previous root can survive.
func TestParseCache_ReloadRebuildsSessionOnRootChange(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Seed two distinct config directories on disk so reloadConfig
	// can flip between them via the settings override.
	dirA := t.TempDir()
	dirB := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dirA, ".mdsmith.yml"), []byte("{}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dirB, ".mdsmith.yml"), []byte("{}\n"), 0o600))
	require.NotEqual(t, dirA, dirB, "test setup: the two configs must live in different directories")

	// Point reloadConfig at dirA's config and let it land.
	h.srv.settingsMu.Lock()
	h.srv.settings.ConfigPath = filepath.Join(dirA, ".mdsmith.yml")
	h.srv.settingsMu.Unlock()
	h.srv.reloadConfig()
	_, _, rootA := h.srv.snapshotConfig()
	require.Equal(t, dirA, rootA, "first reload must adopt dirA as the workspace root")
	sessA, _ := h.srv.currentSession()
	require.NotNil(t, sessA, "first reload must build a session")

	// Flip the override to dirB and reload. configPath changes, so
	// reloadConfig must rebuild the session against the new root.
	h.srv.settingsMu.Lock()
	h.srv.settings.ConfigPath = filepath.Join(dirB, ".mdsmith.yml")
	h.srv.settingsMu.Unlock()
	h.srv.reloadConfig()
	_, _, rootB := h.srv.snapshotConfig()
	require.Equal(t, dirB, rootB, "second reload must adopt dirB as the workspace root")
	require.NotEqual(t, rootA, rootB, "the reload must change the workspace root")

	sessB, _ := h.srv.currentSession()
	require.NotNil(t, sessB, "second reload must build a session")
	assert.NotSame(t, sessA, sessB, "a config-path change must rebuild the session with fresh caches")
}
