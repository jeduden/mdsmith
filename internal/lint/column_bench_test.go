package lint

import (
	"bytes"
	"testing"
)

// BenchmarkColumnOfOffset_LongLine exercises the worst case for
// ColumnOfOffset: an offset near the end of a very long line, forcing a
// full backward scan to find the line start. Per
// docs/development/high-performance-go.md, a hand-rolled byte-at-a-time
// scan is not vectorized by the compiler; bytes.LastIndexByte is SIMD
// assembly on amd64.
func BenchmarkColumnOfOffset_LongLine(b *testing.B) {
	line := bytes.Repeat([]byte("x"), 8192)
	src := append(append([]byte("prefix\n"), line...), '\n')
	f := &File{Source: src}
	offset := len(src) - 2 // last byte of the long line

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.ColumnOfOffset(offset)
	}
}
