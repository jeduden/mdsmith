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
// paragraph. Guards a regression back to the string-round-trip form.
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
	// ExtractPlainText's buf.String() copy). After: 6 (walk-closure box,
	// growing []paragraph, sort, and CountWordsInNode's own small
	// bookkeeping). Budget with headroom over the measured post-fix count.
	assert.LessOrEqualf(t, allocs, 10.0,
		"collectParagraphs allocs regressed: got %v, want <= 10", allocs)
}
