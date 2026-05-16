package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ruleIndexFixture = `---
title: Rule Directory
summary: >-
  Complete list of all mdsmith rules.
---
# Rule Directory

All mdsmith rules.

<?catalog
glob: "MDS*/README.md"
?>
- [MDS001](MDS001-line-length/README.md)
<?/catalog?>
`

// ruleIndexAt writes the rule-directory fixture to
// <parent>/internal/rules/index.md so BuildWebsite's sibling
// lookup (filepath.Dir(srcDir)/internal/rules) finds it when
// srcDir is <parent>/docs.
func ruleIndexAt(t *testing.T, parent string) {
	t.Helper()
	writeFile(t, filepath.Join(parent, "internal", "rules", "index.md"), ruleIndexFixture)
}

func TestBuildWebsite_PublishesRuleDirectory(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "docs")
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	ruleIndexAt(t, parent)

	require.NoError(t, NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(src, dst, false))

	assertFile(t, filepath.Join(dst, "top.md"), "top body\n")
	got, err := os.ReadFile(filepath.Join(dst, "rules", "_index.md"))
	require.NoError(t, err, "Rules section page must be written")
	body := string(got)
	assert.Contains(t, body,
		"https://github.com/jeduden/mdsmith/blob/main/internal/rules/MDS001-line-length/README.md",
		"rule-README link must be rewritten to its absolute GitHub URL")
	assert.NotContains(t, body, "<?catalog", "directive markers must be stripped")
	assert.NotContains(t, body, "# Rule Directory", "the body H1 must be lifted to front matter")
	assert.Contains(t, body, "title: Rule Directory", "front-matter title is preserved")
}

func TestBuildWebsite_NoRuleDirectoryIsNotAnError(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "docs")
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")

	require.NoError(t, NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(src, dst, false))

	_, err := os.Stat(filepath.Join(dst, "rules"))
	assert.True(t, os.IsNotExist(err), "no rule index -> no Rules section, no error")
}

func TestSyncRuleIndex_ReadErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent) // ReadDir finds a real (non-symlink) index.md
	ff := newFakeFS()
	ff.failOnReadFileCall = 1 // errInjected, not fs.ErrNotExist

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read rule index")
}

func TestSyncRuleIndex_ReadDirErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent)
	ff := newFakeFS()
	ff.failOnReadDirCall = 1 // not fs.ErrNotExist

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read rule dir")
}

func TestSyncRuleIndex_SymlinkIndexSkipped(t *testing.T) {
	parent := t.TempDir()
	rulesDir := filepath.Join(parent, "internal", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	// A symlink planted at index.md must not be followed: the
	// link target's bytes would otherwise be published into the
	// Hugo tree.
	secret := filepath.Join(parent, "secret.md")
	writeFile(t, secret, "PRIVATE RUNNER FILE\n")
	require.NoError(t, os.Symlink(secret, filepath.Join(rulesDir, "index.md")))
	dst := filepath.Join(t.TempDir(), "out")

	require.NoError(t, NewWithFS(osFS{}).syncRuleIndex(rulesDir, dst))

	_, err := os.Stat(filepath.Join(dst, "rules"))
	assert.True(t, os.IsNotExist(err), "symlinked index.md -> no Rules section")
}

func TestSyncRuleIndex_NoIndexFileIsNoop(t *testing.T) {
	rulesDir := t.TempDir() // exists, but holds no index.md
	// A decoy sibling exercises the non-index loop branch.
	writeFile(t, filepath.Join(rulesDir, "proto.md"), "schema\n")
	dst := filepath.Join(t.TempDir(), "out")

	require.NoError(t, NewWithFS(osFS{}).syncRuleIndex(rulesDir, dst))

	_, err := os.Stat(filepath.Join(dst, "rules"))
	assert.True(t, os.IsNotExist(err))
}

func TestSyncRuleIndex_MkdirErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent)
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1 // ReadFile succeeds, the rules-dir mkdir fails

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mkdir")
}

func TestSyncRuleIndex_WriteErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent)
	ff := newFakeFS()
	ff.failOnWriteFileCall = 1 // ReadFile + MkdirAll succeed, the page write fails

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "write rule index")
}

func TestBuildWebsite_RunsFixThenSync(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	rec := &recordingRunner{}

	require.NoError(t, NewWithDeps(osFS{}, rec).BuildWebsite(src, dst, true))

	require.Len(t, rec.calls, 1)
	assert.Equal(t, "go", rec.calls[0].name)
	assert.Equal(t, []string{"run", "./cmd/mdsmith", "fix", src}, rec.calls[0].args)
	assertFile(t, filepath.Join(dst, "top.md"), "top body\n")
}

func TestBuildWebsite_NoFixSkipsRunner(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	rec := &recordingRunner{}

	require.NoError(t, NewWithDeps(osFS{}, rec).BuildWebsite(src, dst, false))

	assert.Empty(t, rec.calls, "no-fix must not invoke the runner")
	assertFile(t, filepath.Join(dst, "top.md"), "top body\n")
}

func TestBuildWebsite_FixFailureWraps(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")

	err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1}).
		BuildWebsite(src, filepath.Join(t.TempDir(), "out"), true)

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mdsmith fix")
}

func TestBuildWebsite_SyncErrorSurfacedNotDoubleWrapped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.md"), "x\n")

	// recordingRunner succeeds on fix; src==dst trips the
	// SyncDocs overlap guard. BuildWebsite must surface that
	// error unwrapped — SyncDocs already contextualizes it,
	// so there must be no duplicated prefix.
	err := NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(dir, dir, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "same path")
	assert.NotContains(t, err.Error(), "sync ", "no redundant build-website wrap")
}

// TestBuildWebsite_SyncErrorNotDoubleWrapped is the regression
// for the duplicated `sync a -> b: sync a -> b:` prefix:
// SyncDocs already wraps the syncDocsDir failure with the
// `sync <src> -> <dst>:` prefix, so BuildWebsite must not add
// its own — the prefix must appear exactly once.
func TestBuildWebsite_SyncErrorNotDoubleWrapped(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnReadDirCall = 1

	err := NewWithDeps(ff, &recordingRunner{}).
		BuildWebsite(src, filepath.Join(t.TempDir(), "out"), false)

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read dir")
	assert.Equal(t, 1, strings.Count(err.Error(), "sync "),
		"the sync prefix must appear exactly once")
}
