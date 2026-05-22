package linkgraph

import (
	"fmt"
	"testing"
	"testing/fstest"
)

// buildBenchFS returns a synthetic workspace of n .md files spread
// across a depth-2 directory tree, with one matching target stem
// `target`.
func buildBenchFS(n int, target string) fstest.MapFS {
	mfs := fstest.MapFS{}
	for i := 0; i < n; i++ {
		mfs[fmt.Sprintf("dir%d/sub/file%d.md", i%8, i)] = &fstest.MapFile{Data: []byte{}}
	}
	mfs["dir0/sub/"+target+".md"] = &fstest.MapFile{Data: []byte{}}
	return mfs
}

// BenchmarkResolveWikiLink_PerCall measures the cost of one
// ResolveWikiLink call against a 200-file workspace — the baseline
// for the per-call walk path used when no run-wide index is cached.
func BenchmarkResolveWikiLink_PerCall(b *testing.B) {
	mfs := buildBenchFS(200, "page")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := ResolveWikiLink(mfs, "from.md", "page"); !ok {
			b.Fatal("expected page to resolve")
		}
	}
}

// BenchmarkWikilinkIndex_Resolve measures one lookup against a
// prebuilt index of the same 200-file workspace — the steady-state
// cost MDS027 and `mdsmith list backlinks` see once the per-run
// cache is warm. Construction is outside the timer; the benchmark
// reflects what every call after the first pays.
func BenchmarkWikilinkIndex_Resolve(b *testing.B) {
	mfs := buildBenchFS(200, "page")
	idx := NewWikilinkIndex(mfs)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := idx.Resolve("page"); !ok {
			b.Fatal("expected page to resolve")
		}
	}
}

// BenchmarkNewWikilinkIndex measures one-time index construction:
// the fs.WalkDir + sort that the per-run cache amortises across
// every wikilink in every host file.
func BenchmarkNewWikilinkIndex(b *testing.B) {
	mfs := buildBenchFS(200, "page")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if NewWikilinkIndex(mfs) == nil {
			b.Fatal("index nil")
		}
	}
}
