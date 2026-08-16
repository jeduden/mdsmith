package catalog

import (
	"fmt"
	"testing"
)

// buildNumericEntries returns n fileEntry values with a numeric "id"
// field in reverse order, so sortEntries has real work to do.
func buildNumericEntries(n int) []fileEntry {
	entries := make([]fileEntry, n)
	for i := 0; i < n; i++ {
		id := n - i
		entries[i] = fileEntry{fields: map[string]any{
			"filename": fmt.Sprintf("file_%05d.md", id),
			"id":       id,
		}}
	}
	return entries
}

// BenchmarkSortEntriesNumeric measures sortEntries in numeric mode.
// Per docs/development/high-performance-go.md's "memoize per-input
// computations" pattern, each entry's sort value should be parsed once
// (allParseAsInt already parses every entry once just to decide
// useInts) rather than re-parsed by fieldinterp.ParseCUEPath on every
// comparator call during the O(n log n) sort.
func BenchmarkSortEntriesNumeric(b *testing.B) {
	entries := buildNumericEntries(5000)
	work := make([]fileEntry, len(entries))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(work, entries)
		sortEntries(work, "id", false, true)
	}
}
