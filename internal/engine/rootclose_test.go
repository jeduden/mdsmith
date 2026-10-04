package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
	"github.com/jeduden/mdsmith/internal/rule"
)

// TestRunClosesEachFileRoots locks that Run closes every root it opens
// for a linted file once that file's check ends: the file's own
// directory (f.FS) and, for a file below the root, the separate project
// root (f.RootFS) MDS027's wikilink walk reads. The File is not
// published anywhere, so lintFile owns both. Not parallel: it records
// lint.OpenRootFS.
func TestRunClosesEachFileRoots(t *testing.T) {
	root := t.TempDir()
	host := filepath.Join(root, "host.md")
	nested := filepath.Join(root, "sub", "b.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0o755))
	require.NoError(t, os.WriteFile(host, []byte("# Host\n"), 0o644))
	require.NoError(t, os.WriteFile(nested, []byte("# B\n"), 0o644))
	opened := rootfstest.Record(t)

	runner := &Runner{
		Config:  &config.Config{Rules: map[string]config.RuleCfg{"snap-rule": {Enabled: true}}},
		Rules:   []rule.Rule{&fileSnapRule{id: "MDS999", name: "snap-rule"}},
		RootDir: root,
	}
	require.Empty(t, runner.Run([]string{host, nested}).Errors)

	got := opened()
	require.Len(t, got, 3, "host's root, b's directory, b's project root")
	for i, r := range got {
		_, err := fs.Stat(r, ".")
		assert.Error(t, err, "root %d is closed once its file's check ends", i)
	}
}

// snapRunner returns a runner at root whose only rule records the File
// it checks.
func snapRunner(root string) (*Runner, *fileSnapRule) {
	snap := &fileSnapRule{id: "MDS999", name: "snap-rule"}
	return &Runner{
		Config:  &config.Config{Rules: map[string]config.RuleCfg{"snap-rule": {Enabled: true}}},
		Rules:   []rule.Rule{snap},
		RootDir: root,
	}, snap
}

// TestRunSourceClosesItsOwnRoot locks that a RunSource with no lent
// RootFS closes the project root it opened for the File once the call
// ends: the File is not published to a ParseCache, so the call owns it.
// Not parallel: it records lint.OpenRootFS.
func TestRunSourceClosesItsOwnRoot(t *testing.T) {
	root := t.TempDir()
	opened := rootfstest.Record(t)
	runner, snap := snapRunner(root)

	require.Empty(t, runner.RunSource("a.md", []byte("# A\n")).Errors)
	require.NotNil(t, snap.last)
	got := opened()
	require.Len(t, got, 1)
	_, err := fs.Stat(got[0], ".")
	assert.Error(t, err, "the call's own root is closed once it ends")
}

// TestRunSourceBorrowsLentRootFS locks that a runner given a RootFS by
// its owner (a Session, whose parse cache keeps the File past the call)
// opens no root of its own, hands the File the lent one, and leaves it
// open for the owner.
func TestRunSourceBorrowsLentRootFS(t *testing.T) {
	root := t.TempDir()
	opened := rootfstest.Record(t)
	lent := os.DirFS(root)
	runner, snap := snapRunner(root)
	runner.RootFS = lent
	runner.ParseCache = lint.NewParseCache()

	require.Empty(t, runner.RunSourceWithVersion("a.md", []byte("# A\n"), 1).Errors)
	require.Empty(t, runner.RunSource("b.md", []byte("# B\n")).Errors)
	assert.Empty(t, opened(), "a lent root means the runner opens none")
	require.NotNil(t, snap.last)
	assert.Equal(t, lent, snap.last.RootFS)
	assert.Equal(t, root, snap.last.RootDir)
}

// TestRunSourceWithVersionKeepsNoOwnRootInParseCache locks that a
// runner given a ParseCache but no lent RootFS does not publish a File
// holding a root it opened: nothing could close that root while the
// cache keeps the File, so the call parses uncached and closes its own
// root once it ends. Not parallel: it records lint.OpenRootFS.
func TestRunSourceWithVersionKeepsNoOwnRootInParseCache(t *testing.T) {
	root := t.TempDir()
	opened := rootfstest.Record(t)
	runner, _ := snapRunner(root)
	cache := lint.NewParseCache()
	runner.ParseCache = cache

	require.Empty(t, runner.RunSourceWithVersion("a.md", []byte("# A\n"), 1).Errors)
	_, ok := cache.Get("a.md", 1)
	assert.False(t, ok, "a File holding the call's own root is not cached")
	got := opened()
	require.Len(t, got, 1)
	_, err := fs.Stat(got[0], ".")
	assert.Error(t, err, "the call's own root is closed once it ends")
}
