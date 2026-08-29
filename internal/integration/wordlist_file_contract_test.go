package integration

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contract tests for the `.mdsmith/wordlists/` directory surface.
// Each case locks one invariant the public surface promises so the
// contract survives refactor pressure.

// wordlistFileContractFixture stages a workspace with a config file
// and optional wordlist files under `.mdsmith/wordlists/`. Returns
// the `.mdsmith.yml` path (use filepath.Dir to get the workspace dir).
func wordlistFileContractFixture(
	t *testing.T, configBody string, wordlistFiles map[string]string,
) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(configBody), 0o644))
	if len(wordlistFiles) > 0 {
		require.NoError(t, os.MkdirAll(
			filepath.Join(dir, ".mdsmith", "wordlists"), 0o755))
		for name, body := range wordlistFiles {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "wordlists", name),
				[]byte(body), 0o644))
		}
	}
	return cfgPath
}

// TestWordlistFileContract_MDS060TokensViaListsFile locks MDS060's
// WordlistTarget() path: tokens declared in a .mdsmith/wordlists/ file
// and referenced via lists: are counted toward the occurrence limit.
func TestWordlistFileContract_MDS060TokensViaListsFile(t *testing.T) {
	// "synergy" appears 3 times in the section; max is 2, so MDS060 fires.
	cfg := "rules:\n  occurrence:\n    scope: section\n    count: each\n    max: 2\n    lists: [buzzwords]\n"
	cfgPath := wordlistFileContractFixture(t, cfg, map[string]string{
		"buzzwords.yaml": "entries:\n  - synergy\n",
	})
	dir := filepath.Dir(cfgPath)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "doc.md"),
		[]byte("# Introduction\n\nWe need synergy here. Synergy drives growth. Synergy is the answer.\n"),
		0o644))

	diags := runCheckOnDoc(t, dir)

	assert.True(t, slices.ContainsFunc(diags, func(d diagKey) bool {
		return d.rule == "MDS060" && strings.Contains(strings.ToLower(d.message), "synergy")
	}), "expected MDS060 diagnostic for 'synergy' sourced from wordlist; got %v", diags)
}

// TestWordlistFileContract_ExtendsChainResolvesAndDiagnosticFires locks
// plan 2606251522's acceptance criterion #1: a wordlist that extends:
// another resolves the chain, and all three entry sources (inherited,
// direct child, and inline contains) fire MDS056 diagnostics.
func TestWordlistFileContract_ExtendsChainResolvesAndDiagnosticFires(t *testing.T) {
	cfgPath := wordlistFileContractFixture(t,
		"rules:\n  forbidden-text:\n    contains: [no-magic]\n    lists: [team]\n",
		map[string]string{
			"base.yaml": "entries:\n  - synergy\n",
			"team.yaml": "extends: base\nentries:\n  - circle back\n",
		})
	dir := filepath.Dir(cfgPath)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "doc.md"),
		[]byte("# Title\n\nWe need synergy. Let us circle back. Avoid no-magic.\n"), 0o644))

	diags := runCheckOnDoc(t, dir)

	// synergy=inherited via team->base, "circle back"=team's own entries, no-magic=inline merge.
	for _, word := range []string{"synergy", "circle back", "no-magic"} {
		assert.True(t, slices.ContainsFunc(diags, func(d diagKey) bool {
			return d.rule == "MDS056" && strings.Contains(d.message, word)
		}), "expected MDS056 diagnostic mentioning %q; got %v", word, diags)
	}
}
