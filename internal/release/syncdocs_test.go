package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncDocs_CopiesMarkdownPreservingTree(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "top.md"), "# top\n")
	writeFile(t, filepath.Join(src, "guides", "install.md"), "# install\n")

	require.NoError(t, SyncDocs(src, filepath.Join(dst, "out")))

	assertFile(t, filepath.Join(dst, "out", "top.md"), "# top\n")
	assertFile(t, filepath.Join(dst, "out", "guides", "install.md"), "# install\n")
}

func TestSyncDocs_DropsProtoMd(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "proto.md"), "schema\n")
	writeFile(t, filepath.Join(src, "real.md"), "# real\n")

	require.NoError(t, SyncDocs(src, dst))

	_, err := os.Stat(filepath.Join(dst, "proto.md"))
	assert.True(t, os.IsNotExist(err), "proto.md should not be copied")
	assertFile(t, filepath.Join(dst, "real.md"), "# real\n")
}

func TestSyncDocs_RenamesIndexMdToUnderscoreIndex(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "index.md"), "# root\n")
	writeFile(t, filepath.Join(src, "guides", "index.md"), "# guides\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "_index.md"), "# root\n")
	assertFile(t, filepath.Join(dst, "guides", "_index.md"), "# guides\n")
	_, err := os.Stat(filepath.Join(dst, "index.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestSyncDocs_PrunesNonMarkdownNonImage(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "doc.md"), "x\n")
	writeFile(t, filepath.Join(src, "embed.go"), "package x\n")
	writeFile(t, filepath.Join(src, "diagram.svg"), "<svg/>\n")
	writeFile(t, filepath.Join(src, "notes.txt"), "ignore me\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "doc.md"), "x\n")
	assertFile(t, filepath.Join(dst, "diagram.svg"), "<svg/>\n")
	for _, dropped := range []string{"embed.go", "notes.txt"} {
		_, err := os.Stat(filepath.Join(dst, dropped))
		assert.Truef(t, os.IsNotExist(err), "%s should not be copied", dropped)
	}
}

func TestSyncDocs_RemovesEmptyDirsLeftAfterPruning(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "kept.md"), "x\n")
	// `internal-helpers` contains only non-content; the synced
	// tree should not expose a hollow directory.
	writeFile(t, filepath.Join(src, "internal-helpers", "embed.go"), "package x\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "kept.md"), "x\n")
	_, err := os.Stat(filepath.Join(dst, "internal-helpers"))
	assert.True(t, os.IsNotExist(err), "empty subdir should be pruned")
}

func TestSyncDocs_EscapesHugoShortcodePatterns(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	body := "Hugo uses `{{< readfile file=\"x.md\" >}}` and `{{% note %}}`.\n"
	writeFile(t, filepath.Join(src, "x.md"), body)

	require.NoError(t, SyncDocs(src, dst))

	got, err := os.ReadFile(filepath.Join(dst, "x.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got), `{{</* readfile file="x.md" */>}}`)
	assert.Contains(t, string(got), `{{%/* note */%}}`)
}

// TestSyncDocs_DoesNotDoubleEscape pins a regression: source docs
// that already contain Hugo's escape syntax (because the author
// is documenting how to escape shortcodes verbatim) must not be
// re-escaped on the second pass. Without this guard, the percent
// form's [^}]* group would re-match the already-escaped body and
// produce broken nested markers like `{{%/*/* … *//*%}}`.
func TestSyncDocs_DoesNotDoubleEscape(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	body := "Escape angle: `{{</* readfile */>}}` and percent: `{{%/* note */%}}`.\n"
	writeFile(t, filepath.Join(src, "x.md"), body)

	require.NoError(t, SyncDocs(src, dst))

	got, err := os.ReadFile(filepath.Join(dst, "x.md"))
	require.NoError(t, err)
	assert.Equal(t, body, string(got),
		"already-escaped patterns must pass through untouched")
}

func TestSyncDocs_OverwritesExistingDestination(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "fresh.md"), "fresh\n")
	writeFile(t, filepath.Join(dst, "stale.md"), "stale\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "fresh.md"), "fresh\n")
	_, err := os.Stat(filepath.Join(dst, "stale.md"))
	assert.True(t, os.IsNotExist(err), "stale destination files should be cleared")
}

func TestSyncDocs_MissingSourceFails(t *testing.T) {
	err := SyncDocs(filepath.Join(t.TempDir(), "does-not-exist"), t.TempDir())
	assert.Error(t, err)
}

// TestSyncDocs_RefusesSameSrcAndDst pins the safety check that
// blocks SyncDocs from running when caller passes overlapping
// paths. Without the guard, the leading RemoveAll(dstDir) would
// wipe the very tree we're about to read from.
func TestSyncDocs_RefusesSameSrcAndDst(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.md"), "x\n")
	err := SyncDocs(dir, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same path")

	// Source survived: the guard fired before the RemoveAll.
	body, readErr := os.ReadFile(filepath.Join(dir, "x.md"))
	require.NoError(t, readErr)
	assert.Equal(t, "x\n", string(body))
}

func TestSyncDocs_RefusesDstInsideSrc(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	err := SyncDocs(src, filepath.Join(src, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside")
	_, readErr := os.Stat(filepath.Join(src, "x.md"))
	require.NoError(t, readErr, "source must survive the rejected call")
}

func TestSyncDocs_RefusesSrcInsideDst(t *testing.T) {
	dst := t.TempDir()
	src := filepath.Join(dst, "inner")
	require.NoError(t, os.MkdirAll(src, 0o755))
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	err := SyncDocs(src, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside")
	_, readErr := os.Stat(filepath.Join(src, "x.md"))
	require.NoError(t, readErr, "source must survive the rejected call")
}

// TestSyncDocs_StatNonNotExistWrapsError covers the
// non-ErrNotExist branch of the Stat error handler. The
// fakeFS-injected error is not fs.ErrNotExist, so SyncDocs must
// surface it through %w rather than collapsing it to the
// "source not found" message.
func TestSyncDocs_StatNonNotExistWrapsError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnStatCall = 1

	err := NewWithFS(ff).SyncDocs(src, dst)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "stat ")
	assert.NotContains(t, err.Error(), "source not found")
}

// TestSyncDocs_AbsResolveErrorBubblesUp covers the absPath()
// error branches in checkSyncDocsPaths. filepath.Abs only fails
// when os.Getwd does, which is unreachable from a test process —
// the package-level absPath seam (shared with BuildWheels) lets
// us drive both branches deterministically.
func TestSyncDocs_AbsResolveErrorBubblesUp(t *testing.T) {
	orig := absPath
	t.Cleanup(func() { absPath = orig })

	// Branch 1: src resolve fails.
	absPath = func(string) (string, error) { return "", errInjected }
	err := SyncDocs("any-src", "any-dst")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "resolve src")

	// Branch 2: src succeeds, dst fails.
	calls := 0
	absPath = func(p string) (string, error) {
		calls++
		if calls == 2 {
			return "", errInjected
		}
		return orig(p)
	}
	err = SyncDocs(t.TempDir(), "any-dst")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "resolve dst")
}

func TestSyncDocs_DestRemoveAllErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnRemoveAllCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_DestMkdirAllErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_ReadDirErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnReadDirCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_ReadFileErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnReadFileCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_WriteFileErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnWriteFileCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_ChildMkdirErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "sub", "x.md"), "x\n")
	ff := newFakeFS()
	// MkdirAll #1 = SyncDocs's own dest mkdir. #2 = child subdir.
	ff.failOnMkdirAllCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_RecursiveFailurePropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "x\n")
	writeFile(t, filepath.Join(src, "sub", "b.md"), "y\n")
	ff := newFakeFS()
	// ReadFile #1 = a.md (succeeds). The recursive syncDocsDir
	// call hits ReadFile #2 on sub/b.md, which fails. Covers the
	// parent loop's "if err != nil { return ... }" arm after the
	// recursive call.
	ff.failOnReadFileCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_EmptySubdirRemoveAllErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "kept.md"), "x\n")
	// `pruned/` contains only an excluded extension, so the
	// recursive call ends with !wrote and tries RemoveAll on the
	// freshly-created empty child. RemoveAll #1 = SyncDocs's
	// initial dest wipe (passes); #2 = the empty-child cleanup
	// (fails).
	writeFile(t, filepath.Join(src, "pruned", "embed.go"), "package x\n")
	ff := newFakeFS()
	ff.failOnRemoveAllCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	assert.Equal(t, want, string(got))
}
