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
