package noundefinedreferencelabels

import (
	"strings"
	"testing"
)

// proseWithoutBrackets builds n lines of ordinary prose containing no
// '[' at all — the worst case for nextBracket's forward scan, since
// every byte in the file must be inspected before the scanner can
// report EOF.
func proseWithoutBrackets(lines int) []byte {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString("The quick brown fox jumps over the lazy dog again and again.\n")
	}
	return []byte(b.String())
}

// BenchmarkNextBracket_NoBrackets pins the doc's "bytes.IndexByte over
// a hand-rolled byte loop" guideline
// (docs/development/high-performance-go.md#strings-and-bytes):
// collectBrackets calls nextBracket to scan the *entire* file source
// once, byte by byte, looking for '['. On a file with no reference-style
// links at all (the common case — most files don't use them) the loop
// must inspect every byte before reporting EOF, so this is exactly the
// "big Source scan" case the guideline calls out as worth vectorizing.
func BenchmarkNextBracket_NoBrackets(b *testing.B) {
	source := proseWithoutBrackets(200)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _, _, ok := nextBracket(source, 0)
		if ok {
			b.Fatal("expected no bracket to be found")
		}
	}
}
