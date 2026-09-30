package callouttype

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/setutil"
)

// TestIsAllowed_NoAllocOnUppercaseToken pins the case-folded allow-set
// lookup to zero allocations: `[!NOTE]` is uppercase by convention, so
// strings.ToLower would allocate on every callout (see
// docs/development/high-performance-go.md, "Strings and bytes").
func TestIsAllowed_NoAllocOnUppercaseToken(t *testing.T) {
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	allowed := setutil.FromStrings([]string{"note", "warning"})
	allocs := testing.AllocsPerRun(100, func() {
		if !isAllowed(allowed, "NOTE") {
			t.Fatal("NOTE must be allowed")
		}
	})
	assert.Zero(t, allocs)
}

func TestIsAllowed_Semantics(t *testing.T) {
	allowed := setutil.FromStrings([]string{"note", "my-type"})
	assert.True(t, isAllowed(allowed, "note"))
	assert.True(t, isAllowed(allowed, "Note"))
	assert.True(t, isAllowed(allowed, "MY-TYPE"))
	assert.False(t, isAllowed(allowed, "bogus"))
	assert.False(t, isAllowed(allowed, ""))
	long := "ABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMNOPQRSTUVWXYZ"
	assert.False(t, isAllowed(allowed, long))
	lowerLong := setutil.FromStrings([]string{strings.ToLower(long)})
	assert.True(t, isAllowed(lowerLong, long))
}
