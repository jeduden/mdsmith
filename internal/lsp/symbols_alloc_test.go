package lsp

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildOutlineSplitsSourceOnce guards the "memoize per-input
// computations" rule in docs/development/high-performance-go.md:
// buildOutline must split the document into lines once, not once per
// symbol. Re-splitting a 6,000-line document for each of 300 headings
// allocates tens of megabytes; one split stays well under the budget.
func TestBuildOutlineSplitsSourceOnce(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "# Heading %d\n\n", i)
		for j := 0; j < 18; j++ {
			b.WriteString("some body text\n")
		}
	}
	src := []byte(b.String())

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	out := buildOutline(src)
	runtime.ReadMemStats(&after)

	assert.Len(t, out, 300)
	const budget = 20 << 20
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(budget),
		"buildOutline allocated too much; is the source re-split per symbol?")
}
