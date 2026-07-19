package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWordlistFiles_ExtendsChainResolvesAndDiagnosticFires pins plan
// 2606251522's acceptance criterion #1: a .mdsmith/wordlists/team.yaml
// that extends another file resolves, and a doc using a word inherited
// through the extends chain fails check.
func TestWordlistFiles_ExtendsChainResolvesAndDiagnosticFires(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"),
		[]byte("rules:\n  forbidden-text:\n    lists: [team]\n"), 0o644))
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "wordlists"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "wordlists", "base.yaml"),
		[]byte("entries:\n  - synergy\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "wordlists", "team.yaml"),
		[]byte("extends: base\nentries:\n  - circle back\n"), 0o644))
	// "synergy" is in base; team extends base, so the extends chain must
	// deliver it into forbidden-text.contains for the check to fire.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "doc.md"),
		[]byte("# Title\n\nWe need more synergy to move this forward.\n"), 0o644))

	diags := runCheckOnDoc(t, dir)
	require.NotEmpty(t, diags,
		"forbidden-text must fire on \"synergy\" (inherited via team → base)")

	found := false
	for _, d := range diags {
		if d.rule == "MDS056" && strings.Contains(d.message, "synergy") {
			found = true
			break
		}
	}
	require.True(t, found,
		"expected MDS056 diagnostic mentioning \"synergy\"; got %v", diags)
}
