package discovery

import "testing"

// BenchmarkMatchesAny measures walker.matchesAny's cost against a
// realistic pattern set (mirrors the multi-pattern globs a project's
// .mdsmith.yml commonly declares) over paths that never match, the
// worst case: every pattern must be tried for every file. Per
// docs/development/high-performance-go.md, validatePatterns already
// runs doublestar.ValidatePattern over w.patterns once up front, so
// matchesAny's per-file, per-pattern match must not re-validate.
func BenchmarkMatchesAny(b *testing.B) {
	w := &walker{
		patterns: validatePatterns([]string{
			"docs/**/*.md",
			"!docs/research/**",
			"!docs/security/**",
			"!docs/brand/**",
			"!**/proto.md",
			"internal/**/*.md",
			"cmd/**/*.md",
			"*.md",
		}),
	}
	rel := "internal/some/deeply/nested/package/file.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.matchesAny(rel)
	}
}
