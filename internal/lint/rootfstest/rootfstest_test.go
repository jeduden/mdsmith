package rootfstest

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

// TestRecord locks that Record opens through the seam it replaced,
// records each handle in order, and restores the seam at cleanup.
func TestRecord(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))
	seam := lint.OpenRootFS
	t.Run("records", func(t *testing.T) {
		opened := Record(t, &seam)
		r := seam(dir)
		require.Len(t, *opened, 1)
		assert.Same(t, r, (*opened)[0])
		_, err := fs.Stat(r, "a.md")
		assert.NoError(t, err, "the handle opens through the previous seam")
		assert.NoError(t, r.Close())
	})
	r := seam(dir)
	defer func() { _ = r.Close() }()
	_, err := fs.Stat(r, "a.md")
	assert.NoError(t, err, "the restored seam still opens")
}
