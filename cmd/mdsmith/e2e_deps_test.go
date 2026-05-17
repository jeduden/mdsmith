package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type depRecordE2E struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// setupDepsWorkspace builds a workspace where index.md links to api.md
// and includes frag.md, so the outgoing and incoming directions both
// have content to assert on.
func setupDepsWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mk := func(rel string) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, rel), 0o755))
	}
	wf := func(rel, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	mk("docs")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	wf(".mdsmith.yml", "files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n")
	wf("docs/api.md", "# API\n\n## Authentication\n")
	wf("docs/frag.md", "Shared fragment body.\n")
	wf("docs/index.md", "# Index\n\nSee [API](api.md#authentication).\n\n"+
		"<?include\nfile: frag.md\n?>\n<?/include?>\n")
	return dir
}

func TestE2E_Deps_Outgoing_Text(t *testing.T) {
	dir := setupDepsWorkspace(t)
	stdout, _, code := runBinaryInDir(t, dir, "", "deps", "docs/index.md")
	require.Equal(t, 0, code, "expected exit 0 when edges found; stdout=%q", stdout)
	assert.Contains(t, stdout, "docs/index.md:")
	assert.Contains(t, stdout, "file-link docs/api.md#authentication")
	assert.Contains(t, stdout, "include")
}

func TestE2E_Deps_Incoming_Text(t *testing.T) {
	dir := setupDepsWorkspace(t)
	stdout, _, code := runBinaryInDir(t, dir, "", "deps", "docs/api.md", "--incoming")
	require.Equal(t, 0, code, "stdout=%q", stdout)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.Len(t, lines, 1)
	assert.True(t, strings.HasPrefix(lines[0], "docs/index.md:"))
	assert.Contains(t, lines[0], "docs/api.md")
}

func TestE2E_Deps_JSON_Shape(t *testing.T) {
	dir := setupDepsWorkspace(t)
	stdout, _, code := runBinaryInDir(t, dir, "", "deps", "--format", "json", "docs/index.md")
	require.Equal(t, 0, code)
	var got []depRecordE2E
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.NotEmpty(t, got)
	assert.Equal(t, "docs/index.md", got[0].Source)
}

func TestE2E_Deps_NoEdges_ExitsOne(t *testing.T) {
	dir := setupDepsWorkspace(t)
	stdout, _, code := runBinaryInDir(t, dir, "", "deps", "docs/frag.md")
	require.Equal(t, 1, code)
	assert.Empty(t, strings.TrimSpace(stdout))
}

func TestE2E_Deps_NoArg_ExitsTwo(t *testing.T) {
	dir := setupDepsWorkspace(t)
	_, _, code := runBinaryInDir(t, dir, "", "deps")
	require.Equal(t, 2, code)
}

func TestE2E_Deps_TooManyArgs_ExitsTwo(t *testing.T) {
	dir := setupDepsWorkspace(t)
	_, _, code := runBinaryInDir(t, dir, "", "deps", "a.md", "b.md")
	require.Equal(t, 2, code)
}

func TestE2E_Deps_HelpFlag(t *testing.T) {
	dir := setupDepsWorkspace(t)
	_, stderr, code := runBinaryInDir(t, dir, "", "deps", "--help")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "Usage: mdsmith deps")
}

func TestE2E_Deps_AbsoluteTarget_ExitsTwo(t *testing.T) {
	dir := setupDepsWorkspace(t)
	_, _, code := runBinaryInDir(t, dir, "", "deps", "/etc/passwd")
	require.Equal(t, 2, code)
}

func TestE2E_Deps_UnknownFormat_ExitsTwo(t *testing.T) {
	dir := setupDepsWorkspace(t)
	_, _, code := runBinaryInDir(t, dir, "", "deps", "--format", "yaml", "docs/index.md")
	require.Equal(t, 2, code)
}
