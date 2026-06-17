package atxheadingwhitespace

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS064Fix is the per-Fix() ceiling for a 30-line file with
// no heading violations. The []string + strings.Join pattern allocates
// one string per line plus one join; [][]byte + bytes.Join needs only
// the single joined allocation — budget is set below the old per-line cost.
const allocBudgetMDS064Fix = 10

// fixAllocFixture is a 30-line file with correct ATX headings and prose.
// No line has a violation so every line passes unchanged through Fix —
// which makes the per-line string allocation the dominant cost.
var fixAllocFixture = strings.Join([]string{
	"# Title",
	"",
	"## Section One",
	"",
	"Paragraph one line one.",
	"Paragraph one line two.",
	"",
	"## Section Two",
	"",
	"Paragraph two line one.",
	"Paragraph two line two.",
	"Paragraph two line three.",
	"",
	"### Sub Section",
	"",
	"Sub paragraph line one.",
	"Sub paragraph line two.",
	"",
	"## Section Three",
	"",
	"Line one.",
	"Line two.",
	"Line three.",
	"Line four.",
	"Line five.",
	"",
	"## Section Four",
	"",
	"Final paragraph.",
	"",
}, "\n")

func TestFixAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}

	src := []byte(fixAllocFixture)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	// Warm: prime any cached state.
	_ = r.Fix(f)

	const runs = 100
	allocs := testing.AllocsPerRun(runs, func() {
		_ = r.Fix(f)
	})
	t.Logf("MDS064 Fix allocs/op = %.0f (budget = %d)", allocs, allocBudgetMDS064Fix)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS064Fix),
		"MDS064 Fix allocs/op = %.0f exceeds budget %d: "+
			"Fix must use [][]byte+bytes.Join, not []string+strings.Join, "+
			"to avoid allocating one string per source line",
		allocs, allocBudgetMDS064Fix)
}
