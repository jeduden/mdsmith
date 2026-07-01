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
// count on a multi-line paragraph. Reworked from a per-paragraph
// strings.Builder pair plus a []byte(normalized) copy before hashing
// (8 allocs/op originally, 6 after a first presizing pass) to two
// scratch []byte buffers presized via slices.Grow from the segment
// lengths and hashed directly with sha256.Sum256 — see
// docs/development/high-performance-go.md "Reuse loop-local
// buffers" / "Pre-size slices". Measured baseline after the fix: 5
// allocs/op on this 4-line paragraph.
const extractParagraphsAllocBudget = 5

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

// manyParagraphsFixture builds 20 paragraphs of 4 lines each, separated
// by blank lines — a realistic multi-paragraph file, the case where a
// per-file reused scratch buffer amortizes to zero allocations per
// paragraph after the first few, unlike a fresh strings.Builder created
// per paragraph.
func manyParagraphsFixture() []byte {
	body := strings.Join([]string{
		"This is a moderately long sentence that wraps across",
		"many lines in the source file, the way a real paragraph",
		"does once a reflow rule has run over it and broken it",
		"into several lines instead of one long unbroken line.",
	}, "\n")
	var b strings.Builder
	b.WriteString("# Heading\n\n")
	for i := 0; i < 20; i++ {
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	return []byte(b.String())
}

// extractParagraphsManyAllocBudget pins extractParagraphs's total
// allocation count across a 20-paragraph file. The pre-byte-native
// pipeline paid a fresh strings.Builder per paragraph (line-gathering),
// a second fresh strings.Builder inside normalize (whitespace/case
// folding), and a []byte(normalized) copy before hashing — three
// allocations every paragraph, none reused across paragraphs even
// though each is at most the size of the largest paragraph in the
// file. Reworking the pipeline to append into two scratch []byte
// buffers reused across the whole file (reset via buf[:0], presized
// via slices.Grow from the segment-length sum) collapses that to
// near-zero allocations per paragraph after the first few. See
// docs/development/high-performance-go.md "Reuse loop-local buffers".
// Measured baseline after the fix: 48 allocs/op (down from 106) on
// this 20-paragraph fixture.
const extractParagraphsManyAllocBudget = 48

func TestExtractParagraphsManyAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	f, err := lint.NewFile("many.md", manyParagraphsFixture())
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(20, func() {
		_ = extractParagraphs(f, 10)
	})
	t.Logf("extractParagraphs allocs/op (20 paragraphs) = %.0f (budget = %d)",
		allocs, extractParagraphsManyAllocBudget)
	if allocs > float64(extractParagraphsManyAllocBudget) {
		t.Fatalf("extractParagraphs allocs/op = %.0f, budget = %d; see "+
			"docs/development/high-performance-go.md",
			allocs, extractParagraphsManyAllocBudget)
	}
}
