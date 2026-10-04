package lint

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLayer0BlockquoteAllocScalesWithQuote guards the "pre-size slices"
// rule in docs/development/high-performance-go.md: tryBlockquote must
// size its body buffers by the quote it scans, not by the number of
// lines left in the document. Fifty one-line quotes at the top of a
// 20,000-line file would otherwise reserve ~30 MB.
func TestLayer0BlockquoteAllocScalesWithQuote(t *testing.T) {
	var src bytes.Buffer
	for i := 0; i < 50; i++ {
		src.WriteString("> quoted\n\n")
	}
	for i := 0; i < 20000; i++ {
		src.WriteString("plain line\n\n")
	}
	lines := bytes.Split(src.Bytes(), []byte{'\n'})

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	scan := scanLayer0(lines)
	runtime.ReadMemStats(&after)

	assert.NotNil(t, scan)
	const budget = 8 << 20
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(budget),
		"block quote scan allocated by remaining document length")
}

func TestNonBlankRun(t *testing.T) {
	lines := [][]byte{[]byte("a"), []byte("b"), []byte(""), []byte("c")}
	assert.Equal(t, 2, nonBlankRun(lines, 0))
	assert.Equal(t, 0, nonBlankRun(lines, 2))
	assert.Equal(t, 1, nonBlankRun(lines, 3))
	assert.Equal(t, 0, nonBlankRun(lines, 4))
}
