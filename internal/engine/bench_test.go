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

// checkCorpusFiles and checkCorpusLines size the synthetic workspace
// the performance gate lints. They are deliberately fixed so the
// budget is meaningful across runs.
const (
	checkCorpusFiles = 300
	checkCorpusLines = 150
)

// BenchmarkCheckCorpus is the `mdsmith check` performance gate,
// modelled on internal/lsp/bench_test.go. It lints a 300-file
// synthetic workspace with the full production rule set and fails
// when p95 wall time exceeds the budget. The budget is generous
// (CI runners are shared and slow); it catches order-of-magnitude
// regressions, not micro-noise. p95 is also reported as a metric
// so trends stay visible in the job log.
//
// Local baseline (4-core dev box, 2026-05): p95 ~1.2 s. The 6 s
// budget is ~5x headroom — it trips on an order-of-magnitude
// regression (an accidental O(n^2), a per-file network call),
// not on shared-runner jitter.
func BenchmarkCheckCorpus(b *testing.B) {
	benchCheck(b, checkCorpusFiles, checkCorpusLines, 6*time.Second)
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
		if err := os.WriteFile(p, []byte(buildCorpusDoc(i, lines)), 0o644); err != nil {
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
func buildCorpusDoc(idx, lines int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Document %d\n\n", idx)
	for i := 0; i < lines; i++ {
		switch {
		case i%25 == 0:
			fmt.Fprintf(&b, "## Section %d\n\n", i/25)
		case i%17 == 0:
			b.WriteString("```go\nfunc f() int { return 0 }\n```\n\n")
		case i%11 == 0:
			fmt.Fprintf(&b, "See [the next doc](doc%03d.md) for details.\n\n", (idx+1)%checkCorpusFiles)
		default:
			b.WriteString("This is a synthetic sentence used to exercise " +
				"the prose and structure rules under benchmark.\n\n")
		}
	}
	return b.String()
}
