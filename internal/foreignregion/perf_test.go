package foreignregion

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
)

// TestRestoreAllocBudget pins Restore's per-call allocation cost on a
// moderately large multi-line source. matchedRegionSpans must not
// allocate per line — only the two marker []byte conversions per call
// are budgeted for.
func TestRestoreAllocBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("# T\n\n")
	for i := 0; i < 300; i++ {
		b.WriteString("some out-of-region prose line that is not a marker\n")
	}
	b.WriteString(apm.Start + "\n")
	for i := 0; i < 50; i++ {
		b.WriteString("in-region line   \n")
	}
	b.WriteString(apm.End + "\n")
	for i := 0; i < 100; i++ {
		b.WriteString("more prose outside the region\n")
	}
	original := []byte(b.String())
	fixed := []byte(b.String())
	regions := []config.ForeignRegion{apm}

	const allocBudget = 8

	allocs := testing.AllocsPerRun(20, func() {
		Restore(original, fixed, regions)
	})

	if allocs > allocBudget {
		t.Fatalf("Restore allocs/op = %.1f, want <= %d", allocs, allocBudget)
	}
}
