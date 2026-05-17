package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/rule"

	// Production rule set, so the gate measures what `mdsmith check`
	// actually runs — not a stripped test subset.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// checkCorpusLines fixes the per-file size so a budget is
// comparable across runs; only the file count varies by tier.
const checkCorpusLines = 150

// The `mdsmith check` performance gate is tiered, mirroring
// internal/lsp/bench_test.go's 1k/5k split. Two budgets catch two
// different regressions:
//
//   - Small (60 files) — per-file fixed overhead. A regression in
//     startup, config resolution, or rule registration shows here
//     even though the absolute time is tiny.
//   - Large (600 files) — scaling. A superlinear regression (an
//     accidental O(n^2) over the workspace, a per-file rescan)
//     barely moves Small but blows Large past its budget.
//
// Budgets are generous (~15-20x the local baseline) so shared CI
// jitter does not flake them; they trip on order-of-magnitude
// regressions, not micro-noise. p95 and per-file cost are reported
// as metrics so trends stay visible in the job log.
//
// Local baseline (4-core dev box, 2026-05, after the plan-175
// LineOfOffset line-index and MDS024 tokenizer-skip fixes):
// Small p95 ~0.09 s, Large p95 ~0.8 s.
func BenchmarkCheckCorpusSmall(b *testing.B) {
	benchCheck(b, 60, checkCorpusLines, 2*time.Second)
}

func BenchmarkCheckCorpusLarge(b *testing.B) {
	benchCheck(b, 600, checkCorpusLines, 12*time.Second)
}

func benchCheck(b *testing.B, files, lines int, budget time.Duration) {
	b.Helper()
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}

	dir := b.TempDir()
	paths := make([]string, 0, files)
	for i := 0; i < files; i++ {
		p := filepath.Join(dir, fmt.Sprintf("doc%03d.md", i))
		if err := os.WriteFile(p, []byte(buildCorpusDoc(i, lines, files)), 0o644); err != nil {
			b.Fatalf("write corpus file: %v", err)
		}
		paths = append(paths, p)
	}

	cfg := config.Defaults()
	newRunner := func() *engine.Runner {
		return &engine.Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          dir,
			// The gate discards the Result, so the per-diagnostic
			// source window is pure allocation here; skipping it
			// measures rule CPU, not SourceLines string copies.
			SkipSourceContext: true,
		}
	}
	// Warm one run so first-touch allocations and the rule
	// registry are not charged to the first sample.
	_ = newRunner().Run(paths)

	samples := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_ = newRunner().Run(paths)
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()

	if len(samples) == 0 {
		b.Skip("no samples — benchmark needs more iterations")
	}
	p95 := percentileDur(samples, 0.95)
	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	b.ReportMetric(float64(p95.Microseconds())/float64(files), "us_per_file")
	if p95 > budget {
		b.Fatalf("check p95 %v exceeds budget %v for %d-file corpus", p95, budget, files)
	}
}

func percentileDur(samples []time.Duration, q float64) time.Duration {
	cp := append([]time.Duration(nil), samples...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)-1) * q)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

// buildCorpusDoc emits a syntactically valid but rule-exercising
// Markdown file: headings, prose, a fenced code block, a link, and
// a table, so the lint pass touches a representative rule spread
// rather than a trivial one-line file.
func buildCorpusDoc(idx, lines, total int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Document %d\n\n", idx)
	for i := 0; i < lines; i++ {
		switch {
		case i%25 == 0:
			fmt.Fprintf(&b, "## Section %d\n\n", i/25)
		case i%17 == 0:
			b.WriteString("```go\nfunc f() int { return 0 }\n```\n\n")
		case i%11 == 0:
			fmt.Fprintf(&b, "See [the next doc](doc%03d.md) for details.\n\n", (idx+1)%total)
		default:
			b.WriteString("This is a synthetic sentence used to exercise " +
				"the prose and structure rules under benchmark.\n\n")
		}
	}
	return b.String()
}

// BenchmarkCheckCorpusLargeAlwaysDedupe is a control variant that forces the
// unconditional DedupeDiagnostics path for comparison against the skip-when-
// safe path in BenchmarkCheckCorpusLarge. Only run manually to measure the
// allocation delta from plan 183; not part of the standing CI gate.
func BenchmarkCheckCorpusLargeAlwaysDedupe(b *testing.B) {
	if !testing.Verbose() {
		b.Skip("control benchmark: run with -v to measure vs BenchmarkCheckCorpusLarge")
	}
	b.Helper()
	const files, lines = 600, 150
	dir := b.TempDir()
	paths := make([]string, 0, files)
	for i := 0; i < files; i++ {
		p := filepath.Join(dir, fmt.Sprintf("doc%03d.md", i))
		if err := os.WriteFile(p, []byte(buildCorpusDoc(i, lines, files)), 0o644); err != nil {
			b.Fatalf("write corpus file: %v", err)
		}
		paths = append(paths, p)
	}
	cfg := config.Defaults()
	newRunner := func() *engine.Runner {
		return &engine.Runner{
			Config: cfg, Rules: rule.All(),
			StripFrontMatter: true, RootDir: dir,
			SkipSourceContext: true,
		}
	}
	_ = newRunner().Run(paths)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := newRunner().Run(paths)
		// Force the allocation that the skip path avoids.
		_ = engine.DedupeDiagnostics(res.Diagnostics)
	}
}
