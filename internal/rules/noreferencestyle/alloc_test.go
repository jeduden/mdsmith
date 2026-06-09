package noreferencestyle

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
