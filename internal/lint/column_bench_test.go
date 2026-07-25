package lint

import (
	"bytes"
	"testing"
)

// BenchmarkColumnOfOffset_LongLine exercises the former worst case for
// ColumnOfOffset: an offset near the end of a very long line, which used
// to force a full backward byte-at-a-time scan to find the line start.
// ColumnOfOffset now reuses LineOfOffset's cached newline index via a
// binary search instead (docs/development/high-performance-go.md).
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
