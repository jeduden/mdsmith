package markdownflavor

import (
	"strconv"
	"strings"
	"testing"
)

// allocBudgetFixGitHubAlerts is the per-call ceiling for fixGitHubAlerts
// itself (called directly, not through Fix, which always re-parses the
// stripped result afterward — a legitimate, unrelated allocation this
// budget must not have to absorb) on a file whose alert blockquote spans
// many lines. fixGitHubAlerts used to build its result line-by-line in
// an unpresized []string (only the marker line is skipped, so the final
// size is knowable up front — see docs/development/high-performance-go.md
// "Pre-size slices") joined via strings.Join (one string() copy per
// line plus the join), and buildAlertSkipMaps converted every
// continuation line to a string just to test its first byte. Both are
// now byte-native. Baseline measured 10 on this mixed prefixed/lazy-
// continuation fixture (the buf.Grow estimate under-shoots once
// addPrefix lines are present, so one re-grow is expected — see the
// comment on the buf.Grow call in fixGitHubAlerts); +4 headroom follows
// the project's "baseline plus max(20%, 4)" convention so an unrelated
// +1 doesn't turn CI red.
const allocBudgetFixGitHubAlerts = 14

// fixGitHubAlertsFixture is a single alert blockquote with many content
// lines, so both the unpresized `out` slice and the per-line string()
// conversion in buildAlertSkipMaps would have scaled with line count.
// Every fourth line drops the "> " prefix (a lazy continuation), so the
// fixture also exercises fixGitHubAlerts' addPrefix rewrite branch
// instead of only the pass-through branch.
func fixGitHubAlertsFixture(lines int) string {
	var b strings.Builder
	b.WriteString("> [!NOTE]\n")
	for i := 0; i < lines; i++ {
		if i%4 == 3 {
			b.WriteString("content line " + strconv.Itoa(i) + "\n")
		} else {
			b.WriteString("> content line " + strconv.Itoa(i) + "\n")
		}
	}
	return b.String()
}

// TestFixGitHubAlertsAllocBudget pins fixGitHubAlerts' own per-call
// allocation count on a many-line alert blockquote, calling it directly
// so Fix's unrelated re-parse pass does not swamp the measurement.
func TestFixGitHubAlertsAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	src := fixGitHubAlertsFixture(63)
	r := &Rule{}

	f := mkFile(t, src)
	got := r.fixGitHubAlerts(f) // warm up any cached state
	if !strings.Contains(string(got), "> content line 3\n") {
		t.Fatalf("fixGitHubAlerts did not re-add the \"> \" prefix on a lazy-continuation line:\n%s", got)
	}

	const runs = 50
	allocs := testing.AllocsPerRun(runs, func() {
		_ = r.fixGitHubAlerts(f)
	})
	t.Logf("fixGitHubAlerts allocs/op = %.0f (budget = %d)", allocs, allocBudgetFixGitHubAlerts)
	if allocs > float64(allocBudgetFixGitHubAlerts) {
		t.Fatalf("fixGitHubAlerts allocs/op = %.0f, budget = %d: the per-line result must be "+
			"built via a presized buffer, not an unpresized []string + strings.Join, and the "+
			"continuation-line scan in buildAlertSkipMaps must not copy each line to a string",
			allocs, allocBudgetFixGitHubAlerts)
	}
}
