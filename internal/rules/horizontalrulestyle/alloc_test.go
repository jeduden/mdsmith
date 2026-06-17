package horizontalrulestyle

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS044Fix is the per-Fix() ceiling for a 30-line file
// containing one horizontal rule that needs replacement. The
// []string + strings.Join pattern allocates one string per unchanged
// line; [][]byte + bytes.Join avoids those per-line allocations.
const allocBudgetMDS044Fix = 15

// hrFixAllocFixture is a 30-line file with one five-dash horizontal rule
// that the rule would normalise to three dashes. All other lines are
// prose or headings and pass through Fix unchanged — maximising the
// share of allocs from per-line string conversions.
var hrFixAllocFixture = strings.Join([]string{
	"# Title",
	"",
	"Section one paragraph one.",
	"Section one paragraph two.",
	"",
	"Section one paragraph three.",
	"Section one paragraph four.",
	"",
	"## Section Two",
	"",
	"Paragraph one.",
	"Paragraph two.",
	"",
	"Paragraph three.",
	"Paragraph four.",
	"",
	"",
	"-----",
	"",
	"",
	"Paragraph after rule.",
	"",
	"## Section Three",
	"",
	"Line one.",
	"Line two.",
	"Line three.",
	"",
	"Final line.",
	"",
}, "\n")

func TestFixAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}

	src := []byte(hrFixAllocFixture)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{Style: "dash", Length: 3, RequireBlankLines: false}
	// Warm: prime any cached state on f.
	_ = r.Fix(f)

	const runs = 100
	allocs := testing.AllocsPerRun(runs, func() {
		_ = r.Fix(f)
	})
	t.Logf("MDS044 Fix allocs/op = %.0f (budget = %d)", allocs, allocBudgetMDS044Fix)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS044Fix),
		"MDS044 Fix allocs/op = %.0f exceeds budget %d: "+
			"Fix must use [][]byte+bytes.Join, not []string+strings.Join, "+
			"to avoid allocating one string per source line",
		allocs, allocBudgetMDS044Fix)
}
