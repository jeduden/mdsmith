package lsp

import (
	"fmt"
	"path/filepath"
	"testing"
)

// manyOpenDocsServer builds a bare Server (no transport I/O) with a
// configured workspace root and n open documents, none of which match
// rel — the shape of a workspace with several buffers open in the
// editor while a rename targets a file that isn't one of them (the
// common case: a batch rename or a ref-def rewrite touches files the
// user hasn't opened). Document paths are absolute, matching what
// handleDidOpen actually stores (uriToPath(uri)), and the root is set,
// so resolveURIAndSource's match closure exercises the real
// filepath.Rel + index.NormalizePath cost instead of workspaceRelative's
// root=="" no-op short-circuit.
func manyOpenDocsServer(n int) (*Server, string) {
	root := filepath.FromSlash("/workspace")
	s := New(Options{})
	s.rootDir = root
	for i := 0; i < n; i++ {
		rel := fmt.Sprintf("open/doc%d.md", i)
		path := filepath.Join(root, filepath.FromSlash(rel))
		uri := "file://" + filepath.ToSlash(path)
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

// resolveURIAndSourceNoMatchAllocBudget pins the gate's real effect on
// a realistic fixture (absolute document paths, a configured root, so
// the match closure runs the real filepath.Rel + index.NormalizePath
// work rather than workspaceRelative's root=="" no-op short-circuit).
// The old implementation called openURIs() (one slice allocation) then
// docs.get(uri) for every open document — one struct-copy allocation
// each, even for a non-matching document, since get() always copies
// before the caller can check doc.path — measured at 64 allocs/op on
// this fixture. findByPath's single-lock (uri, path)-pair snapshot
// still leaves the per-candidate path-normalization cost (match runs
// once per candidate either way), but drops the per-candidate document
// struct copy entirely: measured at 14 allocs/op. The budget sits with
// headroom above that for environment variance, well below the old
// figure, so a regression that reintroduces the per-candidate copy
// trips this test.
const resolveURIAndSourceNoMatchAllocBudget = 25

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
