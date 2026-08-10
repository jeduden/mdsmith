package flavor

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/pkg/markdown"
)

// BenchmarkDetectReusesPool exercises the dual-parser code path
// repeatedly to confirm the package-shared sync.Pool (borrowed via
// WithSharedParser from dualFindings) avoids rebuilding the
// goldmark parser per call. Each iteration parses a small document
// that triggers the dual pass.
func BenchmarkDetectReusesPool(b *testing.B) {
	src := []byte("# Title {#top}\n\n" +
		"- [ ] task\n\n" +
		"| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
		"~~old~~ text\n")
	doc := markdown.Parse(src)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Detect(doc, nil)
	}
}

// BenchmarkLineColLateOffset exercises LineCol at an offset near the
// end of a large document — the case docs/development/high-performance-go.md
// flags ("bytes.IndexByte ... worth it on big Source scans"): every
// finding late in a large file rescans from byte 0 to its own offset,
// so this offset shape is where a manual byte loop costs the most
// relative to bytes.Count/bytes.LastIndexByte's SIMD-accelerated scan.
//
// Baseline (2026-07-11, 5000-line / ~150KB source, offset near EOF,
// manual byte-by-byte loop): ~45.1µs/op. After switching to
// bytes.Count/bytes.LastIndexByte: ~2.4µs/op (-94.7%, benchstat
// p=0.008).
//
// The gate below takes the MEDIAN of many small timed rounds rather
// than one b.N-wide average: this sandbox's scheduler occasionally
// stalls the whole process for a few milliseconds mid-benchmark, which
// would blow out a single-average budget on pure noise. The median
// across roundCount independent rounds ignores that tail as long as
// most rounds stay clean, while still catching a real regression
// (every round would be slow, not just one).
const (
	lineColLateOffsetBudget   = 8 * time.Microsecond
	lineColLateOffsetRounds   = 200
	lineColLateOffsetPerRound = 100
)

func BenchmarkLineColLateOffset(b *testing.B) {
	src := []byte(strings.Repeat("some line of prose text here\n", 5000))
	offset := len(src) - 10
	_, _ = LineCol(src, offset) // warm the page cache before timing

	samples := make([]time.Duration, lineColLateOffsetRounds)
	for i := range samples {
		start := time.Now()
		for j := 0; j < lineColLateOffsetPerRound; j++ {
			_, _ = LineCol(src, offset)
		}
		samples[i] = time.Since(start) / lineColLateOffsetPerRound
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median := samples[len(samples)/2]
	b.ReportMetric(float64(median.Nanoseconds()), "ns/op_median")
	if median > lineColLateOffsetBudget {
		b.Fatalf("LineCol late-offset median = %s/op, budget %s — see docs/development/high-performance-go.md",
			median, lineColLateOffsetBudget)
	}
}

// BenchmarkBareURLNoMatches exercises BareURLFindingsInTree over prose
// with no bare URLs at all — the common case. Before gating
// bareURLPattern.FindAllIndex behind a cheap bytes.Contains(body,
// "://") pre-check (docs/development/high-performance-go.md "gate
// expensive analyzers behind a cheap pre-check"), the regex engine ran
// unconditionally on every Text AST node's segment.
//
// The gate below takes the MEDIAN of many small timed rounds rather
// than one b.N-wide average, matching BenchmarkLineColLateOffset's
// rationale: this sandbox's scheduler occasionally stalls the whole
// process for a few milliseconds mid-benchmark, which would blow out a
// single-average budget on pure noise.
//
// Baseline (2026-08-10, 200-paragraph prose doc with zero bare URLs,
// unconditional regex per Text node): ~607µs/op median. After adding
// the byte-needle gate: ~6-14µs/op median (two orders of magnitude —
// the regex engine never runs when no Text node contains "://").
const (
	bareURLNoMatchesBudget   = 80 * time.Microsecond
	bareURLNoMatchesRounds   = 100
	bareURLNoMatchesPerRound = 20
)

func BenchmarkBareURLNoMatches(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("This paragraph discusses the project roadmap and open questions ")
		sb.WriteString("without linking anywhere at all.\n\n")
	}
	src := []byte(sb.String())
	doc := markdown.Parse(src)

	samples := make([]time.Duration, bareURLNoMatchesRounds)
	for i := range samples {
		start := time.Now()
		for j := 0; j < bareURLNoMatchesPerRound; j++ {
			_ = BareURLFindingsInTree(doc.Body, doc.AST, 0)
		}
		samples[i] = time.Since(start) / bareURLNoMatchesPerRound
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median := samples[len(samples)/2]
	b.ReportMetric(float64(median.Nanoseconds()), "ns/op_median")
	if median > bareURLNoMatchesBudget {
		b.Fatalf("BareURLFindingsInTree no-match median = %s/op, budget %s — see "+
			"docs/development/high-performance-go.md", median, bareURLNoMatchesBudget)
	}
}

// BenchmarkDetectManyFindings exercises Detect end-to-end over a
// document with a custom {#id} heading every few lines — findHeadingID
// resolves each finding's line/col with no regex involved, isolating
// the newline-index win from bareURLPattern's own matching cost (a
// separate, inherent cost covered by BenchmarkBareURLNoMatches).
// Before caching a source's newline offsets once per Detect call,
// every finding's line/col paid LineCol's O(offset) rescan-from-byte-0
// independently (docs/development/high-performance-go.md "memoize
// per-input computations" — the same anti-pattern
// lint.File.LineOfOffset was rewritten to fix), so total cost degraded
// toward O(n^2) as finding count grew with document size.
//
// The gate below takes the MEDIAN of many small timed rounds, matching
// BenchmarkLineColLateOffset's and BenchmarkBareURLNoMatches's
// rationale: this sandbox's scheduler occasionally stalls the whole
// process for a few milliseconds mid-benchmark, which would blow out a
// single-average budget on pure noise.
//
// Baseline (2026-08-10, 1000 {#id} headings, ~50KB source, O(offset)
// rescan per finding): ~4.9ms/op median. After caching the newline
// index once per Detect call: ~2.3-2.5ms/op median. Detect always
// re-parses the whole document through the dual parser regardless of
// finding count, which floors this fixture's achievable median well
// above zero — LineCol no longer appears in a CPU profile of this
// benchmark at all (confirmed via `go tool pprof`), so the remaining
// time is inherent parse cost, not the anti-pattern this benchmark
// guards against.
const (
	detectManyFindingsBudget   = 3500 * time.Microsecond
	detectManyFindingsRounds   = 30
	detectManyFindingsPerRound = 3
)

func BenchmarkDetectManyFindings(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("## Section heading {#heading-x}\n\nSome prose for this section.\n\n")
	}
	src := []byte(sb.String())
	doc := markdown.Parse(src)
	_ = Detect(doc, nil) // warm the page cache before timing

	samples := make([]time.Duration, detectManyFindingsRounds)
	for i := range samples {
		start := time.Now()
		for j := 0; j < detectManyFindingsPerRound; j++ {
			_ = Detect(doc, nil)
		}
		samples[i] = time.Since(start) / detectManyFindingsPerRound
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median := samples[len(samples)/2]
	b.ReportMetric(float64(median.Nanoseconds()), "ns/op_median")
	if median > detectManyFindingsBudget {
		b.Fatalf("Detect many-findings median = %s/op, budget %s — see docs/development/high-performance-go.md",
			median, detectManyFindingsBudget)
	}
}
