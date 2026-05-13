package main_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLSPRenameE2E spawns the shared mdsmith binary and drives a
// three-file rename round-trip
// (initialize → didOpen → prepareRename → rename) over stdio.
// This is the headline acceptance test for plan 151.
func TestLSPRenameE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LSP rename subprocess test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tmp, sources := writeRenameCorpus(t)
	pipe := startLSPSubprocess(t, ctx, binaryPath)
	rootURI := pathToFileURIE2E(t, tmp)
	uris := map[string]string{
		"a.md": rootURI + "/a.md",
		"b.md": rootURI + "/b.md",
		"c.md": rootURI + "/c.md",
	}

	initRenameE2E(t, pipe, rootURI)
	for name, uri := range uris {
		pipe.openDocument(uri, sources[name])
		_ = pipe.awaitDiagnostics(t, uri, time.Now().Add(15*time.Second))
	}

	assertPrepareRenameHeading(t, pipe, uris["a.md"])
	assertRenameRewritesAcrossFiles(t, pipe, uris)

	pipe.shutdown(t)
}

// writeRenameCorpus writes a three-file workspace shared between
// the prepareRename + rename assertions.
func writeRenameCorpus(t *testing.T) (string, map[string]string) {
	t.Helper()
	tmp := t.TempDir()
	sources := map[string]string{
		"a.md": "# Alpha\n\n## Setup\n\nbody\n",
		"b.md": "# Beta\n\n[s](./a.md#setup)\n",
		"c.md": "# Gamma\n\n[other](./a.md#setup)\n",
	}
	for name, body := range sources {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte(body), 0o644))
	}
	return tmp, sources
}

func initRenameE2E(t *testing.T, pipe *lspPipe, rootURI string) {
	t.Helper()
	resp := pipe.request("initialize", 1, map[string]any{
		"rootUri":      rootURI,
		"capabilities": fullClientCapabilities(),
	})
	require.Equal(t, float64(1), resp["id"])
	res, ok := resp["result"].(map[string]any)
	require.True(t, ok)
	caps, ok := res["capabilities"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, caps, "renameProvider")
	rp, ok := caps["renameProvider"].(map[string]any)
	require.True(t, ok, "expected renameProvider object, got %T", caps["renameProvider"])
	assert.Equal(t, true, rp["prepareProvider"])
	pipe.notify("initialized", map[string]any{})
}

func assertPrepareRenameHeading(t *testing.T, pipe *lspPipe, uriA string) {
	t.Helper()
	prep := pipe.requestPickResult(t, "textDocument/prepareRename", 100, map[string]any{
		"textDocument": map[string]any{"uri": uriA},
		"position":     map[string]any{"line": 2, "character": 4},
	})
	prepObj, ok := prep.(map[string]any)
	require.True(t, ok, "expected object result, got %T", prep)
	assert.Equal(t, "Setup", prepObj["placeholder"])
}

func assertRenameRewritesAcrossFiles(t *testing.T, pipe *lspPipe, uris map[string]string) {
	t.Helper()
	renameRaw := pipe.requestPickResult(t, "textDocument/rename", 101, map[string]any{
		"textDocument": map[string]any{"uri": uris["a.md"]},
		"position":     map[string]any{"line": 2, "character": 4},
		"newName":      "Configuration",
	})
	renameObj, ok := renameRaw.(map[string]any)
	require.True(t, ok, "expected object result, got %T", renameRaw)
	changes, ok := renameObj["changes"].(map[string]any)
	require.True(t, ok, "expected changes map, got %T", renameObj["changes"])
	for _, want := range uris {
		require.Contains(t, changes, want, "WorkspaceEdit missing %s", want)
	}
	var bEdits []map[string]any
	bRaw, _ := json.Marshal(changes[uris["b.md"]])
	require.NoError(t, json.Unmarshal(bRaw, &bEdits))
	require.Len(t, bEdits, 1)
	assert.Equal(t, "configuration", bEdits[0]["newText"])
}
