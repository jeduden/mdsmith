package maxsectionlength

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCollectHeadings_AlreadyInDocumentOrder confirms the ast.Walk in
// collectHeadings visits headings in ascending source-line order on
// its own, including headings nested inside blockquotes and list
// items. lint's parser config installs no extension (e.g. footnotes)
// that relocates nodes out of document order — the same guarantee
// astutil.CollectSectionParagraphs documents and relies on to skip
// sorting. A sort after the walk is therefore dead work.
func TestCollectHeadings_AlreadyInDocumentOrder(t *testing.T) {
	src := "# Top\n\n" +
		"body\n\n" +
		"> ## Blockquoted heading\n" +
		"> body\n\n" +
		"- list item\n" +
		"  ### Heading in list item\n\n" +
		"## Second top-level\n\n" +
		"body\n"
	f := mustFile(t, src)
	heads := collectHeadings(f)
	require := assert.New(t)
	require.True(len(heads) >= 4, "expected at least 4 headings, got %d", len(heads))
	for i := 1; i < len(heads); i++ {
		require.LessOrEqualf(heads[i-1].line, heads[i].line,
			"heading %d (%q, line %d) out of order after heading %d (%q, line %d)",
			i, heads[i].text, heads[i].line, i-1, heads[i-1].text, heads[i-1].line)
	}
}

// manySectionsDoc builds a document with n level-2 headings, each
// followed by a short body paragraph.
func manySectionsDoc(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("## Section\n\nbody text here more words to be real content.\n\n")
	}
	return b.String()
}

// TestCollectHeadings_AllocBudget guards the fix for
// docs/development/high-performance-go.md's "reflect in hot paths"
// anti-pattern: sort.Slice drives reflect.Swapper internally, and
// TestCollectHeadings_AlreadyInDocumentOrder establishes the sort was
// unneeded — the ast.Walk already yields document order. Deleting it
// removes that reflect-based allocation from every Check call that
// walks headings.
func TestCollectHeadings_AllocBudget(t *testing.T) {
	f := mustFile(t, manySectionsDoc(50))
	allocs := testing.AllocsPerRun(200, func() {
		_ = collectHeadings(f)
	})
	t.Logf("collectHeadings(50 headings) allocs/op = %.1f", allocs)
	// 50 headings' own struct-slice growth plus per-heading text
	// extraction costs a real, expected amount; the ceiling here is set
	// just above that so a reintroduced sort.Slice (its own allocation
	// on top) trips it.
	const ceiling = 57
	if allocs > ceiling {
		t.Fatalf("collectHeadings allocs/op = %.1f, want <= %d (reflect-based sort.Slice reintroduced?)",
			allocs, ceiling)
	}
}
