//go:build !tinygo

package lint_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/testsymlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRootFS_NonExistentDir(t *testing.T) {
	// When os.OpenRoot fails (dir does not exist), OpenRootFS returns an
	// openRootErrFS that propagates the error on every Open call rather
	// than panicking at construction time.
	fsys := lint.OpenRootFS(t.TempDir() + "/does-not-exist")
	_, err := fsys.Open("any.md")
	require.Error(t, err, "Open on an openRootErrFS must return the construction error")
}

// writeRootFile writes body to name under a fresh temp dir and returns
// the dir.
func writeRootFile(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	return dir
}

// TestOpenRootFS_CloseReleasesRoot locks that the returned handle closes
// its os.Root: a read before Close succeeds, and every read after it
// fails.
func TestOpenRootFS_CloseReleasesRoot(t *testing.T) {
	fsys := lint.OpenRootFS(writeRootFile(t, "a.md", "# A\n"))
	_, err := fs.ReadFile(fsys, "a.md")
	require.NoError(t, err)
	require.NoError(t, fsys.Close())
	_, err = fsys.Open("a.md")
	assert.Error(t, err, "Open after Close must fail")
	_, err = fs.ReadFile(fsys, "a.md")
	assert.Error(t, err, "ReadFile after Close must fail")
}

// TestOpenRootFS_NonExistentDirClose locks that closing the error FS a
// missing dir yields is a harmless no-op.
func TestOpenRootFS_NonExistentDirClose(t *testing.T) {
	assert.NoError(t, lint.OpenRootFS(t.TempDir()+"/does-not-exist").Close())
}

// TestCloseFS locks that CloseFS closes a closable view and leaves one
// without a Close (os.DirFS) readable.
func TestCloseFS(t *testing.T) {
	dir := writeRootFile(t, "a.md", "# A\n")
	root := lint.OpenRootFS(dir)
	lint.CloseFS(root)
	_, err := fs.Stat(root, "a.md")
	assert.Error(t, err, "CloseFS closes a RootFS")

	plain := os.DirFS(dir)
	lint.CloseFS(plain)
	_, err = fs.Stat(plain, "a.md")
	assert.NoError(t, err, "a view with no Close is left alone")
}

// The forwarding tests below lock that the closable wrapper keeps the
// optional fs interfaces os.Root's FS implements, so fs.ReadFile,
// fs.ReadDir, fs.Stat, fs.ReadLink, and fs.Lstat keep their fast paths.

func TestOpenRootFS_ReadFile(t *testing.T) {
	fsys := lint.OpenRootFS(writeRootFile(t, "a.md", "# A\n"))
	t.Cleanup(func() { require.NoError(t, fsys.Close()) })
	rf, ok := fsys.(fs.ReadFileFS)
	require.True(t, ok)
	b, err := rf.ReadFile("a.md")
	require.NoError(t, err)
	assert.Equal(t, "# A\n", string(b))
}

func TestOpenRootFS_ReadDir(t *testing.T) {
	fsys := lint.OpenRootFS(writeRootFile(t, "a.md", "# A\n"))
	t.Cleanup(func() { require.NoError(t, fsys.Close()) })
	rd, ok := fsys.(fs.ReadDirFS)
	require.True(t, ok)
	ents, err := rd.ReadDir(".")
	require.NoError(t, err)
	require.Len(t, ents, 1)
	assert.Equal(t, "a.md", ents[0].Name())
}

func TestOpenRootFS_Stat(t *testing.T) {
	fsys := lint.OpenRootFS(writeRootFile(t, "a.md", "# A\n"))
	t.Cleanup(func() { require.NoError(t, fsys.Close()) })
	st, ok := fsys.(fs.StatFS)
	require.True(t, ok)
	fi, err := st.Stat("a.md")
	require.NoError(t, err)
	assert.Equal(t, int64(4), fi.Size())
}

// symlinkRoot returns a dir holding a.md and link.md → a.md, skipping
// the test where the host cannot create symlinks.
func symlinkRoot(t *testing.T) string {
	t.Helper()
	testsymlink.SkipIfSymlinkUnsupported(t)
	dir := writeRootFile(t, "a.md", "# A\n")
	require.NoError(t, os.Symlink("a.md", filepath.Join(dir, "link.md")))
	return dir
}

func TestOpenRootFS_ReadLink(t *testing.T) {
	fsys := lint.OpenRootFS(symlinkRoot(t))
	t.Cleanup(func() { require.NoError(t, fsys.Close()) })
	rl, ok := fsys.(fs.ReadLinkFS)
	require.True(t, ok)
	target, err := rl.ReadLink("link.md")
	require.NoError(t, err)
	assert.Equal(t, "a.md", target)
}

func TestOpenRootFS_Lstat(t *testing.T) {
	fsys := lint.OpenRootFS(symlinkRoot(t))
	t.Cleanup(func() { require.NoError(t, fsys.Close()) })
	rl, ok := fsys.(fs.ReadLinkFS)
	require.True(t, ok)
	fi, err := rl.Lstat("link.md")
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&fs.ModeSymlink)
}
