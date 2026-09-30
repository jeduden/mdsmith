package requiredstructure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkCheckPathPatterns pins checkPathPatterns' per-file cost.
// r.PathPatterns' Pattern strings pass doublestar.ValidatePattern once,
// at config-parse time (parsePathPatterns), yet checkPathPatterns called
// doublestar.Match — which re-validates its pattern argument on every
// call — for every file assigned a kind with path-pattern: configured.
// See docs/development/high-performance-go.md "Skip work you don't need".
func BenchmarkCheckPathPatterns(b *testing.B) {
	root := b.TempDir()
	relPath := "docs/guides/install.md"
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		b.Fatal(err)
	}
	source := []byte("# Install\n")
	if err := os.WriteFile(abs, source, 0o644); err != nil {
		b.Fatal(err)
	}
	f, err := lint.NewFileFromSource(abs, source, true)
	if err != nil {
		b.Fatal(err)
	}
	f.SetRootDir(root)

	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("plan", "plan/[0-9][0-9]*_*.md"),
		newPathPattern("rfc", "docs/rfc/RFC-*.md"),
		newPathPattern("adr", "docs/adr/*.md"),
	}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.checkPathPatterns(f)
	}
}
