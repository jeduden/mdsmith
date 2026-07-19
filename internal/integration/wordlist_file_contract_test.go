package integration

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Contract tests for the `.mdsmith/wordlists/` directory surface.
// Each case locks one invariant the public surface promises so the
// contract survives refactor pressure.

// wordlistFileContractFixture stages a workspace with a config file
// and optional wordlist files under `.mdsmith/wordlists/`. Returns
// the workspace root directory path.
func wordlistFileContractFixture(
	t *testing.T, configBody string, wordlistFiles map[string]string,
) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(configBody), 0o644))
	if len(wordlistFiles) > 0 {
		require.NoError(t, os.MkdirAll(
			filepath.Join(dir, ".mdsmith", "wordlists"), 0o755))
		for name, body := range wordlistFiles {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "wordlists", name),
				[]byte(body), 0o644))
		}
	}
	return dir
}

// TestWordlistFileContract_ExtendsChainResolvesAndDiagnosticFires locks
// plan 2606251522's acceptance criterion #1: a wordlist that extends:
// another resolves the chain, and all three entry sources (inherited,
// direct child, and inline contains) fire MDS056 diagnostics.
func TestWordlistFileContract_ExtendsChainResolvesAndDiagnosticFires(t *testing.T) {
	dir := wordlistFileContractFixture(t,
		"rules:\n  forbidden-text:\n    contains: [no-magic]\n    lists: [team]\n",
		map[string]string{
			"base.yaml": "entries:\n  - synergy\n",
			"team.yaml": "extends: base\nentries:\n  - circle back\n",
		})
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "doc.md"),
		[]byte("# Title\n\nWe need synergy. Let us circle back. Avoid no-magic.\n"), 0o644))

	diags := runCheckOnDoc(t, dir)

	// synergy=inherited via team->base, "circle back"=team's own entries, no-magic=inline merge.
	for _, word := range []string{"synergy", "circle back", "no-magic"} {
		require.True(t, slices.ContainsFunc(diags, func(d diagKey) bool {
			return d.rule == "MDS056" && strings.Contains(d.message, word)
		}), "expected MDS056 diagnostic mentioning %q; got %v", word, diags)
	}
}
