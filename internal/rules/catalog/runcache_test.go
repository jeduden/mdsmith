package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCache_CrossHostCatalogReadsTargetOnce is the multi-host
// counterpart to memo_test.go's per-Check pins. CLAUDE.md, PLAN.md,
// and every docs/**/index.md in the repo carry catalogs over an
// overlapping docs/** glob; before the run-scoped cache each host
// re-read the same matched targets, so a 569-file repo paid
// O(host-files x shared-targets) front-matter reads.
//
// We Check two host files that share one fs.FS (wrapped with a
// counter that bumps on every Open / ReadFile) and one RunCache. Each
// matched target lives at the same fsys-relative path and the same
// absolute cache key. We assert the second Check serves both reads
// from the cache — the count stays at the single-Check baseline
// rather than doubling.
func TestRunCache_CrossHostCatalogReadsTargetOnce(t *testing.T) {
	inner := fstest.MapFS{
		"host-a.md": &fstest.MapFile{Data: []byte(catalogHostSource("Host A"))},
		"host-b.md": &fstest.MapFile{Data: []byte(catalogHostSource("Host B"))},
		"docs/a.md": &fstest.MapFile{Data: []byte("---\ntitle: Alpha\n---\n# A\n")},
		"docs/b.md": &fstest.MapFile{Data: []byte("---\ntitle: Beta\n---\n# B\n")},
	}
	cfs := newCountingFS(inner)
	cache := lint.NewRunCache()

	for _, host := range []string{"host-a.md", "host-b.md"} {
		runCheckOnHost(t, cfs, cache, host)
	}

	for _, p := range []string{"docs/a.md", "docs/b.md"} {
		got := cfs.count(p)
		assert.Greaterf(t, got, 0,
			"%s should be read at least once during the first Check; a "+
				"zero count means the catalog rule never reached it and "+
				"the cross-host assertion below is trivially true",
			p)
		assert.LessOrEqualf(t, got, 2,
			"%s should be read at most twice across BOTH host files "+
				"(one front-matter read + one include-cycle scan, both "+
				"shared across hosts via RunCache). Without the cache "+
				"each host re-reads, doubling the count to 4. Observed=%d.",
			p, got)
	}
}

// TestRunCache_InvalidateForcesReread pins the LSP invalidation seam:
// once Invalidate(absPath) fires between Checks, the next Check that
// reaches absPath re-reads. The host file's catalog globs docs/a.md
// and docs/b.md; after the first Check both are cached. The test
// invalidates only docs/a.md and re-Checks the same host. The
// counter for docs/a.md bumps; the counter for docs/b.md stays at
// its baseline.
func TestRunCache_InvalidateForcesReread(t *testing.T) {
	inner := fstest.MapFS{
		"host.md":   &fstest.MapFile{Data: []byte(catalogHostSource("Host"))},
		"docs/a.md": &fstest.MapFile{Data: []byte("---\ntitle: Alpha\n---\n# A\n")},
		"docs/b.md": &fstest.MapFile{Data: []byte("---\ntitle: Beta\n---\n# B\n")},
	}
	cfs := newCountingFS(inner)
	cache := lint.NewRunCache()

	runCheckOnHost(t, cfs, cache, "host.md")
	baselineA := cfs.count("docs/a.md")
	baselineB := cfs.count("docs/b.md")
	require.Greater(t, baselineA, 0,
		"first Check must reach docs/a.md so the baseline is meaningful")
	require.Greater(t, baselineB, 0,
		"first Check must reach docs/b.md so the baseline is meaningful")

	// Compute the absolute cache key the catalog rule uses for
	// docs/a.md: localFSResolution sets gitignoreBase =
	// filepath.Abs(filepath.Dir(f.Path)) = abs(".") = the test
	// process's cwd. Mirror that here so Invalidate hits the right
	// slot.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	cache.Invalidate(filepath.Join(cwd, "docs", "a.md"))

	runCheckOnHost(t, cfs, cache, "host.md")

	assert.Greater(t, cfs.count("docs/a.md"), baselineA,
		"docs/a.md was invalidated; a follow-up Check must re-read it. "+
			"baseline=%d, after-invalidate=%d", baselineA, cfs.count("docs/a.md"))
	assert.Equal(t, baselineB, cfs.count("docs/b.md"),
		"docs/b.md was NOT invalidated; its read count must hold at "+
			"the baseline. Invalidate must only drop the named path.")
}

// runCheckOnHost reads the host file from cfs and runs Catalog.Check
// against it with the supplied RunCache attached. Shared by the
// cross-host and invalidate tests so both exercise the same wiring.
func runCheckOnHost(t *testing.T, cfs *countingFS, cache *lint.RunCache, host string) {
	t.Helper()
	data, err := cfs.ReadFile(host)
	require.NoError(t, err)
	f, err := lint.NewFile(host, data)
	require.NoError(t, err)
	f.FS = cfs
	f.RunCache = cache

	r := &Rule{}
	diags := r.Check(f)
	require.Empty(t, diags,
		"fixture is well-formed; the run-cache optimization must not "+
			"change rule correctness")
}

// catalogHostSource produces a host-file body with a catalog over
// docs/*.md plus matching expected rows. {filename} substitutes the
// fs-relative match (docs/X.md under localFSResolution) so the row
// template carries no docs/ prefix of its own. Used by both tests so
// the fixture stays identical.
func catalogHostSource(title string) string {
	return "---\nkinds: []\n---\n# " + title + "\n\n" +
		"<?catalog\n" +
		"glob: \"docs/*.md\"\n" +
		"row: \"- [{title}]({filename})\"\n" +
		"?>\n" +
		"- [Alpha](docs/a.md)\n" +
		"- [Beta](docs/b.md)\n" +
		"<?/catalog?>\n"
}
