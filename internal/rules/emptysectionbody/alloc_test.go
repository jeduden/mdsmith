package emptysectionbody

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS030 is the per-Check ceiling for this rule on a fixture
// that exercises hasNonBlankLines with a multi-blank-line fenced code block.
// The dominant allocations are topLevelNodes ([]ast.Node slice), headingLabel
// (mdtext.ExtractPlainText + strings.Repeat + concatenation), and the
// diagnostic fmt.Sprintf — not hasNonBlankLines itself, which allocates 0:
// it uses bytes.TrimSpace(seg.Value(source)), which returns a sub-slice of
// the source with no string conversion or copy per line.
// This budget is a regression guard: it fails if new allocations are added.
const allocBudgetMDS030 = 14

// blankCodeBlockFixture is a section whose only body content is a fenced
// code block filled with blank lines.  hasNonBlankLines walks every line
// before returning false, making the string-allocation pattern visible.
var blankCodeBlockFixture = "# Doc\n\n## Empty Section\n\n" +
	"```go\n" + strings.Repeat("\n", 20) + "```\n"

func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}

	src := []byte(blankCodeBlockFixture)
	r := &Rule{MinLevel: 2, MaxLevel: 6, AllowMarker: defaultAllowMarker}

	// Warm: prime any package-level singletons.
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(t, err)
	_ = r.Check(warm)

	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("parse.md", src)
		require.NoError(t, err)
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
		"MDS030 Check allocs/op = %.0f exceeds budget %d: "+
			"new allocations were added to the hot path",
		delta, allocBudgetMDS030)
}
