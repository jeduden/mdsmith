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
