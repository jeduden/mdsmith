package linkgraph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
