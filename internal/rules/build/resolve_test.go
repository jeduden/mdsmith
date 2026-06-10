package build

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CheckGlobMatchCap ---

func TestCheckGlobMatchCap_Under(t *testing.T) {
	assert.NoError(t, CheckGlobMatchCap(0))
	assert.NoError(t, CheckGlobMatchCap(1))
	assert.NoError(t, CheckGlobMatchCap(MaxGlobMatches))
}

func TestCheckGlobMatchCap_Over(t *testing.T) {
	err := CheckGlobMatchCap(MaxGlobMatches + 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10000")
}

// --- ResolvePathInRoot ---

func TestResolvePathInRoot_PlainInRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "in.md"), []byte("x"), 0o644))
	got, err := ResolvePathInRoot(root, "in.md", true)
	require.NoError(t, err)
	assert.Equal(t, "in.md", got)
}

func TestResolvePathInRoot_NestedInRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a/b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a/b/in.md"), []byte("x"), 0o644))
	got, err := ResolvePathInRoot(root, "a/b/in.md", true)
	require.NoError(t, err)
	assert.Equal(t, "a/b/in.md", got)
}

func TestResolvePathInRoot_MissingInputErrors(t *testing.T) {
	root := t.TempDir()
	// mustExist=true: a non-existent input is an error.
	_, err := ResolvePathInRoot(root, "nope.md", true)
	require.Error(t, err)
}

func TestResolvePathInRoot_MissingOutputAllowed(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "build"), 0o755))
	// mustExist=false: an output that does not exist yet resolves to its
	// in-root relative path via the longest existing prefix.
	got, err := ResolvePathInRoot(root, "build/out.html", false)
	require.NoError(t, err)
	assert.Equal(t, "build/out.html", got)
}

func TestResolvePathInRoot_SymlinkedInputEscapesRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable on Windows CI")
	}
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.md")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o644))
	// A symlink inside root that points outside it.
	link := filepath.Join(root, "leak.md")
	require.NoError(t, os.Symlink(target, link))

	_, err := ResolvePathInRoot(root, "leak.md", true)
	require.Error(t, err, "a symlinked input resolving outside the root is an error")
}

func TestResolvePathInRoot_SymlinkedOutputDirEscapesRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable on Windows CI")
	}
	root := t.TempDir()
	outside := t.TempDir()
	// A symlinked directory inside root pointing outside; the output
	// file does not exist yet, so the check walks the existing prefix
	// (the symlinked dir) with EvalSymlinks.
	link := filepath.Join(root, "dist")
	require.NoError(t, os.Symlink(outside, link))

	_, err := ResolvePathInRoot(root, "dist/out.html", false)
	require.Error(t, err, "an output under a symlinked-out directory is an error")
}

func TestResolvePathInRoot_SymlinkWithinRootOK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable on Windows CI")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "real/in.md"), []byte("x"), 0o644))
	// A symlink inside root pointing to another in-root path is fine.
	require.NoError(t, os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")))

	got, err := ResolvePathInRoot(root, "link/in.md", true)
	require.NoError(t, err)
	// The result is normalised back to forward slashes and stays in-root.
	assert.Equal(t, "real/in.md", got)
}
