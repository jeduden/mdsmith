package lint

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWrapOpenRootFS locks that OpenRootFS opens through the wrapped
// opener, which receives the opener it replaced, and that restore puts
// that opener back. Not parallel: it swaps the package opener.
func TestWrapOpenRootFS(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	restore := WrapOpenRootFS(func(current func(string) RootFS) func(string) RootFS {
		return func(d string) RootFS {
			calls = append(calls, d)
			return current(d)
		}
	})
	r := OpenRootFS(dir)
	_, err := fs.Stat(r, ".")
	require.NoError(t, err, "the wrapper opens through the replaced opener")
	require.NoError(t, r.Close())
	assert.Equal(t, []string{dir}, calls)

	restore()
	r = OpenRootFS(dir)
	require.NoError(t, r.Close())
	assert.Len(t, calls, 1, "restore puts the replaced opener back")
}
