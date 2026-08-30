package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// TestInitializeAdvertisesFileOperations checks that the server offers
// workspace.fileOperations.willRename/didRename filtered to Markdown, so
// a capable client sends willRenameFiles before renaming a `.md` file.
func TestInitializeAdvertisesFileOperations(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	resultRaw, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	require.NotNil(t, res.Capabilities.Workspace)
	require.NotNil(t, res.Capabilities.Workspace.FileOperations)
	wr := res.Capabilities.Workspace.FileOperations.WillRename
	require.NotNil(t, wr)
	require.Len(t, wr.Filters, 1)
	assert.Equal(t, "**/*.{md,markdown}", wr.Filters[0].Pattern.Glob)
	require.NotNil(t, res.Capabilities.Workspace.FileOperations.DidRename)
}

// TestWillRenameFilesRewritesReferences drives a three-file workspace:
// b.md and c.md both link to a.md by path, and a.md links out to
// b.md. Renaming a.md → docs/a.md must return edits that rewrite the
// incoming links and the moved file's own outbound link.
func TestWillRenameFilesRewritesReferences(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n\nSee [b](./b.md).\n"
	srcB := "# Beta\n\n[a](./a.md)\n"
	srcC := "# Gamma\n\n[a](./a.md) and [a2](./a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "c.md": srcC,
	})
	uriA := rootURI + "/a.md"
	uriB := rootURI + "/b.md"
	uriC := rootURI + "/c.md"
	for _, d := range []struct{ uri, src string }{{uriA, srcA}, {uriB, srcB}, {uriC, srcC}} {
		h.notify("textDocument/didOpen", didOpenTextDocumentParams{
			TextDocument: textDocumentItem{URI: d.uri, LanguageID: "markdown", Version: 1, Text: d.src},
		})
		_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	}

	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{{OldURI: uriA, NewURI: rootURI + "/docs/a.md"}},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))

	// Incoming links in b.md and c.md are rewritten to the new path.
	require.Contains(t, edit.Changes, uriB)
	require.Len(t, edit.Changes[uriB], 1)
	// b.md is at the root and used an explicit `./`, so the prefix is kept.
	assert.Equal(t, "./docs/a.md", edit.Changes[uriB][0].NewText)
	require.Contains(t, edit.Changes, uriC)
	assert.Len(t, edit.Changes[uriC], 2)
	// The moved file's own outbound link to b.md is recomputed.
	require.Contains(t, edit.Changes, uriA)
	assert.Equal(t, "../b.md", edit.Changes[uriA][0].NewText)
}

// TestWillRenameFilesDestinationExistsSkips confirms a move whose
// destination already exists contributes no edit rather than failing
// the request.
func TestWillRenameFilesDestinationExistsSkips(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n"
	srcB := "# Beta\n\n[a](./a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{{OldURI: rootURI + "/a.md", NewURI: rootURI + "/b.md"}},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	assert.Empty(t, edit.Changes)
}

func TestWillRenameFilesMalformedAndNoop(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": "# A\n"})

	// Malformed params → InvalidParams error, no crash.
	_, errResp := h.request("workspace/willRenameFiles", []int{1})
	require.NotNil(t, errResp)

	// old == new URI → the file is skipped, yielding an empty edit.
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{{OldURI: rootURI + "/a.md", NewURI: rootURI + "/a.md"}},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	assert.Empty(t, edit.Changes)
}

func TestDidRenameFilesMalformedAndNonMarkdown(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": "# Alpha\n"})
	// Warm the index.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: "Alpha"})

	// Malformed params are ignored (notification, no reply).
	h.notify("workspace/didRenameFiles", []int{1})

	// A rename to a non-Markdown path drops the old entry but does not
	// index the new (non-Markdown) file.
	h.notify("workspace/didRenameFiles", renameFilesParams{
		Files: []fileRename{{OldURI: rootURI + "/a.md", NewURI: rootURI + "/a.txt"}},
	})
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "Alpha"})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	assert.Empty(t, hits, "old path dropped, new non-Markdown path not indexed")
}

// TestDidRenameFilesSwapsIndexPath confirms the notification updates the
// warm index: after the client renames a.md to c.md on disk and reports
// it, a workspace symbol search finds the moved file's heading under the
// new path and no longer finds the old one.
func TestDidRenameFilesSwapsIndexPath(t *testing.T) {
	t.Parallel()
	h, dir, rootURI := rootedHarness(t, map[string]string{"a.md": "# Alpha\n"})

	// Warm the index and confirm the old heading is present.
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "Alpha"})
	require.Nil(t, errResp)
	var before []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &before))
	require.NotEmpty(t, before)

	// The client performs the rename on disk, then notifies the server.
	require.NoError(t, os.Remove(filepath.Join(dir, "a.md")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"), []byte("# Gamma\n"), 0o644))
	h.notify("workspace/didRenameFiles", renameFilesParams{
		Files: []fileRename{{OldURI: rootURI + "/a.md", NewURI: rootURI + "/c.md"}},
	})

	// The new heading is now indexed and the old one is gone.
	raw, errResp = h.request("workspace/symbol", workspaceSymbolParams{Query: "Gamma"})
	require.Nil(t, errResp)
	var gamma []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &gamma))
	assert.NotEmpty(t, gamma, "moved file's heading indexed under new path")

	raw, errResp = h.request("workspace/symbol", workspaceSymbolParams{Query: "Alpha"})
	require.Nil(t, errResp)
	var alpha []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &alpha))
	assert.Empty(t, alpha, "old path dropped from the index")
}
