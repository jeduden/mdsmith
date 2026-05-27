package flavor

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/markdown"
)

// BenchmarkDetectReusesPool exercises the dual-parser code path
// repeatedly to confirm the sync.Pool inside Detect avoids
// rebuilding the goldmark parser per call. Each iteration parses a
// small document that triggers the dual pass.
func BenchmarkDetectReusesPool(b *testing.B) {
	src := []byte("# Title {#top}\n\n" +
		"- [ ] task\n\n" +
		"| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
		"~~old~~ text\n")
	doc := markdown.Parse(src)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Detect(doc, nil)
	}
}
