package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/wordlist"
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

	// The discovered list resolves through its extends chain (parent
	// entries first, then its own).
	resolved, err := wordlist.Resolve("team", toWordlistMap(cfg.Wordlists))
	require.NoError(t, err)
	assert.Equal(t, []string{"delve", "synergy"}, resolved)
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

func TestToWordlistMap_Nil(t *testing.T) {
	assert.Nil(t, toWordlistMap(nil))
	assert.Nil(t, toWordlistMap(map[string]UserWordlist{}))
}

func TestToWordlistMap_Populated(t *testing.T) {
	m := map[string]UserWordlist{
		"team": {Extends: "base", Entries: []string{"delve"}, SourcePath: "/cfg"},
	}
	got := toWordlistMap(m)
	require.Len(t, got, 1)
	wl := got["team"]
	assert.Equal(t, "team", wl.Name)
	assert.Equal(t, "base", wl.Extends)
	assert.Equal(t, []string{"delve"}, wl.Entries)
	assert.Equal(t, "/cfg", wl.SourcePath)
}

func TestStripLists_RemovesKey(t *testing.T) {
	in := map[string]any{"lists": []any{"x"}, "contains": []any{"y"}}
	out := stripLists(in)
	_, hasLists := out["lists"]
	assert.False(t, hasLists, "lists key must be absent after strip")
	assert.Equal(t, []any{"y"}, out["contains"])
}

func TestStripLists_NoListsKey(t *testing.T) {
	in := map[string]any{"contains": []any{"a"}}
	out := stripLists(in)
	assert.Equal(t, map[string]any{"contains": []any{"a"}}, out)
}

func TestResolveListEntries_Empty(t *testing.T) {
	assert.Nil(t, resolveListEntries(nil, nil))
}

func TestResolveListEntries_SkipsUnresolvable(t *testing.T) {
	got := resolveListEntries([]string{"ghost"}, nil)
	assert.Nil(t, got)
}

func TestResolveListEntries_ResolvesEntry(t *testing.T) {
	userMap := map[string]wordlist.Wordlist{
		"base": {Name: "base", Entries: []string{"delve", "robust"}},
	}
	got := resolveListEntries([]string{"base"}, userMap)
	assert.Equal(t, []string{"delve", "robust"}, got)
}

func TestExpandRuleLists_NonListValueIsNoop(t *testing.T) {
	settings := map[string]any{"contains": []any{"existing"}}
	expandRuleLists("not-a-list", "contains", settings, nil)
	assert.Equal(t, []any{"existing"}, settings["contains"])
}

func TestExpandRuleLists_UnionsResolvedFirst(t *testing.T) {
	userMap := map[string]wordlist.Wordlist{
		"base": {Name: "base", Entries: []string{"delve"}},
	}
	settings := map[string]any{"contains": []any{"robust"}}
	expandRuleLists([]any{"base"}, "contains", settings, userMap)
	assert.Equal(t, []any{"delve", "robust"}, settings["contains"])
}

func TestExpandRuleLists_DeduplicatesEntries(t *testing.T) {
	userMap := map[string]wordlist.Wordlist{
		"base": {Name: "base", Entries: []string{"delve", "robust"}},
	}
	settings := map[string]any{"contains": []any{"delve"}}
	expandRuleLists([]any{"base"}, "contains", settings, userMap)
	assert.Equal(t, []any{"delve", "robust"}, settings["contains"])
}
