package blockquotewhitespace

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS042Fix is the per-Fix() ceiling for a 30-line file with
// no blockquote multi-space violations. The []string + strings.Join
// pattern allocates one string per line plus one for the join;
// [][]byte + bytes.Join needs only the single joined allocation.
const allocBudgetMDS042Fix = 10

// bqFixAllocFixture is a 30-line file with no blockquote whitespace
// violations so every line passes through Fix unchanged — making the
// per-line string conversion the dominant allocation cost.
var bqFixAllocFixture = strings.Join([]string{
	"# Title",
	"",
	"> A blockquote line.",
	"> Another blockquote line.",
	"",
	"> A second blockquote.",
	">",
	"> With a blank marker line.",
	"",
	"## Section",
	"",
	"Prose paragraph one.",
	"Prose paragraph two.",
	"",
	"> Third blockquote paragraph.",
	"> Same paragraph continued.",
	"",
	"Prose after blockquote.",
	"",
	"## Another Section",
	"",
	"Line one.",
	"Line two.",
	"Line three.",
	"Line four.",
	"",
	"Line five.",
	"Line six.",
	"",
	"Final line.",
}, "\n")

func TestFixAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}

	src := []byte(bqFixAllocFixture)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	// Warm: prime any cached state on f.
	_ = r.Fix(f)

	const runs = 100
	allocs := testing.AllocsPerRun(runs, func() {
		_ = r.Fix(f)
	})
	t.Logf("MDS042 Fix allocs/op = %.0f (budget = %d)", allocs, allocBudgetMDS042Fix)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS042Fix),
		"MDS042 Fix allocs/op = %.0f exceeds budget %d: "+
			"Fix must use [][]byte+bytes.Join, not []string+strings.Join, "+
			"to avoid allocating one string per source line",
		allocs, allocBudgetMDS042Fix)
}
