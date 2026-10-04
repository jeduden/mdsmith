package fix

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
	"github.com/jeduden/mdsmith/internal/rule"
)

// assertAllClosed fails for each recorded root a Stat still reads.
func assertAllClosed(t *testing.T, roots []fs.FS) {
	t.Helper()
	for i, r := range roots {
		_, err := fs.Stat(r, ".")
		assert.Error(t, err, "root %d is closed once its file's fix ends", i)
	}
}

// TestFixClosesEachFileRoots locks that Fix closes every root it opens
// for a file once that file's fix ends: the file's own directory and,
// for a file below RootDir, the project root. Not parallel: it records
// lint.OpenRootFS.
func TestFixClosesEachFileRoots(t *testing.T) {
	root := t.TempDir()
	host := filepath.Join(root, "host.md")
	nested := filepath.Join(root, "sub", "b.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0o755))
	require.NoError(t, os.WriteFile(host, []byte("# Host  \n"), 0o644))
	require.NoError(t, os.WriteFile(nested, []byte("# B  \n"), 0o644))
	opened := rootfstest.Record(t)

	fixer := &Fixer{
		Config:  &config.Config{Rules: map[string]config.RuleCfg{"mock-trailing": {Enabled: true}}},
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		RootDir: root,
	}
	require.Empty(t, fixer.Fix([]string{host, nested}).Errors)

	got := opened()
	// Each fix pass opens host's root, b's directory, and b's project
	// root; Fix runs a pass more to confirm the bytes are stable.
	require.GreaterOrEqual(t, len(got), 3)
	roots := make([]fs.FS, len(got))
	for i, r := range got {
		roots[i] = r
	}
	assertAllClosed(t, roots)
}

// TestSourceClosesItsProjectRoot locks that Source, given a SourceFS it
// does not own, closes only the project root it opened for a file below
// RootDir. Not parallel: it records lint.OpenRootFS.
func TestSourceClosesItsProjectRoot(t *testing.T) {
	root := t.TempDir()
	opened := rootfstest.Record(t)
	lent := os.DirFS(root)

	_, err := Source(SourceOptions{
		Config:   &config.Config{Rules: map[string]config.RuleCfg{"mock-trailing": {Enabled: true}}},
		Rules:    []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		Path:     filepath.Join("sub", "b.md"),
		Source:   []byte("# B  \n"),
		RootDir:  root,
		SourceFS: lent,
	})
	require.NoError(t, err)

	got := opened()
	require.Len(t, got, 1, "only the project root is opened")
	assertAllClosed(t, []fs.FS{got[0]})
	_, err = fs.Stat(lent, ".")
	assert.NoError(t, err, "the lent SourceFS is left alone")
}

// TestSourceBorrowsALentRootFS locks that Source, given a RootFS the
// caller lends, reads a file below RootDir through it and opens no
// project root of its own. Not parallel: it records lint.OpenRootFS.
func TestSourceBorrowsALentRootFS(t *testing.T) {
	root := t.TempDir()
	opened := rootfstest.Record(t)
	spy := &rootFSSpyRule{}

	_, err := Source(SourceOptions{
		Config:   &config.Config{Rules: map[string]config.RuleCfg{"root-spy": {Enabled: true}}},
		Rules:    []rule.Rule{spy},
		Path:     filepath.Join("sub", "b.md"),
		Source:   []byte("# B\n"),
		RootDir:  root,
		SourceFS: os.DirFS(root),
		RootFS:   spy.lent(root),
	})
	require.NoError(t, err)
	assert.Empty(t, opened(), "a lent RootFS leaves no project root to open")
	assert.True(t, *spy.sawLent, "the file reads its project root through the lent RootFS")
}

// rootFSSpyRule is a fixable rule that records whether the File it
// checks carries the RootFS the test lent. sawLent is a pointer so the
// clone Source checks reports into the test's flag.
type rootFSSpyRule struct {
	want    fs.FS
	sawLent *bool
}

// lent returns the RootFS the test lends and arms the spy to look for
// it. The view is a pointer of its own, so no other FS compares equal.
func (r *rootFSSpyRule) lent(root string) fs.FS {
	r.want = &struct{ fs.FS }{os.DirFS(root)}
	r.sawLent = new(bool)
	return r.want
}

func (*rootFSSpyRule) ID() string       { return "MDS997" }
func (*rootFSSpyRule) Name() string     { return "root-spy" }
func (*rootFSSpyRule) Category() string { return "test" }
func (r *rootFSSpyRule) Check(f *lint.File) []lint.Diagnostic {
	if f.RootFS == r.want {
		*r.sawLent = true
	}
	return nil
}
func (*rootFSSpyRule) Fix(f *lint.File) []byte { return f.Source }
