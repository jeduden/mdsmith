package ext

import "testing"

// TestBestMatchAt_AllocBudget pins bestMatchAt's per-call allocation
// count. findMatches calls bestMatchAt once per word-boundary
// candidate position in a Text node's body — on a prose-heavy
// document with several defined abbreviations that is the dominant
// allocator in the whole-document parse (every parse installs the
// Abbreviation extender by default). The previous implementation
// converted every table term from its map key to []byte on every
// (position, term) probe; bestMatchAt must not allocate at all once
// the term byte forms are precomputed
// (docs/development/high-performance-go.md "Stay in []byte").
func TestBestMatchAt_AllocBudget(t *testing.T) {
	tbl := abbrTable{
		"HTML": nil, "CSS": nil, "API": nil, "URL": nil, "SQL": nil,
	}
	terms := tableTerms(tbl)
	body := []byte("some plain prose with no matching terms at all")
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = bestMatchAt(body, 0, terms)
	})
	if allocs > 0 {
		t.Fatalf("bestMatchAt allocs per call: want 0, got %v", allocs)
	}
}

// TestFindMatches_AllocBudget pins findMatches's own per-call
// allocation count given a precomputed terms slice. Transform builds
// terms once per document via tableTerms and threads it through
// rewriteText into findMatches, which is otherwise called once per
// Text node in the document (one or more per paragraph/heading/
// list-item) — tableTerms itself must not run again inside
// findMatches, or the per-node cost this hoist is meant to eliminate
// would reappear (docs/development/high-performance-go.md "Skip work
// you don't need").
func TestFindMatches_AllocBudget(t *testing.T) {
	tbl := abbrTable{
		"HTML": nil, "CSS": nil, "API": nil, "URL": nil, "SQL": nil,
	}
	terms := tableTerms(tbl)
	body := []byte("some plain prose with no matching terms at all")
	allocs := testing.AllocsPerRun(1000, func() {
		_ = findMatches(body, terms)
	})
	if allocs > 0 {
		t.Fatalf("findMatches allocs per call: want 0, got %v", allocs)
	}
}
