package lsp

import (
	"fmt"
	"testing"
)

// manyOpenDocsServer builds a bare Server (no transport I/O) with n
// open documents, none of which match rel — the shape of a workspace
// with several buffers open in the editor while a rename targets a
// file that isn't one of them (the common case: a batch rename or a
// ref-def rewrite touches files the user hasn't opened).
func manyOpenDocsServer(n int) (*Server, string) {
	s := New(Options{})
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("open/doc%d.md", i)
		uri := "file:///" + path
		s.docs.set(uri, &document{uri: uri, path: path, text: []byte("# Doc\n")})
	}
	return s, "unopened/target.md"
}

// BenchmarkResolveURIAndSource_NoMatch exercises resolveURIAndSource's
// open-document scan when none of the open buffers match; benchstat-
// friendly (no assertion), consumed by
// TestResolveURIAndSourceNoMatchAllocBudget below for the enforced gate.
func BenchmarkResolveURIAndSource_NoMatch(b *testing.B) {
	s, rel := manyOpenDocsServer(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = s.resolveURIAndSource(rel)
	}
}

// resolveURIAndSourceNoMatchAllocBudget pins the gate's real effect.
// The old implementation called openURIs() (one slice allocation) then
// docs.get(uri) for every open document (one struct-copy allocation
// each, even for a non-matching document, since get() always copies
// before the caller can check doc.path) — O(open-docs) allocations on
// every miss. findByPath scans under a single read lock and only
// copies the document that actually matches, so a miss across the same
// 50 open documents allocates none of that per-document copy.
const resolveURIAndSourceNoMatchAllocBudget = 3

// TestResolveURIAndSourceNoMatchAllocBudget pins the alloc regression
// gate under a normal `go test` run, following the Benchmark/Test pair
// pattern used elsewhere in this codebase for a gate that CI's bench
// jobs don't cover (see noreferencestyle's
// TestCheckFootnotes_NoNeedleBudget).
func TestResolveURIAndSourceNoMatchAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	s, rel := manyOpenDocsServer(50)

	const runs = 100
	allocs := testing.AllocsPerRun(runs, func() {
		_, _, _ = s.resolveURIAndSource(rel)
	})
	t.Logf("resolveURIAndSource (miss across 50 open docs) allocs/op = %.0f (budget = %d)",
		allocs, resolveURIAndSourceNoMatchAllocBudget)
	if allocs > float64(resolveURIAndSourceNoMatchAllocBudget) {
		t.Fatalf("resolveURIAndSource allocs/op = %.0f exceeds budget %d: "+
			"the open-document scan must not copy every open document "+
			"just to check its path — see internal/lsp/documents.go's "+
			"findByPath", allocs, resolveURIAndSourceNoMatchAllocBudget)
	}
}
