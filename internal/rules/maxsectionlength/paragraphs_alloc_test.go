package maxsectionlength

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// manyParagraphsSource builds a document with n short paragraphs so
// collectParagraphs materializes n word counts per call.
func manyParagraphsSource(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("A short prose paragraph for counting words here.\n\n")
	}
	return b.String()
}

// TestCollectParagraphs_SkipsTextExtraction pins
// docs/development/high-performance-go.md's "skip work you don't
// need": collectParagraphs only needs each paragraph's word count, not
// its text, so it must use mdtext.CountWordsInNode (which counts
// directly off the AST) instead of ExtractPlainText(...) +
// CountWords(...), which materializes and then discards a string per
// paragraph. It must also reuse astutil.CollectSectionParagraphs's
// per-File memoized walk instead of re-walking the AST itself.
// Guards a regression back to either the string-round-trip form or a
// second walk.
func TestCollectParagraphs_SkipsTextExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	f := mustFile(t, manyParagraphsSource(30))

	allocs := testing.AllocsPerRun(50, func() {
		collectParagraphs(f)
	})
	// Baseline before this fix: 36 allocs on 30 paragraphs (~1/paragraph,
	// ExtractPlainText's buf.String() copy, on top of collectParagraphs'
	// own ast.Walk). After: 1 (the presized []paragraph result itself —
	// AllocsPerRun reuses the same *lint.File across all 50 runs, so the
	// underlying walk warms astutil's memo on the first call and every
	// later call is a cache hit). Budget with headroom over that.
	assert.LessOrEqualf(t, allocs, 4.0,
		"collectParagraphs allocs regressed: got %v, want <= 4", allocs)
}
