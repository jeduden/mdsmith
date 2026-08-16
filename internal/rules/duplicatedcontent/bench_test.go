package duplicatedcontent

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// setupBenchCorpus writes n .md files into a fresh temp dir, each
// with a distinct short paragraph, and returns the dir.
func setupBenchCorpus(b *testing.B, n int) string {
	b.Helper()
	dir := b.TempDir()
	for i := 0; i < n; i++ {
		name := filepath.Join(dir, "file"+strconv.Itoa(i)+".md")
		body := "# Heading\n\n" + longParagraph("distinct paragraph text number "+strconv.Itoa(i)) + "\n"
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			b.Fatal(err)
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
// of 100 times).
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
