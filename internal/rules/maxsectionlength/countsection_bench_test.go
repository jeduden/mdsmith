package maxsectionlength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkCheck_ManySections pins docs/development/high-performance-go.md's
// "Skip work you don't need" guideline: countSection used to rescan the
// *entire* paragraphs slice from index 0 for every heading, making
// Check's per-heading loop O(headings * paragraphs) instead of
// O(headings + paragraphs). This benchmark's n scales both headings and
// paragraphs together, so the quadratic term dominates unless the
// forward-cursor fix is in place.
func BenchmarkCheck_ManySections(b *testing.B) {
	f, err := lint.NewFile("test.md", []byte(manySectionsDoc(600)))
	if err != nil {
		b.Fatal(err)
	}
	r := &Rule{MaxWords: 1000, MaxParagraphs: 1000}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}
