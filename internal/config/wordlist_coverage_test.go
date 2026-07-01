package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- copyWordlists (merge.go) ---

func TestCopyWordlists_DeepCopiesPopulated(t *testing.T) {
	in := map[string]UserWordlist{
		"team": {Extends: "base", Entries: []string{"x", "y"}, SourcePath: "/p"},
	}
	out := copyWordlists(in)
	assert.Equal(t, in, out)
	// Entries must be a distinct backing array, not shared with the input.
	out["team"].Entries[0] = "MUT"
	assert.Equal(t, "x", in["team"].Entries[0], "Entries deep-copied")
}

// --- discoverWordlists error branches (wordlist_files.go) ---

// wordlistsScratch returns a workspace root plus its
// .mdsmith/wordlists/ directory, created.
func wordlistsScratch(t *testing.T) (root, wlDir string) {
	t.Helper()
	root = t.TempDir()
	wlDir = filepath.Join(root, ".mdsmith", "wordlists")
	require.NoError(t, os.MkdirAll(wlDir, 0o755))
	return root, wlDir
}

func TestDiscoverWordlists_ReadDirError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o755))
	// .mdsmith/wordlists is a regular file, so ReadDir fails with a
	// non-NotExist error.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith", "wordlists"), []byte("x"), 0o644))
	_, err := discoverWordlists(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
}

func TestDiscoverWordlists_RejectsSymlink(t *testing.T) {
	root, wlDir := wordlistsScratch(t)
	require.NoError(t, os.Symlink("target", filepath.Join(wlDir, "link.yaml")))
	_, err := discoverWordlists(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestDiscoverWordlists_RejectsSubdir(t *testing.T) {
	root, wlDir := wordlistsScratch(t)
	require.NoError(t, os.Mkdir(filepath.Join(wlDir, "sub"), 0o755))
	_, err := discoverWordlists(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subdirectories")
}

func TestDiscoverWordlists_SkipsNonYAML(t *testing.T) {
	root, wlDir := wordlistsScratch(t)
	require.NoError(t, os.WriteFile(filepath.Join(wlDir, "notes.txt"), []byte("x"), 0o644))
	got, err := discoverWordlists(root)
	require.NoError(t, err)
	assert.Empty(t, got, "non-yaml file ignored")
}

func TestDiscoverWordlists_RejectsBadBasename(t *testing.T) {
	root, wlDir := wordlistsScratch(t)
	require.NoError(t, os.WriteFile(filepath.Join(wlDir, "Bad.yaml"), []byte("entries: [a]\n"), 0o644))
	_, err := discoverWordlists(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "basename")
}

func TestDiscoverWordlists_RejectsExtCollision(t *testing.T) {
	root, wlDir := wordlistsScratch(t)
	require.NoError(t, os.WriteFile(filepath.Join(wlDir, "team.yaml"), []byte("entries: [a]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(wlDir, "team.yml"), []byte("entries: [b]\n"), 0o644))
	_, err := discoverWordlists(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declared by both")
}

func TestDiscoverWordlists_PropagatesParseError(t *testing.T) {
	root, wlDir := wordlistsScratch(t)
	require.NoError(t, os.WriteFile(filepath.Join(wlDir, "bad.yaml"), []byte("bogus: 1\nentries: [a]\n"), 0o644))
	_, err := discoverWordlists(root)
	require.Error(t, err)
}

// --- parseWordlistFile read error ---

func TestParseWordlistFile_ReadError(t *testing.T) {
	_, err := parseWordlistFile(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
}

// --- mergeWordlistFiles tags inline source path ---

func TestMergeWordlistFiles_TagsInlineSourcePath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	cfg := &Config{Wordlists: map[string]UserWordlist{"a": {Entries: []string{"x"}}}}
	require.NoError(t, mergeWordlistFiles(cfg, cfgPath))
	assert.Equal(t, cfgPath, cfg.Wordlists["a"].SourcePath)
}

// --- expandWordlists direct branches ---

func TestExpandWordlists_EmptyResultReturns(t *testing.T) {
	expandWordlists(map[string]RuleCfg{}, nil) // early return; must not panic
}

func TestExpandWordlists_EmptyListsIsNoop(t *testing.T) {
	// An empty lists: is stripped but adds no target entries.
	result := map[string]RuleCfg{
		"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{}}},
	}
	expandWordlists(result, nil)

	got := result["forbidden-text"].Settings
	_, hasLists := got["lists"]
	assert.False(t, hasLists, "lists key stripped")
	_, hasContains := got["contains"]
	assert.False(t, hasContains, "no target added for empty lists")
}

func TestExpandWordlists_LeavesInvalidTargetUntouched(t *testing.T) {
	// A non-list target value (a user type error) must not be silently
	// replaced by the expanded entries; leave it so ApplySettings can
	// surface the error. lists: is still stripped.
	result := map[string]RuleCfg{
		"forbidden-text": {Enabled: true, Settings: map[string]any{
			"lists":    []any{"team"},
			"contains": "notalist",
		}},
	}
	users := map[string]UserWordlist{"team": {Entries: []string{"delve"}}}
	expandWordlists(result, users)

	got := result["forbidden-text"].Settings
	_, hasLists := got["lists"]
	assert.False(t, hasLists, "lists key stripped")
	assert.Equal(t, "notalist", got["contains"], "invalid target left untouched")
}

func TestExpandWordlists_SkipsUnresolvableList(t *testing.T) {
	// A list that fails to resolve is skipped during expansion (the real
	// error is reported by validateWordlists); the lists: key is still
	// stripped before a rule sees it.
	result := map[string]RuleCfg{
		"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"ghost"}}},
	}
	expandWordlists(result, nil)
	_, hasLists := result["forbidden-text"].Settings["lists"]
	assert.False(t, hasLists, "lists key stripped")
}

// --- validateWordlists branches ---

func TestValidateWordlists_NilConfig(t *testing.T) {
	require.NoError(t, validateWordlists(nil))
}

func TestValidateWordlists_KindError(t *testing.T) {
	cfg := &Config{Kinds: map[string]KindBody{
		"k": {Rules: map[string]RuleCfg{
			"forbidden-text": {Settings: map[string]any{"lists": []any{"ghost"}}},
		}},
	}}
	err := validateWordlists(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestValidateWordlists_ConventionPresetError(t *testing.T) {
	// cfg.Rules is empty, so validation reaches the convention-preset
	// check, where the bad list lives.
	cfg := &Config{
		ConventionPreset: map[string]RuleCfg{
			"forbidden-text": {Settings: map[string]any{"lists": []any{"ghost"}}},
		},
	}
	err := validateWordlists(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestValidateWordlists_OverrideError(t *testing.T) {
	cfg := &Config{Overrides: []Override{
		{Rules: map[string]RuleCfg{
			"forbidden-text": {Settings: map[string]any{"lists": []any{"ghost"}}},
		}},
	}}
	err := validateWordlists(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

// --- checkRuleLists branches ---

func TestCheckRuleLists_NonStringListsValue(t *testing.T) {
	rules := map[string]RuleCfg{"forbidden-text": {Settings: map[string]any{"lists": "nope"}}}
	err := checkRuleLists("rules", rules, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list of strings")
}

func TestCheckRuleLists_UnknownRule(t *testing.T) {
	rules := map[string]RuleCfg{"no-such-rule": {Settings: map[string]any{"lists": []any{"x"}}}}
	err := checkRuleLists("rules", rules, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown rule")
}

// --- anyToStrings branches ---

func TestAnyToStrings_Variants(t *testing.T) {
	got, ok := anyToStrings(nil)
	assert.True(t, ok)
	assert.Nil(t, got)

	got, ok = anyToStrings([]string{"a"})
	assert.True(t, ok)
	assert.Equal(t, []string{"a"}, got)

	_, ok = anyToStrings([]any{"a", 1})
	assert.False(t, ok, "non-string element rejected")

	_, ok = anyToStrings("not-a-list")
	assert.False(t, ok, "non-list rejected")
}

// --- dedupStrings branches ---

func TestDedupStrings_EmptyAndDuplicates(t *testing.T) {
	assert.Nil(t, dedupStrings(nil))
	assert.Equal(t, []string{"a", "b"}, dedupStrings([]string{"a", "b", "a"}))
}

// --- load.go error branches ---

func TestLoadFromBytes_ValidateWordlistsError(t *testing.T) {
	_, err := loadFromBytes([]byte("rules:\n  forbidden-text:\n    lists: [ghost]\n"), "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating wordlists")
}

func TestLoad_WordlistFileError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))
	// A subdirectory under .mdsmith/wordlists makes mergeWordlistFiles fail.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "wordlists", "sub"), 0o755))
	_, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wordlist")
}
