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
