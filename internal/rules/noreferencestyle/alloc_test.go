package noreferencestyle

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParagraphEndLine_ZeroAllocs confirms paragraphEndLine allocates nothing
// after changing its signature to accept pre-split lines instead of raw source.
func TestParagraphEndLine_ZeroAllocs(t *testing.T) {
	lines := [][]byte{
		[]byte("First line of paragraph."),
		[]byte("Second line of paragraph."),
		[]byte(""),
		[]byte("After blank."),
	}
	defLines := map[int]struct{}{}

	allocs := testing.AllocsPerRun(100, func() {
		paragraphEndLine(lines, 0, defLines)
	})
	assert.Equal(t, 0.0, allocs, "paragraphEndLine allocs: want 0, got %v", allocs)
}

// TestParagraphEndLine_Correctness verifies behavior is unchanged by the
// signature change from []byte source to [][]byte lines.
func TestParagraphEndLine_Correctness(t *testing.T) {
	lines := [][]byte{
		[]byte("First line."),
		[]byte("Second line."),
		[]byte(""),
		[]byte("After blank."),
	}
	defLines := map[int]struct{}{3: {}}

	// paragraph starting at line 0 ends before the blank (line 2 → index 2)
	end := paragraphEndLine(lines, 0, defLines)
	assert.Equal(t, 2, end, "should stop at blank line (index 2)")

	// paragraph starting at line 3 (index 3 is "After blank.")
	// defLines has line 3 (1-based), so it stops before consuming it
	lines2 := [][]byte{
		[]byte("Para start."),
	}
	end2 := paragraphEndLine(lines2, 0, map[int]struct{}{})
	assert.Equal(t, 1, end2, "single-line paragraph ends at 1")
}

// manyFootnoteRefsSource builds a document with n footnote references,
// each in its own paragraph, so scanFootnoteReferences/
// scanFootnoteDefinitions see n regex matches per call.
func manyFootnoteRefsSource(n int) []byte {
	var src []byte
	for i := 0; i < n; i++ {
		src = append(src, []byte(fmt.Sprintf("See [^ref%d] for details.\n\n", i))...)
	}
	for i := 0; i < n; i++ {
		src = append(src, []byte(fmt.Sprintf("[^ref%d]: some definition text here.\n", i))...)
	}
	return src
}

// TestScanFootnoteReferences_PresizedAllocs pins
// docs/development/high-performance-go.md's "pre-size slices" pattern:
// scanFootnoteReferences knows FindAllSubmatchIndex's match count up
// front, so out should be allocated once via
// make([]footnoteOccurrence, 0, len(matches)) instead of growing from a
// nil slice across repeated append calls.
func TestScanFootnoteReferences_PresizedAllocs(t *testing.T) {
	f, err := lint.NewFile("refs.md", manyFootnoteRefsSource(20))
	require.NoError(t, err)
	codeLines := map[int]struct{}{}

	allocs := testing.AllocsPerRun(50, func() {
		scanFootnoteReferences(f, codeLines, nil)
	})
	assert.LessOrEqualf(t, allocs, 64.0,
		"scanFootnoteReferences allocs regressed: got %v, want <= 64", allocs)
}

// TestScanFootnoteDefinitions_PresizedAllocs mirrors
// TestScanFootnoteReferences_PresizedAllocs for scanFootnoteDefinitions.
func TestScanFootnoteDefinitions_PresizedAllocs(t *testing.T) {
	f, err := lint.NewFile("defs.md", manyFootnoteRefsSource(20))
	require.NoError(t, err)
	codeLines := map[int]struct{}{}

	allocs := testing.AllocsPerRun(50, func() {
		scanFootnoteDefinitions(f, codeLines)
	})
	assert.LessOrEqualf(t, allocs, 43.0,
		"scanFootnoteDefinitions allocs regressed: got %v, want <= 43", allocs)
}

// TestScanFootnoteReferences_NoMatches_ReturnsNil and
// TestScanFootnoteDefinitions_NoMatches_ReturnsNil pin
// docs/development/high-performance-go.md's "return nil, not []T{}"
// convention for the zero-match case: make([]footnoteOccurrence, 0,
// len(matches)) with len(matches) == 0 still returns a non-nil empty
// slice in Go, so the pre-sizing fix must special-case the no-match
// return to stay nil, matching the convention this same PR applies to
// headingincrement and tableformat.
func TestScanFootnoteReferences_NoMatches_ReturnsNil(t *testing.T) {
	f, err := lint.NewFile("clean.md", []byte("No footnotes here.\n"))
	require.NoError(t, err)
	out := scanFootnoteReferences(f, map[int]struct{}{}, nil)
	assert.Nil(t, out, "scanFootnoteReferences must return nil when there are no matches")
}

func TestScanFootnoteDefinitions_NoMatches_ReturnsNil(t *testing.T) {
	f, err := lint.NewFile("clean.md", []byte("No footnotes here.\n"))
	require.NoError(t, err)
	out := scanFootnoteDefinitions(f, map[int]struct{}{})
	assert.Nil(t, out, "scanFootnoteDefinitions must return nil when there are no matches")
}
