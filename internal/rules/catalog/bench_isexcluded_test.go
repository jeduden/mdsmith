package catalog

import "testing"

// BenchmarkIsExcluded pins isExcluded's per-candidate cost. Each glob
// match found by a catalog directive's include pattern is checked
// against every configured exclude pattern; for a repository with
// several exclude: patterns, most (pattern, candidate) pairs mismatch —
// the case where doublestar.Match's internal re-validation step fires
// on every call. See docs/development/high-performance-go.md "Skip
// work you don't need".
func BenchmarkIsExcluded(b *testing.B) {
	patterns := []string{
		"vendor/**",
		"node_modules/**",
		"**/*.generated.md",
		"docs/internal/**",
	}
	candidate := "docs/development/high-performance-go.md"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if isExcluded(candidate, patterns) {
			b.Fatal("expected candidate to match none of the exclude patterns")
		}
	}
}
