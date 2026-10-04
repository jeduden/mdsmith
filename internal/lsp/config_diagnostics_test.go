package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publishedFor decodes every LSP frame in out and returns the params of
// each textDocument/publishDiagnostics notification for uri, in order.
func publishedFor(t *testing.T, out, uri string) []publishDiagnosticsParams {
	t.Helper()
	r := bufio.NewReader(strings.NewReader(out))
	var got []publishDiagnosticsParams
	for {
		hdr, err := textproto.NewReader(r).ReadMIMEHeader()
		if err != nil {
			return got
		}
		n, err := strconv.Atoi(hdr.Get("Content-Length"))
		require.NoError(t, err)
		body := make([]byte, n)
		_, err = io.ReadFull(r, body)
		require.NoError(t, err)
		var msg struct {
			Method string                   `json:"method"`
			Params publishDiagnosticsParams `json:"params"`
		}
		require.NoError(t, json.Unmarshal(body, &msg))
		if msg.Method == "textDocument/publishDiagnostics" && msg.Params.URI == uri {
			got = append(got, msg.Params)
		}
	}
}

func TestReloadConfigPublishesPositionedConfigDiagnostic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, writeFile(cfgPath, "rules: {}\nforeign-regions:\n  - start: \"<!-- a -->\"\n    end: \"\"\n"))

	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()

	uri := pathToURI(cfgPath)
	pubs := publishedFor(t, buf.String(), uri)
	require.Len(t, pubs, 1)
	require.Len(t, pubs[0].Diagnostics, 1)
	d := pubs[0].Diagnostics[0]
	assert.Equal(t, 3, d.Range.Start.Line, "0-based line of the `end:` key")
	assert.Equal(t, 4, d.Range.Start.Character)
	assert.Equal(t, "config", d.Code)
	assert.Equal(t, "foreign-regions[0]: end marker must not be empty", d.Message)
	// The logMessage summary is kept for when the file is not open.
	assert.Contains(t, buf.String(), `"window/logMessage"`)

	// Fixing the file and reloading clears the squiggle.
	require.NoError(t, writeFile(cfgPath, "rules: {}\n"))
	s.reloadConfig()
	pubs = publishedFor(t, buf.String(), uri)
	require.Len(t, pubs, 2)
	assert.Empty(t, pubs[1].Diagnostics)
	assert.NotNil(t, pubs[1].Diagnostics, "clear must send [] not null")

	// A further clean reload publishes nothing more for the config.
	s.reloadConfig()
	assert.Len(t, publishedFor(t, buf.String(), uri), 2)
}

func TestReloadConfigDiagnosticUsesOpenBuffer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	// The open buffer's line 3 is longer than the one on disk; the
	// squiggle's end column is measured against the buffer.
	require.NoError(t, writeFile(cfgPath, "kinds:\n  plan:\n    extends: ghost\n"))

	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	uri := pathToURI(cfgPath)
	s.docs.set(uri, &document{uri: uri, path: cfgPath, text: []byte("kinds:\n  plan:\n    extends: ghost # unsaved\n")})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()

	pubs := publishedFor(t, buf.String(), uri)
	require.Len(t, pubs, 1)
	require.Len(t, pubs[0].Diagnostics, 1)
	r := pubs[0].Diagnostics[0].Range
	assert.Equal(t, 2, r.Start.Line)
	assert.Equal(t, 4, r.Start.Character)
	assert.Equal(t, len("    extends: ghost # unsaved"), r.End.Character)
}

func TestReloadConfigUnpositionedErrorPublishesNothing(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.settings.ConfigPath = filepath.Join(t.TempDir(), "missing.yml")
	s.reloadConfig()
	assert.NotContains(t, buf.String(), "textDocument/publishDiagnostics")
	assert.Contains(t, buf.String(), `"window/logMessage"`)
}

func TestRegisterWatchersIncludesPyproject(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.registerWatchers()
	assert.Contains(t, buf.String(), "**/pyproject.toml")
}

func TestWatchedPyprojectChangeReloadsConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	py := filepath.Join(dir, "pyproject.toml")
	require.NoError(t, writeFile(py, "[tool.mdsmith.rules]\nline-length = false\n"))

	s := New(Options{Reader: nil, Writer: &safeBuffer{}})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()

	raw, err := json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{
		{URI: pathToURI(py), Type: fileChangeCreated},
	}})
	require.NoError(t, err)
	s.handleDidChangeWatchedFiles(context.Background(), raw)

	cfg, path, _ := s.snapshotConfig()
	assert.Equal(t, py, path)
	assert.False(t, cfg.Rules["line-length"].Enabled)
	assert.False(t, watchedFilesTreeChanged([]fileEvent{{URI: pathToURI(py), Type: fileChangeCreated}}),
		"a config-only create must not flag a wikilink tree change")
}

func TestReloadConfigLogsPluralTableHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(filepath.Join(dir, "pyproject.toml"), "[tools.mdsmith]\nfiles = []\n"))
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()
	assert.Contains(t, buf.String(), "[tools.mdsmith] is not read; rename the table to [tool.mdsmith]")
	assert.Contains(t, buf.String(), `"type":2`)
}
