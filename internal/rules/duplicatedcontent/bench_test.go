package duplicatedcontent

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// setupBenchCorpus writes n .md files into a fresh temp dir, each
// with a distinct short paragraph, and returns the dir. Takes
// testing.TB so both benchmarks and the alloc-budget test below can
// share it.
func setupBenchCorpus(tb testing.TB, n int) string {
	tb.Helper()
	dir := tb.TempDir()
	for i := 0; i < n; i++ {
		name := filepath.Join(dir, "file"+strconv.Itoa(i)+".md")
		body := "# Heading\n\n" + longParagraph("distinct paragraph text number "+strconv.Itoa(i)) + "\n"
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	return dir
}

// BenchmarkCheck_ManyHostFilesSharedCorpus checks every file in an
// N-file corpus against a shared RunCache, the way the engine runs a
// real workspace. Before caching the resolved corpus file list on
// RunCache.GlobMatches, buildCorpusIndex re-walked the whole corpus
// directory tree with fs.WalkDir on every host file's Check call — an
// O(N) directory walk repeated N times across the run, on top of the
// per-file fingerprint memoization TestCheck_RunCacheReusesCorpusParseAcrossHostFiles
// already pins.
//
// Baseline (2026-08-10, 100-file corpus, 100 host-file Check calls
// sharing one RunCache): ~20.4ms/op, ~101k allocs/op. After caching
// the corpus file list: ~14.0ms/op, ~50k allocs/op (-31% time, -50%
// allocs — halving allocs tracks the walk itself running once instead
// of 100 times). After also memoizing the aggregate index build
// (RunCache.CorpusIndex, TestBuildCorpusIndex_MemoizedAcrossHostFiles):
// ~9.0ms/op, ~10k allocs/op (-36% time, -80% allocs from the prior
// step) — the per-host-file map-build-and-sort was still the
// remaining O(N) cost repeated N times.
func BenchmarkCheck_ManyHostFilesSharedCorpus(b *testing.B) {
	const n = 100
	dir := setupBenchCorpus(b, n)
	names := make([]string, n)
	datas := make([][]byte, n)
	for i := 0; i < n; i++ {
		names[i] = filepath.Join(dir, "file"+strconv.Itoa(i)+".md")
		data, err := os.ReadFile(names[i])
		if err != nil {
			b.Fatal(err)
		}
		datas[i] = data
	}
	fsys := os.DirFS(dir)

	b.ResetTimer()
	b.ReportAllocs()
	for iter := 0; iter < b.N; iter++ {
		runCache := lint.NewRunCache()
		for i := 0; i < n; i++ {
			f, err := lint.NewFile(names[i], datas[i])
			if err != nil {
				b.Fatal(err)
			}
			f.FS = fsys
			f.RootDir = dir
			f.RootFS = fsys
			f.RunCache = runCache
			_ = (&Rule{}).Check(f)
		}
	}
}

// TestCheck_ManyHostFilesSharedCorpus_AllocBudget pins
// BenchmarkCheck_ManyHostFilesSharedCorpus's allocation count as a
// real CI gate (a benchmark alone only runs on request). The ceiling
// sits between the pre-CorpusIndex baseline (~50k allocs/op) and the
// current measured figure (~10k allocs/op) so a reintroduced
// per-host-file aggregate rebuild trips it, with headroom for the
// per-file leaf allocations (paragraph extraction, diagnostics) that
// legitimately scale with corpus size.
func TestCheck_ManyHostFilesSharedCorpus_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	const n = 100
	dir := setupBenchCorpus(t, n)
	names := make([]string, n)
	datas := make([][]byte, n)
	for i := 0; i < n; i++ {
		names[i] = filepath.Join(dir, "file"+strconv.Itoa(i)+".md")
		data, err := os.ReadFile(names[i])
		if err != nil {
			t.Fatal(err)
		}
		datas[i] = data
	}
	fsys := os.DirFS(dir)

	const runs = 3
	allocs := testing.AllocsPerRun(runs, func() {
		runCache := lint.NewRunCache()
		for i := 0; i < n; i++ {
			f, err := lint.NewFile(names[i], datas[i])
			if err != nil {
				t.Fatal(err)
			}
			f.FS = fsys
			f.RootDir = dir
			f.RootFS = fsys
			f.RunCache = runCache
			_ = (&Rule{}).Check(f)
		}
	})

	const ceiling = 20000
	t.Logf("Check across %d host files sharing a RunCache: allocs/op = %.0f", n, allocs)
	if allocs > ceiling {
		t.Fatalf("allocs/op = %.0f, want <= %d (aggregate corpus-index cache regressed?)", allocs, ceiling)
	}
}
