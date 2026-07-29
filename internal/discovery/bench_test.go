package discovery

import "testing"

// BenchmarkWalker_matchesAny pins matchesAny's per-file cost. Patterns
// reaching walker.patterns have already passed doublestar.ValidatePattern
// once in validatePatterns. doublestar.Match's internal validation check
// only fires on the path where matching reaches the end of the candidate
// name before reaching the end of the pattern — i.e. on a rejection, the
// common case when walking a tree against a handful of configured
// patterns, most of which do not apply to most files.
// doublestar.MatchUnvalidated skips that check. See
// docs/development/high-performance-go.md "Skip work you don't need".
func BenchmarkWalker_matchesAny(b *testing.B) {
	w := &walker{
		patterns: []string{
			"internal/**/*.go",
			"pkg/**/*.go",
			"cmd/**/*.go",
			"**/*.proto",
		},
	}
	// Fails every configured pattern — the common case during a
	// workspace walk, where most files do not match most patterns.
	rel := "docs/development/high-performance-go.md"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if w.matchesAny(rel) {
			b.Fatal("expected rel to match none of the configured patterns")
		}
	}
}
