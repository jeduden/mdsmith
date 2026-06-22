package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/stretchr/testify/require"
)

// corpusRuns extracts every inline-bearing run's bytes from the repository's
// own parse-skip-eligible Markdown, grouping lines exactly as scanInlineBlocks
// does (skip set plus skipInlineLine). It restricts the file set to the
// population the production parse-skip gate actually admits — no fenced/indented
// code block and no `<?` directive marker, mirroring runner.layer0SkipEligible
// and TestInlineIndexEquivalence_* — so the scanner hit-rate and the
// scanner-vs-goldmark comparison measure the input that drives the default-on
// decision, without MDSMITH_SPIKE_CORPUS.
func corpusRuns(t testing.TB) [][]byte {
	root := repoRoot(t)
	var runs [][]byte
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		src, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		_, body := StripFrontMatter(src)
		if SourceMayHaveCodeBlock(body) || bytes.Contains(body, []byte("<?")) {
			return nil
		}
		f := NewFileLines(p, body)
		runs = append(runs, fileRuns(f)...)
		return nil
	})
	return runs
}

// fileRuns returns one byte slice per inline-bearing run of f, delegating the
// run-grouping to inlineRunBounds so the benchmark measures the same partition
// that scanInlineBlocks produces.
func fileRuns(f *File) [][]byte {
	bounds := inlineRunBounds(f)
	if len(bounds) == 0 {
		return nil
	}
	runs := make([][]byte, len(bounds))
	for i, b := range bounds {
		runs[i] = f.Source[b[0]:b[1]]
	}
	return runs
}

// TestCorpusRunEligibility reports the scanner hit-rate over the repository
// corpus: how many inline-bearing runs the byte scanner handles without
// falling back to goldmark. It is a measurement, not a strict gate — it only
// asserts the corpus is non-empty and that a meaningful fraction is eligible,
// the precondition for the scanner to be a net win.
func TestCorpusRunEligibility(t *testing.T) {
	runs := corpusRuns(t)
	require.NotEmpty(t, runs)

	var eligible, scanned int
	a := arena.New()
	for _, run := range runs {
		if scanRunEligible(run) {
			eligible++
		}
		if _, ok := scanInlineRun(run, a); ok {
			scanned++
		}
	}
	pctEligible := 100 * float64(eligible) / float64(len(runs))
	pctScanned := 100 * float64(scanned) / float64(len(runs))
	t.Logf("corpus runs=%d eligible=%d (%.1f%%) scanned-ok=%d (%.1f%%)",
		len(runs), eligible, pctEligible, scanned, pctScanned)

	require.Greater(t, pctScanned, 20.0,
		"scanner should handle a meaningful fraction of corpus runs")
}

// BenchmarkScanInlineRun_Eligible measures the byte scanner on the runs it
// actually handles (scanner-eligible, scanInlineRun returns ok). This is the
// per-run cost the parse-skip path pays instead of a goldmark parse for those
// runs.
func BenchmarkScanInlineRun_Eligible(b *testing.B) {
	runs := eligibleRuns(b)
	require.NotEmpty(b, runs)
	a := arena.New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%len(runs) == 0 {
			a.Reset()
		}
		_, _ = scanInlineRun(runs[i%len(runs)], a)
	}
}

// BenchmarkParseInline_Eligible measures the goldmark parse on the same
// scanner-eligible runs, so the two benchmarks are directly comparable: the
// scanner's speedup on the runs it removes from the goldmark path.
func BenchmarkParseInline_Eligible(b *testing.B) {
	runs := eligibleRuns(b)
	require.NotEmpty(b, runs)
	a := arena.New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%len(runs) == 0 {
			a.Reset()
		}
		_ = parseInlineWithRefsArena(runs[i%len(runs)], nil, a)
	}
}

// BenchmarkInlineRunNode_AllRuns measures inlineRunNode over the full corpus
// run set (scanner-first with goldmark fallback) — the production per-run cost
// of the parse-skip path with the scanner in place.
func BenchmarkInlineRunNode_AllRuns(b *testing.B) {
	runs := corpusRuns(b)
	require.NotEmpty(b, runs)
	a := arena.New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%len(runs) == 0 {
			a.Reset()
		}
		_ = inlineRunNode(runs[i%len(runs)], nil, a)
	}
}

// BenchmarkParseInline_AllRuns measures the goldmark parse over the full
// corpus run set — the pre-scanner baseline (every run parsed by goldmark).
func BenchmarkParseInline_AllRuns(b *testing.B) {
	runs := corpusRuns(b)
	require.NotEmpty(b, runs)
	a := arena.New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%len(runs) == 0 {
			a.Reset()
		}
		_ = parseInlineWithRefsArena(runs[i%len(runs)], nil, a)
	}
}

// eligibleRuns returns the corpus runs the scanner handles (scanInlineRun
// returns ok), the subset both eligible benchmarks iterate.
func eligibleRuns(t testing.TB) [][]byte {
	a := arena.New()
	var out [][]byte
	for _, run := range corpusRuns(t) {
		if _, ok := scanInlineRun(run, a); ok {
			out = append(out, run)
		}
	}
	return out
}
