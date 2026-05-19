package catalog

import (
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingFS wraps an fstest.MapFS and counts how many times each path
// is read (via ReadFile, the path lint.ReadFSFileLimited and the YAML
// front-matter reader take when MaxInputBytes is unset). doublestar's
// glob walk uses ReadDir/Stat, not ReadFile, so a matched markdown
// file's count reflects only the rule passes that re-read its content.
type countingFS struct {
	inner fstest.MapFS
	mu    sync.Mutex
	reads map[string]int
}

func newCountingFS(inner fstest.MapFS) *countingFS {
	return &countingFS{inner: inner, reads: map[string]int{}}
}

func (c *countingFS) bump(name string) {
	c.mu.Lock()
	c.reads[name]++
	c.mu.Unlock()
}

func (c *countingFS) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads[name]
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.bump(name)
	return c.inner.Open(name)
}

func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.bump(name)
	return c.inner.ReadFile(name)
}

func (c *countingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return c.inner.ReadDir(name)
}

func (c *countingFS) Stat(name string) (fs.FileInfo, error) {
	return c.inner.Stat(name)
}

func (c *countingFS) Glob(pattern string) ([]string, error) {
	return c.inner.Glob(pattern)
}

// TestCheck_DoesNotRebuildCatalogEntriesPerPass pins that one Check of
// a file with a single catalog directive reads each matched file's
// content a bounded number of times. Before the per-Check entry memo,
// buildCatalogEntries ran three times per directive — once for the
// generate (out-of-date) pass, once for injection, once for
// case-mismatch — so every matched file's front matter was read three
// times, plus once more for the include-cycle scan: four reads per
// matched file. The memo collapses the three entry builds to one, so a
// matched file is read at most twice (front matter once, cycle scan
// once). On the directive-dense repo corpus this is ~24% of the whole
// `mdsmith check` run; the neutral corpus has no directives and never
// paid it, which is the repo-vs-neutral gap.
func TestCheck_DoesNotRebuildCatalogEntriesPerPass(t *testing.T) {
	src := `<?catalog
glob: "docs/*.md"
row: "- [{title}]({filename})"
?>
- [Alpha](docs/a.md)
- [Beta](docs/b.md)
<?/catalog?>
`
	inner := fstest.MapFS{
		"docs/a.md": {Data: []byte("---\ntitle: Alpha\n---\n# A\n")},
		"docs/b.md": {Data: []byte("---\ntitle: Beta\n---\n# B\n")},
	}
	cfs := newCountingFS(inner)

	f, err := lint.NewFile("index.md", []byte(src))
	require.NoError(t, err)
	f.FS = cfs

	r := &Rule{}
	diags := r.Check(f)
	require.Empty(t, diags, "fixture is well-formed; optimization must not change correctness")

	for _, p := range []string{"docs/a.md", "docs/b.md"} {
		assert.LessOrEqualf(t, cfs.count(p), 2,
			"matched file %s should be read at most twice per Check "+
				"(front matter once + cycle scan once), got %d — the "+
				"generate/injection/case-mismatch passes are rebuilding entries",
			p, cfs.count(p))
	}
}

// TestCheck_DoesNotRescanIncludesPerDirective pins that the
// include-cycle scan parses each matched file once per Check, not once
// per catalog directive. The two minimal (glob-only, no row) catalog
// directives below match the same files and read no front matter, so
// the only reads of a matched file come from the include-cycle scan
// (checkCatalogIncludeCycle -> scanIncludesForTarget -> a full
// lint.NewFile parse). Before the per-Check adjacency memo that scan
// re-read and re-parsed every matched file once per directive; files
// like CLAUDE.md carry three catalogs over an overlapping docs/**
// glob, so the same docs tree was parsed three times. With the memo a
// matched file is read once for the whole Check.
func TestCheck_DoesNotRescanIncludesPerDirective(t *testing.T) {
	src := `<?catalog
glob: "docs/*.md"
?>
- [a.md](docs/a.md)
- [b.md](docs/b.md)
<?/catalog?>

<?catalog
glob: "docs/*.md"
?>
- [a.md](docs/a.md)
- [b.md](docs/b.md)
<?/catalog?>
`
	inner := fstest.MapFS{
		"docs/a.md": {Data: []byte("# A\n")},
		"docs/b.md": {Data: []byte("# B\n")},
	}
	cfs := newCountingFS(inner)

	f, err := lint.NewFile("index.md", []byte(src))
	require.NoError(t, err)
	f.FS = cfs

	r := &Rule{}
	diags := r.Check(f)
	require.Empty(t, diags, "fixture is well-formed; optimization must not change correctness")

	for _, p := range []string{"docs/a.md", "docs/b.md"} {
		assert.LessOrEqualf(t, cfs.count(p), 1,
			"matched file %s should be parsed once per Check for the "+
				"include-cycle scan, not once per directive, got %d",
			p, cfs.count(p))
	}
}

// TestCheck_DoesNotReReadFrontMatterPerDirective pins that a matched
// file's front matter is read once per Check, not once per catalog
// directive that globs it. The two directives below carry distinct
// (path, line) keys so their entry builds are not shared, yet they
// match the same files; without a per-path front-matter memo each
// matched file's YAML was read once per directive. Files that carry
// several catalogs over an overlapping glob — CLAUDE.md has three on
// docs/** — paid this per directive. Combined with the cycle-scan
// memo, a matched file is now read at most twice for the whole Check
// (front matter once + cycle scan once) regardless of directive count.
func TestCheck_DoesNotReReadFrontMatterPerDirective(t *testing.T) {
	src := `<?catalog
glob: "docs/*.md"
row: "- [{title}]({filename})"
?>
- [Alpha](docs/a.md)
- [Beta](docs/b.md)
<?/catalog?>

<?catalog
glob: "docs/*.md"
row: "- [{title}]({filename})"
?>
- [Alpha](docs/a.md)
- [Beta](docs/b.md)
<?/catalog?>
`
	inner := fstest.MapFS{
		"docs/a.md": {Data: []byte("---\ntitle: Alpha\n---\n# A\n")},
		"docs/b.md": {Data: []byte("---\ntitle: Beta\n---\n# B\n")},
	}
	cfs := newCountingFS(inner)

	f, err := lint.NewFile("index.md", []byte(src))
	require.NoError(t, err)
	f.FS = cfs

	r := &Rule{}
	diags := r.Check(f)
	require.Empty(t, diags, "fixture is well-formed; optimization must not change correctness")

	for _, p := range []string{"docs/a.md", "docs/b.md"} {
		assert.LessOrEqualf(t, cfs.count(p), 2,
			"matched file %s should be read at most twice per Check "+
				"(front matter once + cycle scan once) no matter how "+
				"many directives glob it, got %d",
			p, cfs.count(p))
	}
}
