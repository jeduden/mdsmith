package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/lint"
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
// Each tier carries TWO hard budgets, both enforced as
// `b.Fatalf` on every run:
//
//   - p95 wall time, sized at ~3-5x the local baseline so CI
//     jitter does not flake but a real perf regression does.
//   - Median allocs/op, sized at ~15-20% headroom over the
//     current measured count. Allocations are CPU-independent,
//     so a tight alloc gate catches algorithmic regressions
//     (extra parse-per-file, lost memoization) the wall-time
//     budget would have to budge for.
//
// Both numbers report as metrics (`p95_ms`, `allocs_per_op`,
// `us_per_file`) so trends stay visible in the job log.
//
// Local baseline (4-core dev box, 2026-05-22, after plan 196's
// lazy SectionParagraph text — paragraph-readability's minWords
// gate no longer materialises text for paragraphs below the floor):
//
//   - Small p95 ~27 ms / ~57 k allocs/op
//   - Large p95 ~191 ms / ~553 k allocs/op
//
// Plan 195's baseline was ~65 k / ~634 k; the lazy-extract chunk
// drops the synthetic-corpus paragraph allocator by ~80 k on the
// large bench, since most of its paragraphs (the 13-word
// "synthetic sentence …" body) fall under default minWords=20.
//
// Budgets sit at ~15-20% headroom over the measured count so a
// real algorithmic regression (an extra parse per file, a lost
// memoization slot, a closure re-escaped to the heap) crosses the
// alloc ceiling on the first run while CI jitter does not.
func BenchmarkCheckCorpusSmall(b *testing.B) {
	benchCheck(b, 60, checkCorpusLines, checkBudget{
		Time:   250 * time.Millisecond,
		Allocs: 70_000,
	})
}

func BenchmarkCheckCorpusLarge(b *testing.B) {
	benchCheck(b, 600, checkCorpusLines, checkBudget{
		Time:   2 * time.Second,
		Allocs: 670_000,
	})
}

// checkBudget pairs the two hard gates BenchmarkCheckCorpus* enforce:
// p95 wall time and median allocs/op. Bundled so a future tier
// addition cannot forget either limit.
type checkBudget struct {
	Time   time.Duration
	Allocs uint64
}

// BenchmarkCheckCorpusFewFiles exercises the small-file-count path
// that the intra-file rule parallelism (plan 190) is designed for:
// a 5-file batch leaves most cores idle for the file-level pool, so
// the inner pool fills the gap. The budget is generous because the
// per-file cost dominates (~5 files = 5x the per-file fixed overhead
// of Small, no scaling beyond that). Not part of the standing
// budget gate.
func BenchmarkCheckCorpusFewFiles(b *testing.B) {
	benchCheck(b, 5, checkCorpusLines, checkBudget{
		Time:   1 * time.Second,
		Allocs: 10_000,
	})
}

// BenchmarkCheckCorpusFewFilesNoIntraFile is the control variant for
// the few-files benchmark with intra-file parallelism disabled. Run
// alongside BenchmarkCheckCorpusFewFiles to measure how much the
// inner pool saves when the outer pool already has worker headroom.
// Manual benchmark only; not part of the standing CI gate.
func BenchmarkCheckCorpusFewFilesNoIntraFile(b *testing.B) {
	benchCheckCapped(b, 5, checkCorpusLines, 1*time.Second, 1)
}

func benchCheckCapped(b *testing.B, files, lines int, budget time.Duration, intraCap int) {
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
			Config:               cfg,
			Rules:                rule.All(),
			StripFrontMatter:     true,
			RootDir:              dir,
			SkipSourceContext:    true,
			IntraFileConcurrency: intraCap,
		}
	}
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
		b.Fatalf("check p95 %v exceeds budget %v for %d-file corpus (cap=%d)", p95, budget, files, intraCap)
	}
}

func benchCheck(b *testing.B, files, lines int, budget checkBudget) {
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
	allocSamples := make([]uint64, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var ms1, ms2 runtime.MemStats
		runtime.ReadMemStats(&ms1)
		start := time.Now()
		_ = newRunner().Run(paths)
		samples = append(samples, time.Since(start))
		runtime.ReadMemStats(&ms2)
		allocSamples = append(allocSamples, ms2.Mallocs-ms1.Mallocs)
	}
	b.StopTimer()

	if len(samples) == 0 {
		b.Skip("no samples — benchmark needs more iterations")
	}
	p95 := percentileDur(samples, 0.95)
	medianAllocs := percentileUint64(allocSamples, 0.5)
	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	b.ReportMetric(float64(p95.Microseconds())/float64(files), "us_per_file")
	b.ReportMetric(float64(medianAllocs), "allocs_per_op")
	if p95 > budget.Time {
		b.Fatalf("check p95 %v exceeds time budget %v for %d-file corpus",
			p95, budget.Time, files)
	}
	if medianAllocs > budget.Allocs {
		b.Fatalf("check median allocs %d exceeds alloc budget %d for "+
			"%d-file corpus — algorithmic regression suspected (lost "+
			"memoization, extra parse per file, missing early-exit)",
			medianAllocs, budget.Allocs, files)
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

func percentileUint64(samples []uint64, q float64) uint64 {
	cp := append([]uint64(nil), samples...)
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
		_ = lint.DedupeDiagnostics(res.Diagnostics)
	}
}
