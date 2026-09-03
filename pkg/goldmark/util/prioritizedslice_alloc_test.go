package util

import "testing"

// TestPrioritizedSlice_Sort_NoReflectSort pins the allocation cost of
// PrioritizedSlice.Sort, which goldmark's parser and renderer call
// once per parser/renderer instance (guarded by sync.Once, and those
// instances are pool-bounded rather than created per file) to order
// block parsers, inline parsers, paragraph/AST transformers, and node
// renderers by priority. sort.Slice drives reflect.Swapper internally
// — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md — which slices.SortFunc
// avoids by comparing the concrete PrioritizedValue values directly.
func TestPrioritizedSlice_Sort_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	s := PrioritizedSlice{
		{Value: "z", Priority: 30},
		{Value: "a", Priority: 10},
		{Value: "m", Priority: 20},
	}
	s.Sort()
	if s[0].Priority != 10 || s[2].Priority != 30 {
		t.Fatalf("Sort did not order by priority: %v", s)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		s.Sort()
	})
	t.Logf("PrioritizedSlice.Sort allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("PrioritizedSlice.Sort allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
