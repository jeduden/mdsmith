package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestWillRenameFilesRewritesWikilink covers a basename-changing move:
// guide.md's `[[api]]` wikilink follows api.md to its new stem, which
// exercises the workspace's wikilink-edge query.
func TestWillRenameFilesRewritesWikilink(t *testing.T) {
	t.Parallel()
	srcA := "# API\n"
	srcG := "See [[api]] for details.\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"api.md": srcA, "guide.md": srcG})
	for _, d := range []struct{ uri, src string }{
		{rootURI + "/api.md", srcA}, {rootURI + "/guide.md", srcG},
	} {
		h.notify("textDocument/didOpen", didOpenTextDocumentParams{
			TextDocument: textDocumentItem{URI: d.uri, LanguageID: "markdown", Version: 1, Text: d.src},
		})
		_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	}

	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{{OldURI: rootURI + "/api.md", NewURI: rootURI + "/service.md"}},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	uriG := rootURI + "/guide.md"
	require.Contains(t, edit.Changes, uriG)
	assert.Equal(t, "service", edit.Changes[uriG][0].NewText)
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

// TestDidRenameFilesRejectsOutOfWorkspaceTarget locks that a rename whose
// destination resolves outside the workspace root is never read into the
// index. workspaceRelative returns an out-of-root path unchanged (not
// ""), so the newRel=="" guard alone did not catch it and the server
// would read an arbitrary .md file off disk via symbolWorkspace.ReadFile;
// the insideWorkspace check closes that gap (and refuses a symlink
// escape the same way the symbol read path does).
func TestDidRenameFilesRejectsOutOfWorkspaceTarget(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": "# Alpha\n"})
	// Warm the index.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: "Alpha"})

	// A markdown file outside the workspace root, in its own temp dir, the
	// server must not pull into the index on a client rename notification.
	outside := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(outside, []byte("# SecretSymbol\n"), 0o644))

	h.notify("workspace/didRenameFiles", renameFilesParams{
		Files: []fileRename{{OldURI: rootURI + "/a.md", NewURI: pathToFileURI(t, outside)}},
	})

	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "SecretSymbol"})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	assert.Empty(t, hits, "out-of-workspace rename target must not be read into the index")
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

// TestWillRenameFilesBatchDropsConflictingEdits locks that a batch
// moving two files which link to each other never returns two edits
// over the same range: each per-file refactor.Move plans against the
// pre-batch snapshot, so a.md's link to b.md gets one rewrite from
// a.md's own move and a different one from b.md's move. Clients reject
// a WorkspaceEdit with overlapping ranges, which would drop every
// rewrite in the batch, so the conflicting pair is withheld instead.
func TestWillRenameFilesBatchDropsConflictingEdits(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n\n[b](b.md)\n"
	srcB := "# Beta\n"
	srcC := "# Gamma\n\n[a](a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "c.md": srcC,
	})
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{
			{OldURI: rootURI + "/a.md", NewURI: rootURI + "/x/a.md"},
			{OldURI: rootURI + "/b.md", NewURI: rootURI + "/x/b.md"},
		},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	for uri, edits := range edit.Changes {
		for i := range edits {
			for j := i + 1; j < len(edits); j++ {
				assert.False(t, rangesOverlap(edits[i].Range, edits[j].Range),
					"%s: edits %d and %d overlap", uri, i, j)
			}
		}
	}
	// a.md's link to b.md is left alone (both still land in x/), and
	// c.md's unconflicted incoming link still follows a.md.
	assert.NotContains(t, edit.Changes, rootURI+"/a.md")
	require.Contains(t, edit.Changes, rootURI+"/c.md")
	assert.Equal(t, "x/a.md", edit.Changes[rootURI+"/c.md"][0].NewText)
}

// TestWillRenameFilesBatchSpellsLinkBetweenMovedFiles locks that a
// link between two files moved together gets the one edit that names
// the target's new path from the holder's new folder. Planned one move
// at a time, docs/a.md moving to the root and b.md moving into docs/
// each spelled `../b.md` as `b.md`, which names a vacated path.
func TestWillRenameFilesBatchSpellsLinkBetweenMovedFiles(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/a.md": "# Alpha\n\n[b](../b.md)\n",
		"b.md":      "# Beta\n",
		"c.md":      "# Gamma\n\n[a](docs/a.md)\n",
	})
	edit := willRename(t, h, rootURI, "docs/a.md", "a.md", "b.md", "docs/b.md")
	require.Len(t, edit.Changes[rootURI+"/docs/a.md"], 1)
	assert.Equal(t, "docs/b.md", edit.Changes[rootURI+"/docs/a.md"][0].NewText)
	require.Len(t, edit.Changes[rootURI+"/c.md"], 1)
	assert.Equal(t, "a.md", edit.Changes[rootURI+"/c.md"][0].NewText)
}

// TestWillRenameFilesBatchRewritesLinkOnlyTargetMoveTouches locks the
// link only the target's move used to rewrite: docs/a.md's
// `../docs/b.md` still resolves from other/, but docs/b.md moves to
// docs/sub/, so the right text is `../docs/sub/b.md`.
func TestWillRenameFilesBatchRewritesLinkOnlyTargetMoveTouches(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/a.md": "# Alpha\n\n[b](../docs/b.md)\n",
		"docs/b.md": "# Beta\n",
		"c.md":      "# Gamma\n\n[b](docs/b.md)\n",
	})
	edit := willRename(t, h, rootURI, "docs/a.md", "other/a.md", "docs/b.md", "docs/sub/b.md")
	require.Len(t, edit.Changes[rootURI+"/docs/a.md"], 1)
	assert.Equal(t, "../docs/sub/b.md", edit.Changes[rootURI+"/docs/a.md"][0].NewText)
	require.Len(t, edit.Changes[rootURI+"/c.md"], 1)
	assert.Equal(t, "docs/sub/b.md", edit.Changes[rootURI+"/c.md"][0].NewText)
}

// TestWillRenameFilesBatchKeepsRightOneSidedRewrite locks a rewrite
// only the target's move plans that is right from the holder's new
// folder too: `../b.md` becomes `../b2.md` from other/.
func TestWillRenameFilesBatchKeepsRightOneSidedRewrite(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/a.md": "# Alpha\n\n[b](../b.md)\n",
		"b.md":      "# Beta\n",
	})
	edit := willRename(t, h, rootURI, "docs/a.md", "other/a.md", "b.md", "b2.md")
	require.Len(t, edit.Changes[rootURI+"/docs/a.md"], 1)
	assert.Equal(t, "../b2.md", edit.Changes[rootURI+"/docs/a.md"][0].NewText)
}

// TestWillRenameFilesBatchDifferentFolders locks that moving a.md to
// x/ and b.md to y/ together rewrites a.md's link to `../y/b.md`.
func TestWillRenameFilesBatchDifferentFolders(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "# Alpha\n\n[b](b.md)\n",
		"b.md": "# Beta\n",
	})
	edit := willRename(t, h, rootURI, "a.md", "x/a.md", "b.md", "y/b.md")
	require.Len(t, edit.Changes[rootURI+"/a.md"], 1)
	assert.Equal(t, "../y/b.md", edit.Changes[rootURI+"/a.md"][0].NewText)
}

// willRename sends one workspace/willRenameFiles request moving each
// (old, new) pair of workspace-relative paths and returns the decoded
// reply.
func willRename(t *testing.T, h *testHarness, rootURI string, pairs ...string) workspaceEdit {
	t.Helper()
	var files []fileRename
	for i := 0; i+1 < len(pairs); i += 2 {
		files = append(files, fileRename{OldURI: rootURI + "/" + pairs[i], NewURI: rootURI + "/" + pairs[i+1]})
	}
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{Files: files})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	return edit
}

// TestWillRenameFilesBatchWithholdsEditInsideUnplannedMove locks that
// a batch member whose own move cannot be planned still counts as a
// moved file. refactor.Move refuses docs/a.md because x/y/a.md exists
// (the editor still moves it, overwriting), yet moving docs/b.md to
// x/y/b.md would spell docs/a.md's `b.md` as `../x/y/b.md` from docs/,
// which names x/x/y/b.md once a.md sits in x/y/ — where `b.md`, left
// alone, is already right.
func TestWillRenameFilesBatchWithholdsEditInsideUnplannedMove(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/a.md": "# Alpha\n\n[b](b.md)\n",
		"docs/b.md": "# Beta\n",
		"x/y/a.md":  "# Old\n",
	})
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{
			{OldURI: rootURI + "/docs/a.md", NewURI: rootURI + "/x/y/a.md"},
			{OldURI: rootURI + "/docs/b.md", NewURI: rootURI + "/x/y/b.md"},
		},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	assert.NotContains(t, edit.Changes, rootURI+"/docs/a.md")
}

// TestWillRenameFilesBatchKeepsCrossEditInSameDirectoryRename locks
// that a file renamed within its directory still receives another
// move's rewrite: the edit is spelled from that directory, which the
// rename does not change, so it stays correct.
func TestWillRenameFilesBatchKeepsCrossEditInSameDirectoryRename(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/a.md": "# Alpha\n\n[b](b.md)\n",
		"docs/b.md": "# Beta\n",
	})
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{
			{OldURI: rootURI + "/docs/a.md", NewURI: rootURI + "/docs/a2.md"},
			{OldURI: rootURI + "/docs/b.md", NewURI: rootURI + "/docs/sub/b.md"},
		},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Len(t, edit.Changes[rootURI+"/docs/a.md"], 1)
	assert.Equal(t, "sub/b.md", edit.Changes[rootURI+"/docs/a.md"][0].NewText)
}

// TestWillRenameFilesBatchKeepsStemRewriteInMovedFile locks that a
// `[[stem]]` rewrite another move plans inside a file the batch moves
// to a new directory is kept: it names the target by stem, not by a
// path from the holder's directory, so the holder's move cannot make
// it wrong. Withholding it would leave `[[b]]` naming a stem no file
// carries once docs/b.md becomes docs/c.md.
func TestWillRenameFilesBatchKeepsStemRewriteInMovedFile(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/a.md": "# Alpha\n\nSee [[b]].\n",
		"docs/b.md": "# Beta\n",
	})
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{
			{OldURI: rootURI + "/docs/a.md", NewURI: rootURI + "/other/a.md"},
			{OldURI: rootURI + "/docs/b.md", NewURI: rootURI + "/docs/c.md"},
		},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Len(t, edit.Changes[rootURI+"/docs/a.md"], 1)
	assert.Equal(t, "c", edit.Changes[rootURI+"/docs/a.md"][0].NewText)
}

// TestWillRenameFilesBatchLogsWithheldEdits locks that a link left
// stale is not silent: docs/b.md moves onto the existing x/b.md, which
// no plan covers, so docs/a.md's `b.md` gets no edit and the server
// sends a window/logMessage warning counting it.
func TestWillRenameFilesBatchLogsWithheldEdits(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/a.md": "# Alpha\n\n[b](b.md)\n",
		"docs/b.md": "# Beta\n",
		"x/b.md":    "# Old\n",
	})
	edit := willRename(t, h, rootURI, "docs/a.md", "other/a.md", "docs/b.md", "x/b.md")
	assert.NotContains(t, edit.Changes, rootURI+"/docs/a.md")
	for {
		var p logMessageParams
		require.NoError(t, json.Unmarshal(h.awaitNotification("window/logMessage", 5*time.Second), &p))
		if strings.Contains(p.Message, "withheld") {
			assert.Equal(t, messageTypeWarning, p.Type)
			assert.Contains(t, p.Message, "1 link rewrite")
			return
		}
	}
}

// TestWillRenameFilesRepeatedPairPlannedOnce locks that a rename pair
// listed twice is planned once, so its edits do not collide with their
// own copies and get withheld.
func TestWillRenameFilesRepeatedPairPlannedOnce(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "# Alpha\n",
		"c.md": "# Gamma\n\n[a](a.md)\n",
	})
	pair := fileRename{OldURI: rootURI + "/a.md", NewURI: rootURI + "/x/a.md"}
	raw, errResp := h.request("workspace/willRenameFiles", renameFilesParams{
		Files: []fileRename{pair, pair},
	})
	require.Nil(t, errResp)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(raw, &edit))
	require.Len(t, edit.Changes[rootURI+"/c.md"], 1)
	assert.Equal(t, "x/a.md", edit.Changes[rootURI+"/c.md"][0].NewText)
}
