package nobareurls

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// fixAllocFixture mixes mostly prose paragraphs (no URL at all, the
// common case in real documents) with a handful of bare-URL
// paragraphs, across a multi-KB document — large enough for missing
// presizing of Fix's result buffer to show up, and skewed enough for
// the per-text-node regex gate to matter.
func fixAllocFixture() []byte {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("Some prose text padding out the paragraph length here ")
		b.WriteString("with a few more words so the node is a realistic size.\n\n")
		if i%8 == 0 {
			b.WriteString("See https://example.com/path/to/resource for more info.\n\n")
		}
	}
	return []byte(b.String())
}

// fixAllocBudgetMDS012 pins Rule.Fix's allocation count on
// fixAllocFixture. Two issues compounded here:
//
//  1. `result []byte` started at zero capacity and grew via append on
//     every replacement segment and every literal byte in between;
//     presizing it to len(f.Source) plus the replacements' `<`/`>`
//     overhead (computable before the first append) avoids that
//     regrowth.
//  2. The AST walk ran urlPattern.FindAllIndex (a regexp call) on
//     every text node, including the prose nodes that never contain a
//     URL. flagTextNode (the Check path) already gates this behind a
//     bytes.Contains(content, urlNeedle) literal check; Fix's walk
//     did not share that gate.
//
// See docs/development/high-performance-go.md "Pre-size slices" and
// "Gate expensive analyzers behind a cheap pre-check". Measured
// baseline after both fixes: 15 allocs/op (down from 21) on this
// 40-paragraph, 5-URL fixture.
const fixAllocBudgetMDS012 = 15

func TestFixAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	src := fixAllocFixture()
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}

	allocs := testing.AllocsPerRun(50, func() {
		_ = r.Fix(f)
	})
	t.Logf("Fix allocs/op = %.0f (budget = %d)", allocs, fixAllocBudgetMDS012)
	require.LessOrEqualf(t, allocs, float64(fixAllocBudgetMDS012),
		"Fix allocs/op = %.0f, budget = %d; see "+
			"docs/development/high-performance-go.md",
		allocs, fixAllocBudgetMDS012)
}
