package astutil

import (
	"fmt"
	"testing"
)

// buildManySectionsFixture returns nHeadings headings and nHeadings*5
// paragraphs (both sorted by Line, matching CollectSectionParagraphs'
// and CollectSectionHeadings' document-order production output), one
// heading per "section" of 5 paragraphs — the shape MDS057/MDS058
// (requiredtextpatterns / requiredmentions) walk once per heading.
func buildManySectionsFixture(nHeadings int) ([]SectionHeading, []SectionParagraph) {
	headings := make([]SectionHeading, nHeadings)
	paragraphs := make([]SectionParagraph, 0, nHeadings*5)
	line := 1
	for i := 0; i < nHeadings; i++ {
		headings[i] = SectionHeading{Level: 2, Line: line}
		line++
		for p := 0; p < 5; p++ {
			paragraphs = append(paragraphs, SectionParagraph{
				Line: line,
				Text: fmt.Sprintf("paragraph %d-%d text", i, p),
			})
			line++
		}
	}
	return headings, paragraphs
}

// BenchmarkSectionBodyPerHeading measures the requiredmentions /
// requiredtextpatterns call shape: SectionEnd + SectionBody once per
// heading over the full paragraph slice. Per
// docs/development/high-performance-go.md, SectionBody's per-call scan
// should not be O(len(paragraphs)) when both headings and paragraphs
// are already sorted by Line — a binary search bounds the range in
// O(log P) instead.
func BenchmarkSectionBodyPerHeading(b *testing.B) {
	headings, paragraphs := buildManySectionsFixture(500)
	totalLines := paragraphs[len(paragraphs)-1].Line

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i, h := range headings {
			end := SectionEnd(headings, i, totalLines)
			_ = SectionBody(paragraphs, nil, h.Line, end)
		}
	}
}
