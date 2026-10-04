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

func TestQuoteRun(t *testing.T) {
	lines := [][]byte{
		[]byte("> a"), []byte("lazy"), []byte("> b"), []byte("# H"),
		[]byte("> c"), []byte(""), []byte("> d"),
	}
	assert.Equal(t, 3, quoteRun(lines, 0), "quote and lazy lines stop at a heading")
	assert.Equal(t, 0, quoteRun(lines, 3))
	assert.Equal(t, 1, quoteRun(lines, 4), "stops at a blank line")
	assert.Equal(t, 0, quoteRun(lines, 5))
	assert.Equal(t, 1, quoteRun(lines, 6))
	assert.Equal(t, 0, quoteRun(lines, 7))
}

// TestLayer0AlternatingQuotesAndHeadingsAllocScalesWithQuote covers a
// document with no blank lines: each one-line quote must size its
// buffers by the quote, not by the heading-interrupted run after it.
func TestLayer0AlternatingQuotesAndHeadingsAllocScalesWithQuote(t *testing.T) {
	var src bytes.Buffer
	for i := 0; i < 5000; i++ {
		src.WriteString("> q\n# H\n")
	}
	lines := bytes.Split(src.Bytes(), []byte{'\n'})

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	scan := scanLayer0(lines)
	runtime.ReadMemStats(&after)

	assert.NotNil(t, scan)
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(16<<20))
}
