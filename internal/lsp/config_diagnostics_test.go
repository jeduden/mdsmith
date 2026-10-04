package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/textproto"
	"os"
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

// mdsmith.run: off publishes nothing, so a config failure shows no
// squiggle, and switching off clears one an earlier reload left.
func TestReloadConfigDiagnosticRespectsRunOff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, writeFile(cfgPath, "kinds:\n  plan:\n    extends: ghost\n"))

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

	s.settingsMu.Lock()
	s.settings.Run = runOff
	s.settingsMu.Unlock()
	s.reloadConfig()
	pubs = publishedFor(t, buf.String(), uri)
	require.Len(t, pubs, 2)
	assert.Empty(t, pubs[1].Diagnostics, "switching off clears the config squiggle")
	assert.Contains(t, buf.String(), `"window/logMessage"`, "the log summary is still sent")
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

// TestReloadConfigLogsHintsFromDiscoverSeam pins that discovery hints
// come from the injected discoverConfig walk, not a second walk of the
// real filesystem.
func TestReloadConfigLogsHintsFromDiscoverSeam(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.discoverConfig = func(string) (string, []string) {
		return "", []string{"stub hint"}
	}
	s.configMu.Lock()
	s.rootDir = "/nonexistent/root"
	s.configMu.Unlock()
	s.reloadConfig()
	assert.Contains(t, buf.String(), "mdsmith: stub hint")
}

// TestReloadConfigLogsUnchangedHintsOnce pins that a reload whose
// hints match the previous reload's does not repeat the warning, while
// a reload after the hints clear and return logs them again.
func TestReloadConfigLogsUnchangedHintsOnce(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	hints := []string{"stub hint"}
	s.discoverConfig = func(string) (string, []string) {
		return "", hints
	}
	s.configMu.Lock()
	s.rootDir = "/nonexistent/root"
	s.configMu.Unlock()
	s.reloadConfig()
	s.reloadConfig()
	assert.Equal(t, 1, strings.Count(buf.String(), "mdsmith: stub hint"))

	hints = nil
	s.reloadConfig()
	hints = []string{"stub hint"}
	s.reloadConfig()
	assert.Equal(t, 2, strings.Count(buf.String(), "mdsmith: stub hint"))
}

// TestIsWatchedConfigChange pins which watched config-named files
// reload config: one in the workspace root or an ancestor (where
// discovery looks), or the loaded config file itself — not one nested
// below the root, which discovery never reads.
func TestIsWatchedConfigChange(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "ws", "repo")
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = root
	s.configMu.Unlock()

	assert.True(t, s.isWatchedConfigChange(filepath.Join(root, "pyproject.toml")))
	assert.True(t, s.isWatchedConfigChange(filepath.Join(root, ".mdsmith.yml")))
	assert.True(t, s.isWatchedConfigChange(filepath.Join(filepath.Dir(root), "pyproject.toml")))
	assert.False(t, s.isWatchedConfigChange(filepath.Join(root, "pkg", "pyproject.toml")))
	assert.False(t, s.isWatchedConfigChange(filepath.Join(root, "pkg", ".mdsmith.yml")))
	assert.False(t, s.isWatchedConfigChange(filepath.Join(root, "doc.md")))

	loaded := filepath.Join(root, "sub", ".mdsmith.yml")
	s.configMu.Lock()
	s.configPath = loaded
	s.configMu.Unlock()
	assert.True(t, s.isWatchedConfigChange(loaded), "the loaded config file always reloads")

	s.settings.ConfigPath = "cfg/pyproject.toml"
	assert.True(t, s.isWatchedConfigChange(filepath.Join(root, "cfg", "pyproject.toml")),
		"the mdsmith.config override reloads even before it loads")
	assert.False(t, s.isWatchedConfigChange(filepath.Join(root, "pyproject.toml")),
		"with an override set, discovery does not run")

	s.settings.ConfigPath = ""
	s.configMu.Lock()
	s.rootDir = ""
	s.configMu.Unlock()
	assert.True(t, s.isWatchedConfigChange(filepath.Join(root, "pkg", "pyproject.toml")),
		"with no root known, any config-named file reloads")
}

// TestIsDirOrAncestor pins the directory test behind
// isWatchedConfigChange, including the filepath.Rel failure when one
// path is relative and the other absolute.
func TestIsDirOrAncestor(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "ws", "repo")
	assert.True(t, isDirOrAncestor(root, root))
	assert.True(t, isDirOrAncestor(string(filepath.Separator), root))
	assert.False(t, isDirOrAncestor(filepath.Join(root, "pkg"), root))
	assert.False(t, isDirOrAncestor(filepath.Join(string(filepath.Separator), "ws", "repo2"), root))
	assert.False(t, isDirOrAncestor("relative", root))
}

// TestNestedPyprojectChangeSkipsReload pins that editing a
// pyproject.toml below the workspace root neither reloads config nor
// counts as config-only for the wikilink tree check.
func TestNestedPyprojectChangeSkipsReload(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := New(Options{Reader: nil, Writer: &safeBuffer{}})
	calls := 0
	s.discoverConfig = func(string) (string, []string) {
		calls++
		return "", nil
	}
	s.configMu.Lock()
	s.rootDir = root
	s.configMu.Unlock()

	nested := filepath.Join(root, "pkg", "pyproject.toml")
	raw, err := json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{
		{URI: pathToURI(nested), Type: fileChangeCreated},
	}})
	require.NoError(t, err)
	s.handleDidChangeWatchedFiles(context.Background(), raw)
	assert.Equal(t, 0, calls, "a nested pyproject.toml must not reload config")
	assert.True(t, watchedFileTreeChanged(fileEvent{URI: pathToURI(nested), Type: fileChangeCreated}),
		"a nested pyproject.toml create is an ordinary tree change")

	top := filepath.Join(root, "pyproject.toml")
	raw, err = json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{
		{URI: pathToURI(top), Type: fileChangeChanged},
	}})
	require.NoError(t, err)
	s.handleDidChangeWatchedFiles(context.Background(), raw)
	assert.Equal(t, 1, calls, "a root pyproject.toml reloads config")
}

// TestRegisterWatchersIncludesSidecars pins that the client watches the
// kind, convention, schema and word-list files Load reads beside a
// config, so editing one reloads config.
func TestRegisterWatchersIncludesSidecars(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.registerWatchers()
	assert.Contains(t, buf.String(), "**/.mdsmith/*/*.yml")
	assert.Contains(t, buf.String(), "**/.mdsmith/*/*.yaml")
}

// TestWatchedSidecarFixClearsSquiggle pins that a positioned error in a
// kind file squiggles that file, and that saving the fix — a watched
// event on the kind file alone — reloads config and clears it.
func TestWatchedSidecarFixClearsSquiggle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(filepath.Join(dir, ".mdsmith.yml"), "rules: {}\n"))
	kf := filepath.Join(dir, ".mdsmith", "kinds", "plan.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(kf), 0o755))
	require.NoError(t, writeFile(kf, "rules: {}\npath-pattern: [x]\n"))

	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()
	uri := pathToURI(kf)
	pubs := publishedFor(t, buf.String(), uri)
	require.Len(t, pubs, 1)
	require.Len(t, pubs[0].Diagnostics, 1)

	require.NoError(t, writeFile(kf, "rules: {}\n"))
	raw, err := json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{
		{URI: uri, Type: fileChangeChanged},
	}})
	require.NoError(t, err)
	s.handleDidChangeWatchedFiles(context.Background(), raw)
	pubs = publishedFor(t, buf.String(), uri)
	require.Len(t, pubs, 2, "the sidecar save must reload config")
	assert.Empty(t, pubs[1].Diagnostics)
}

// TestIsWatchedConfigChangeSidecars pins that a sidecar file reloads
// config under the same rule as the config file beside it.
func TestIsWatchedConfigChangeSidecars(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "ws", "repo")
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = root
	s.configMu.Unlock()

	assert.True(t, s.isWatchedConfigChange(filepath.Join(root, ".mdsmith", "kinds", "plan.yml")))
	assert.True(t, s.isWatchedConfigChange(filepath.Join(filepath.Dir(root), ".mdsmith", "wordlists", "w.yaml")))
	assert.False(t, s.isWatchedConfigChange(filepath.Join(root, "pkg", ".mdsmith", "kinds", "plan.yml")),
		"a sidecar beside a nested config discovery never reads")
	assert.False(t, s.isWatchedConfigChange(filepath.Join(root, ".mdsmith", "other", "x.yml")))

	loaded := filepath.Join(root, "sub", ".mdsmith.yml")
	s.configMu.Lock()
	s.configPath = loaded
	s.configMu.Unlock()
	assert.True(t, s.isWatchedConfigChange(filepath.Join(root, "sub", ".mdsmith", "schemas", "s.yml")),
		"a sidecar beside the loaded config reloads")

	s.settings.ConfigPath = "cfg/.mdsmith.yml"
	assert.True(t, s.isWatchedConfigChange(filepath.Join(root, "cfg", ".mdsmith", "conventions", "c.yml")),
		"a sidecar beside the mdsmith.config override reloads")
	assert.False(t, s.isWatchedConfigChange(filepath.Join(root, ".mdsmith", "kinds", "plan.yml")),
		"with an override set, a sidecar beside the root config is not read")
}

// TestIsWatchedConfigChangeThroughSymlink pins that a watched event
// carrying the symlink-resolved path of a file the server knows by its
// link path (a workspace opened through a symlink) still reloads.
func TestIsWatchedConfigChangeThroughSymlink(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	real := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(real, "sub"), 0o755))
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = link
	s.configMu.Unlock()
	assert.True(t, s.isWatchedConfigChange(filepath.Join(real, ".mdsmith.yml")), "root config by its real path")
	assert.True(t, s.isWatchedConfigChange(filepath.Join(real, ".mdsmith", "kinds", "k.yml")),
		"root sidecar by its real path")
	assert.False(t, s.isWatchedConfigChange(filepath.Join(real, "sub", ".mdsmith.yml")))

	s.configMu.Lock()
	s.configPath = filepath.Join(link, "sub", ".mdsmith.yml")
	s.configMu.Unlock()
	assert.True(t, s.isWatchedConfigChange(filepath.Join(real, "sub", ".mdsmith.yml")),
		"loaded config by its real path")

	s.settings.ConfigPath = filepath.Join(link, "sub", "pyproject.toml")
	assert.True(t, s.isWatchedConfigChange(filepath.Join(real, "sub", "pyproject.toml")), "override by its real path")
}
