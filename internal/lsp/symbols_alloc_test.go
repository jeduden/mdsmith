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

	var out []documentSymbol
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	out = buildOutline(src)
	runtime.ReadMemStats(&after)

	assert.Len(t, out, 300)
	const budget = 8 << 20
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(budget),
		"buildOutline allocated too much; is the source re-split per symbol?")
}

func TestRangeInLines_EmptyLines(t *testing.T) {
	// A nil line table is treated as one empty line, never indexed at -1.
	r := rangeInLines(1, 1, nil)
	assert.Equal(t, Range{}, r)
}

func TestRangeInLines_MatchesRangeForLines(t *testing.T) {
	src := []byte("héllo\nworld\r\nlast")
	lines := splitLines(src)
	for start := 0; start <= 4; start++ {
		for end := 0; end <= 4; end++ {
			assert.Equal(t, rangeForLines(start, end, src), rangeInLines(start, end, lines))
		}
	}
}
