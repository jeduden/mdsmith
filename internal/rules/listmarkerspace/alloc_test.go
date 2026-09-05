package listmarkerspace

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestItemVerdict_NoSprintfAlloc pins the allocation cost of building
// a violation's diagnostic message. itemVerdict used fmt.Sprintf with
// two %d verbs, which drives the reflection-based formatting path;
// every other structural rule in this codebase (listindent,
// headingincrement, atxheadingwhitespace, noduplicateheadings) builds
// the same shape of message with strconv.Itoa + string concatenation
// instead — ~3x faster and allocation-free reflection per
// docs/development/high-performance-go.md "strconv over
// fmt.Sprintf".
func TestItemVerdict_NoSprintfAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	src := []byte("-   item with three spaces\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}

	_, ok := r.itemVerdict(f, false, false, 1)
	require.True(t, ok, "fixture must produce a violation")

	const allocBudget = 1
	allocs := testing.AllocsPerRun(200, func() {
		_, _ = r.itemVerdict(f, false, false, 1)
	})
	t.Logf("itemVerdict allocs/op = %.0f (budget = %d)", allocs, allocBudget)
	require.LessOrEqualf(t, allocs, float64(allocBudget),
		"itemVerdict allocs/op = %.0f, budget = %d (message build should skip fmt.Sprintf reflection)",
		allocs, allocBudget)
}
