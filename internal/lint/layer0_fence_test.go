package lint

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

// TestLayer0_BlockquoteFenceTrackingDoesNotAllocPerLine pins that the
// block-quote scan's open-fence tracking costs no heap allocation per
// body line. A block quote's extra allocations over a paragraph of the
// same line count (its body buffers) must be a constant, the same for
// 40 and 80 lines; comparing at equal line counts cancels the
// line-count-sized maps. Tracking the open fence through a pointer to
// the per-line Open result moved that result to the heap on every body
// line, so the overhead grew with the quote.
func TestLayer0_BlockquoteFenceTrackingDoesNotAllocPerLine(t *testing.T) {
	overhead := func(n int) float64 {
		doc := func(line string) [][]byte {
			return bytes.Split([]byte(strings.Repeat(line, n)), []byte("\n"))
		}
		quote, para := doc("> prose line\n"), doc("prose line\n")
		allocs := func(ls [][]byte) float64 {
			return testing.AllocsPerRun(20, func() { _ = scanLayer0(ls) })
		}
		return allocs(quote) - allocs(para)
	}
	assert.Equal(t, overhead(40), overhead(80))
}

func TestAdvanceFenceState(t *testing.T) {
	open, ok := mdfence.Open([]byte("```go"))
	require.True(t, ok)
	var none mdfence.Fence
	// No fence open: an opener starts one, anything else leaves none.
	assert.Equal(t, open, advanceFenceState(none, []byte("```go"), open, true))
	assert.Equal(t, none, advanceFenceState(none, []byte("text"), none, false))
	// A fence open: content keeps it, a matching closer ends it, and an
	// opener-shaped content line does not replace it.
	assert.Equal(t, open, advanceFenceState(open, []byte("code"), none, false))
	tilde, _ := mdfence.Open([]byte("~~~"))
	assert.Equal(t, open, advanceFenceState(open, []byte("~~~"), tilde, true))
	assert.Equal(t, none, advanceFenceState(open, []byte("```"), open, true))
}
