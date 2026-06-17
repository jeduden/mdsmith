package emptysectionbody

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS030 is the per-Check ceiling for empty-section-body.
// Two optimizations reduce the alloc count:
//   - topLevelNodes pre-sizes its slice to cap 8, saving ~3 backing-array
//     growth allocations for a typical document with 8 top-level nodes.
//   - allowMarkerDirective is precomputed in the Rule struct so Check does
//     not call fmt.Sprintf on the directive string per violation.
const allocBudgetMDS030 = 3

const allocBudgetFixture = "# Document title\n" +
	"\n" +
	"A short prose paragraph for the readability and structural\n" +
	"rules to scan. It stays one paragraph long.\n" +
	"\n" +
	"## Section\n" +
	"\n" +
	"See [other](other.md) and [label][ref] for examples.\n" +
	"\n" +
	"```go\nfunc f() int { return 0 }\n```\n" +
	"\n" +
	"- one item\n" +
	"- two items\n" +
	"\n" +
	"| Col | Other |\n" +
	"|-----|-------|\n" +
	"| a   | b     |\n" +
	"\n" +
	"[ref]: https://example.com/\n"

func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	src := []byte(allocBudgetFixture)
	r := &Rule{
		MinLevel:             defaultMinLevel,
		MaxLevel:             defaultMaxLevel,
		AllowMarker:          defaultAllowMarker,
		allowMarkerDirective: "<?" + defaultAllowMarker + "?>",
	}
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(t, err)
	_ = r.Check(warm)

	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, _ = lint.NewFile("parse.md", src)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("check.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	t.Logf("MDS030 Check allocs/op = %.0f (budget = %d)", delta, allocBudgetMDS030)
	require.LessOrEqualf(t, delta, float64(allocBudgetMDS030),
		"MDS030 Check allocs/op = %.0f, budget = %d",
		delta, allocBudgetMDS030)
}
