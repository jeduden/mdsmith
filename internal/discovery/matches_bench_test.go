package discovery

import "testing"

// matchesAnyBenchPatterns mirrors a realistic .mdsmith.yml glob:
// list — several patterns, none of which match the probed path, so
// matchesAny walks every pattern before giving up (the worst case,
// and the common case for a file discovery walk over a large tree
// where most patterns target other subdirectories).
var matchesAnyBenchPatterns = []string{
	"docs/**/*.md",
	"!docs/research/**",
	"!docs/security/**",
	"plan/*.md",
	"internal/**/*.md",
	"*.md",
}

// BenchmarkMatchesAny exercises walker.matchesAny on a path several
// directories deep that matches none of the configured patterns —
// benchstat-friendly (no assertion), consumed by
// TestMatchesAny_NoRevalidationBudget below for the enforced gate.
func BenchmarkMatchesAny(b *testing.B) {
	w := &walker{patterns: matchesAnyBenchPatterns}
	rel := "cmd/mdsmith/internal/tooling/deeply/nested/file.go"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.matchesAny(rel)
	}
}
