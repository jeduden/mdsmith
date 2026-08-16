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

// TestWordlistFileContract_MDS074StopwordsViaListsFile locks MDS074's
// WordlistTarget() path: words declared in a .mdsmith/wordlists/ file
// and referenced via lists: are excluded from repetition counting.
func TestWordlistFileContract_MDS074StopwordsViaListsFile(t *testing.T) {
	// "process" repeated 5 times in section scope with max=3 would normally
	// flag, but the wordlist exempts it.
	cfg := "rules:\n  over-repetition:\n    scope: section\n    max: 3\n    lists: [domain]\n"
	cfgPath := wordlistFileContractFixture(t, cfg, map[string]string{
		"domain.yaml": "entries:\n  - process\n",
	})
	dir := filepath.Dir(cfgPath)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "doc.md"),
		[]byte("# Introduction\n\nThe process handles requests. Each process step validates input. "+
			"The process logs results after each process step, then the process ends.\n"),
		0o644))

	diags := runCheckOnDoc(t, dir)

	assert.False(t, slices.ContainsFunc(diags, func(d diagKey) bool {
		return d.rule == "MDS074"
	}), "expected no MDS074 diagnostic: stopworded word should not be flagged; got %v", diags)
}
