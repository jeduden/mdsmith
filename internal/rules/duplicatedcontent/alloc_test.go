package duplicatedcontent

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// multiLineParagraphFixture is a single paragraph reflowed across
// several source lines — the case where extractParagraphs's
// strings.Builder pays repeated regrowth without a Grow() call.
func multiLineParagraphFixture() []byte {
	lines := []string{
		"This is a moderately long sentence that wraps across",
		"many lines in the source file, the way a real paragraph",
		"does once a reflow rule has run over it and broken it",
		"into several lines instead of one long unbroken line.",
	}
	return []byte("# Heading\n\n" + strings.Join(lines, "\n") + "\n")
}

// extractParagraphsAllocBudget pins extractParagraphs's allocation
// count on a multi-line paragraph. The per-paragraph strings.Builder
// used to start at zero capacity and grow on every appended line
// segment; presizing it once via Grow (the segment lengths are known
// before any byte is written) avoids that regrowth — see
// docs/development/high-performance-go.md "Reuse loop-local
// buffers" / "Pre-size slices". Measured baseline after the fix: 6
// allocs/op (down from 8) on this 4-line paragraph.
const extractParagraphsAllocBudget = 6

func TestExtractParagraphsAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	f, err := lint.NewFile("p.md", multiLineParagraphFixture())
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(50, func() {
		_ = extractParagraphs(f, 10)
	})
	t.Logf("extractParagraphs allocs/op = %.0f (budget = %d)",
		allocs, extractParagraphsAllocBudget)
	if allocs > float64(extractParagraphsAllocBudget) {
		t.Fatalf("extractParagraphs allocs/op = %.0f, budget = %d; see "+
			"docs/development/high-performance-go.md",
			allocs, extractParagraphsAllocBudget)
	}
}
