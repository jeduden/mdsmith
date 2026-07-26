package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/bytelimit"
)

// writeSyntheticFixCorpus writes n independent Markdown files (no
// cross-links, so orderFilesLeavesFirst's dependency sort is a no-op
// and every measured nanosecond goes to the index build's file reads)
// under dir and returns their absolute paths.
func writeSyntheticFixCorpus(t testing.TB, dir string, n int) []string {
	t.Helper()
	files := make([]string, 0, n)
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, fmt.Sprintf("file_%05d.md", i))
		body := fmt.Sprintf("# Heading %d\n\nSome body text for file %d.\n", i, i)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
		files = append(files, p)
	}
	return files
}

// BenchmarkOrderFilesLeavesFirst measures orderFilesLeavesFirst's cost
// building the dependency index over a workspace-sized file set.
// bytelimit.ReadFileLimited opens its own file handle per call and
// touches no shared state, so the loader is safe for the concurrent
// calls index.Build makes — see docs/development/high-performance-go.md's
// "never do more work than needed" and internal/index's own
// Build-vs-BuildSerial benchmarks for the general case this call site
// specialises.
func BenchmarkOrderFilesLeavesFirst(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	dir := b.TempDir()
	files := writeSyntheticFixCorpus(b, dir, 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_ = orderFilesLeavesFirst(files, dir, bytelimit.DefaultMaxInputBytes)
		b.ReportMetric(float64(time.Since(start).Milliseconds()), "order_ms")
	}
}
