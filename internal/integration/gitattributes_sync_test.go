package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/githooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findRepoRoot walks up from the test's working directory until it
// finds the directory that holds .mdsmith.yml — the repository root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".mdsmith.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir,
			"reached the filesystem root without finding .mdsmith.yml")
		dir = parent
	}
}

// TestRepoGitattributesInSyncWithConfig dogfoods this repository's own
// committed .gitattributes against its .mdsmith.yml.
//
// It guards the exact state that put the merge queue into an infinite
// loop: the queue runs `mdsmith merge-driver install`, which re-renders
// .gitattributes from .mdsmith.yml's ignore list. When the committed
// copy has drifted (an ignore pattern was added without regenerating
// .gitattributes), the re-render dirties the worktree, the action's
// `git merge` aborts with "local changes would be overwritten",
// the PR is requeued, and the labeled trigger fires again — forever.
//
// MDS048 already reports this drift in `mdsmith check`; asserting it
// here pins the invariant at the `go test` gate too, so a stale
// .gitattributes cannot reach main and bounce the queue. The failure
// message names the fix.
func TestRepoGitattributesInSyncWithConfig(t *testing.T) {
	root := findRepoRoot(t)

	expected := githooks.LoadGlobs(root)

	data, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	require.NoError(t, err, "repository .gitattributes must exist")

	installed, ok := githooks.ExtractGlobs(string(data))
	require.True(t, ok,
		"committed .gitattributes has no mdsmith merge-driver managed block")

	assert.True(t, githooks.GlobsEqual(installed, expected),
		"committed .gitattributes is out of sync with .mdsmith.yml — run "+
			"`mdsmith merge-driver install` (or `mdsmith fix`) and commit the "+
			"result.\n  committed exclude: %v\n  expected  exclude: %v",
		installed.Exclude, expected.Exclude)
}
