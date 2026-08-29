package index

import "bytes"

// newlineIndex returns the byte offsets of every '\n' in source, in
// ascending order. buildFileEntry's heading, link-ref-definition, and
// directive collectors each need a line/column for every symbol they
// emit; building this once per source and reusing it via
// lineOfOffsetIndexed/columnOfLineIndexed turns what was an
// O(symbols x file-size) rescan (lineOfOffset/columnOfLine walking
// from byte 0 on every call) into O(file-size) build plus O(log n)
// lookups — see docs/development/high-performance-go.md, "Memoize
// per-input computations". bytes.Count pre-sizes the slice so the
// append loop never regrows; bytes.IndexByte is SIMD-accelerated on
// amd64 where a hand-rolled byte loop is not.
func newlineIndex(source []byte) []int {
	nl := make([]int, 0, bytes.Count(source, newlineByte))
	for base := 0; ; {
		i := bytes.IndexByte(source[base:], '\n')
		if i < 0 {
			break
		}
		nl = append(nl, base+i)
		base += i + 1
	}
	return nl
}

var newlineByte = []byte{'\n'}

// lineOfOffsetIndexed is lineOfOffset but O(log n) via a prebuilt
// newlineIndex instead of an O(offset) rescan from byte 0.
func lineOfOffsetIndexed(nl []int, offset int) int {
	lo, hi := 0, len(nl)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if nl[mid] >= offset {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return 1 + lo
}

// columnOfLineIndexed is columnOfLine but finds the line's start
// offset in O(1) via the shared newlineIndex instead of summing every
// prior line's length on each call.
func columnOfLineIndexed(nl []int, lines [][]byte, lineIdx int, absOffset int) int {
	if lineIdx < 0 || lineIdx >= len(lines) {
		return 1
	}
	start := 0
	if lineIdx > 0 {
		start = nl[lineIdx-1] + 1
	}
	if absOffset < start {
		return 1
	}
	end := start + len(lines[lineIdx])
	if absOffset > end {
		absOffset = end
	}
	return absOffset - start + 1
}
