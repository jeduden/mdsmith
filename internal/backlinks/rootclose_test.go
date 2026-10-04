package backlinks

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
)

// TestExtractBacklinksClosesRoot locks that the os.Root a source file's
// wikilink resolution opens is closed once that file's records are
// built, so `mdsmith list backlinks` holds no handle per source file.
// Not parallel: it swaps the openRootFS seam.
func TestExtractBacklinksClosesRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a.md")
	require.NoError(t, os.WriteFile(src, []byte("See [[b]].\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.md"), []byte("# B\n"), 0o644))
	opened := rootfstest.Record(t, &openRootFS)

	got, err := extractBacklinksFromSource(src, "a.md", root, "b.md", "", 0, true, nil)
	require.NoError(t, err)
	require.Len(t, got, 1, "wikilink resolution read through the root")
	require.Len(t, *opened, 1)
	_, statErr := fs.Stat((*opened)[0], "b.md")
	assert.Error(t, statErr, "the source file's root is closed once its records are built")
}

// TestExtractBacklinksWithIndexOpensNoRoot locks that a source file
// resolved through Collect's prebuilt wikilink index opens no os.Root:
// only the index-less fallback walks the root, so N source files cost no
// N open and close calls. Not parallel: it swaps the openRootFS seam.
func TestExtractBacklinksWithIndexOpensNoRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a.md")
	require.NoError(t, os.WriteFile(src, []byte("See [[b]].\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.md"), []byte("# B\n"), 0o644))
	idx := linkgraph.WikilinkIndexAtDir(root)
	require.NotNil(t, idx)
	opened := rootfstest.Record(t, &openRootFS)

	got, err := extractBacklinksFromSource(src, "a.md", root, "b.md", "", 0, true, idx)
	require.NoError(t, err)
	require.Len(t, got, 1, "wikilink resolution read through the index")
	assert.Empty(t, *opened)
}
