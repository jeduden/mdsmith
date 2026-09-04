package requiredstructure

import (
	"strings"
	"testing"
)

// proseBodyNoFields builds n lines of ordinary prose with no `#`
// headings, no `{field}` references, and no fences or directives — the
// worst case for collectBodySyncPoints's line scan, since every branch
// after the newline search is a cheap no-op and the newline search
// itself dominates.
func proseBodyNoFields(lines int) []byte {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString("Ordinary prose line with no template syntax at all.\n")
	}
	return []byte(b.String())
}

// BenchmarkCollectBodySyncPoints_NoFields pins the doc's "bytes.IndexByte
// over a hand-rolled byte loop" guideline
// (docs/development/high-performance-go.md#strings-and-bytes):
// collectBodySyncPoints scans the schema body's *entire* content once,
// byte by byte, to split it into lines. A large schema with no `{field}`
// text anywhere still pays that full scan.
func BenchmarkCollectBodySyncPoints_NoFields(b *testing.B) {
	content := proseBodyNoFields(200)
	var headings []docHeading
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		syncPoints := make(map[int][]syncPoint)
		collectBodySyncPoints(content, headings, syncPoints, nil)
	}
}
