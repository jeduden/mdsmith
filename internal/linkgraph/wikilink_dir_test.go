package linkgraph

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
	"github.com/jeduden/mdsmith/internal/runcache"
)

// TestWikilinkIndexAtDir locks the on-disk walk every one-shot caller
// shares: it indexes non-Markdown files by name, skips `node_modules`,
// and builds no index for an unreadable or empty root.
func TestWikilinkIndexAtDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range []string{"docs/guide.md", "a/guide.mdx", "node_modules/p/guide.md"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("# G\n"), 0o644))
	}
	idx := WikilinkIndexAtDir(root)
	require.NotNil(t, idx)
	assert.Equal(t, []string{"docs/guide.md"}, idx.StemPaths("guide"))
	assert.Equal(t, []string{"a/guide.mdx"}, idx.NamePaths("guide.mdx"))
	assert.Nil(t, WikilinkIndexAtDir(filepath.Join(root, "missing")), "an unreadable root builds no index")
	assert.Nil(t, WikilinkIndexAtDir(""), "an empty root builds no index")
}

// TestCachedWikilinkIndexAtDir locks that the cached on-disk walk
// shares the slot MDS027 fills: keyed by the absolute dir, so an index
// already in the cache is served without walking, and a miss walks dir
// once and memoizes the result.
func TestCachedWikilinkIndexAtDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "guide.md"), []byte("# G\n"), 0o644))

	t.Run("miss walks dir once", func(t *testing.T) {
		t.Parallel()
		cache := runcache.New()
		idx := CachedWikilinkIndexAtDir(cache, root)
		require.NotNil(t, idx)
		assert.Equal(t, []string{"guide.md"}, idx.StemPaths("guide"))
		assert.Same(t, idx, CachedWikilinkIndexAtDir(cache, root))
	})
	t.Run("hit serves the MDS027 slot", func(t *testing.T) {
		t.Parallel()
		cache := runcache.New()
		warm := NewWikilinkIndexFromPaths([]string{"other.md"})
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		cache.Wikilinks(abs, func() any { return warm })
		assert.Same(t, warm, CachedWikilinkIndexAtDir(cache, root))
	})
}

// TestWikilinkIndexAtDirClosesRoot locks that the one-shot on-disk walk
// closes the os.Root it opened once the index is built. Not parallel:
// it records lint.OpenRootFS.
func TestWikilinkIndexAtDirClosesRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "guide.md"), []byte("# G\n"), 0o644))
	opened := rootfstest.Record(t)

	require.NotNil(t, WikilinkIndexAtDir(root))
	require.Len(t, opened(), 1)
	_, err := fs.Stat(opened()[0], "guide.md")
	assert.Error(t, err, "the walk's root is closed once the index is built")
}
