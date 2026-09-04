package occurrence

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// manySectionsDoc builds a document with n level-2 headings, each
// followed by one short paragraph — the shape that drives
// checkSections's per-heading loop and, through it, the paragraph
// range scans.
func manySectionsDoc(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("## Section\n\nA short paragraph with a foo token in it.\n\n")
	}
	return b.String()
}

// BenchmarkCheck_ManySections pins docs/development/high-performance-go.md's
// "Skip work you don't need" guideline: checkSections's range helpers used
// to rescan the *entire* paragraphs slice from index 0 for every heading,
// making the section-scope Check O(headings * paragraphs) instead of
// O(headings + paragraphs).
func BenchmarkCheck_ManySections(b *testing.B) {
	f, err := lint.NewFile("test.md", []byte(manySectionsDoc(600)))
	if err != nil {
		b.Fatal(err)
	}
	r := &Rule{Scope: "section", Count: "each", Tokens: []string{"foo"}, CaseSensitive: true}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}
