package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

// discoverManyFilesBudget pins docs/development/high-performance-go.md's
// "avoid deprecated filepath.Walk" guidance: filepath.Walk Lstats every
// entry itself even though ReadDir already stat'd it, so every visited
// file or directory pays two syscalls instead of one. Switching Discover
// to filepath.WalkDir (which hands visit the DirEntry ReadDir produced,
// with d.Type()/d.IsDir() served from that same result) cuts the walk to
// one syscall per entry.
//
// Baseline (2026-08-10, 600 files across 30x20 nested directories,
// filepath.Walk): ~1.0-1.5ms/op. After switching to filepath.WalkDir:
// ~0.9-1.1ms/op. Real-filesystem timing is noisier than the in-memory
// benchmarks elsewhere in this repo (page-cache state, tmpfs scheduling),
// so the budget carries generous headroom above the measured median —
// wide enough not to flake, tight enough to catch a revert back to Walk.
const discoverManyFilesBudget = 3 * time.Millisecond

func setupBenchTree(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	for i := 0; i < 30; i++ {
		sub := filepath.Join(dir, "sub", string(rune('a'+i%26)), "nested")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 20; j++ {
			p := filepath.Join(sub, "file"+string(rune('a'+j%26))+".md")
			if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	return dir
}

// BenchmarkDiscoverManyFiles walks a synthetic 600-file tree and gates
// the per-call time so a regression back to filepath.Walk (or any other
// per-entry syscall increase) shows up in `go test -bench`.
func BenchmarkDiscoverManyFiles(b *testing.B) {
	dir := setupBenchTree(b)
	opts := Options{Patterns: []string{"**/*.md"}, BaseDir: dir}
	_, _ = Discover(opts) // warm the page cache before timing
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Discover(opts); err != nil {
			b.Fatal(err)
		}
	}
	perOp := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if perOp > discoverManyFilesBudget {
		b.Fatalf("Discover many-files = %s/op, budget %s — see docs/development/high-performance-go.md",
			perOp, discoverManyFilesBudget)
	}
}
