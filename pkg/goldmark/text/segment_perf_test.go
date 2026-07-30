package text_test

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSegments_Value_PreSized pins the allocation cost of Segments.Value on
// a multi-segment collection. The output is built from many small segments
// over a shared buffer so an unsized `append` (growing from nil) would need
// several grow-and-copy reallocations; a single `make([]byte, 0, total)`
// pre-sized from each Segment's Len() should need exactly one.
func TestSegments_Value_PreSized(t *testing.T) {
	const segLen = 50
	const segCount = 24
	buffer := make([]byte, segLen*segCount)
	for i := range buffer {
		buffer[i] = byte('a' + i%26)
	}

	var segs text.Segments
	for i := 0; i < segCount; i++ {
		segs.Append(text.NewSegment(i*segLen, (i+1)*segLen))
	}

	var got []byte
	allocs := testing.AllocsPerRun(50, func() {
		got = segs.Value(buffer)
	})

	require.Equal(t, buffer, got)
	assert.LessOrEqualf(t, allocs, float64(1),
		"expected a single pre-sized allocation; got %v allocations (missing make([]byte, 0, total)?)", allocs)
}

// TestSegments_Value_ForceNewlineStaysPreSized covers the case
// Len() cannot see: a ForceNewline segment whose buffer bytes don't
// already end in '\n' makes Value() emit one more byte than Len()
// reports (e.g. an unterminated fenced code block at EOF). The
// pre-sizing pass must budget that extra byte per ForceNewline
// segment, or this case falls back to a grow-and-copy every time.
//
// The backing buffer is deliberately longer than the last segment's
// Stop (mirroring a real document, where a block's Lines() rarely
// reach all the way to len(source)) so Segment.Value's own internal
// append — appending '\n' onto a buffer-aliased slice — has spare
// capacity to write into and doesn't itself allocate. That isolates
// what this test targets: Segments.Value's own pre-sizing pass.
func TestSegments_Value_ForceNewlineStaysPreSized(t *testing.T) {
	content := []byte("plain text, then a code block with no trailing newline")
	buffer := append(append([]byte{}, content...), make([]byte, 8)...)

	var segs text.Segments
	segs.Append(text.NewSegment(0, 20))
	segs.Append(text.Segment{Start: 20, Stop: len(content), ForceNewline: true})

	var got []byte
	allocs := testing.AllocsPerRun(50, func() {
		got = segs.Value(buffer)
	})

	want := append(append([]byte{}, content...), '\n')
	require.Equal(t, want, got)
	assert.LessOrEqualf(t, allocs, float64(1),
		"expected a single pre-sized allocation even with a ForceNewline segment; got %v", allocs)
}
