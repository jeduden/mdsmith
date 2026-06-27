package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeWordlistFile creates .mdsmith/wordlists/<name>.yaml under dir.
func writeWordlistFile(t *testing.T, dir, name, body string) {
	t.Helper()
	wlDir := filepath.Join(dir, ".mdsmith", "wordlists")
	require.NoError(t, os.MkdirAll(wlDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(wlDir, name+".yaml"), []byte(body), 0o600))
}

func TestMergeWordlistFiles_LoadsAndResolves(t *testing.T) {
	dir := t.TempDir()
	writeWordlistFile(t, dir, "base", "entries:\n  - delve\n")
	writeWordlistFile(t, dir, "team", "extends: base\nentries:\n  - synergy\n")

	cfg := &Config{}
	require.NoError(t, mergeWordlistFiles(cfg, filepath.Join(dir, ".mdsmith.yml")))

	uw, ok := cfg.Wordlists["team"]
	require.True(t, ok, "team list discovered")
	assert.Equal(t, "base", uw.Extends)
	assert.Equal(t, []string{"synergy"}, uw.Entries)

	// The discovered list is reachable by the resolver.
	require.NotNil(t, toWordlistMap(cfg.Wordlists)["team"])
}

func TestMergeWordlistFiles_InlineFileCollision(t *testing.T) {
	dir := t.TempDir()
	writeWordlistFile(t, dir, "team", "entries:\n  - x\n")

	cfg := &Config{Wordlists: map[string]UserWordlist{"team": {Entries: []string{"y"}}}}
	err := mergeWordlistFiles(cfg, filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both")
}

func TestMergeWordlistFiles_NoDirectoryIsFine(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}
	require.NoError(t, mergeWordlistFiles(cfg, filepath.Join(dir, ".mdsmith.yml")))
	assert.Empty(t, cfg.Wordlists)
}

func TestParseWordlistFile_RejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	writeWordlistFile(t, dir, "bad", "bogus: 1\nentries:\n  - x\n")
	_, err := parseWordlistFile(filepath.Join(dir, ".mdsmith", "wordlists", "bad.yaml"))
	require.Error(t, err)
}
