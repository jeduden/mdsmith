package listscan

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// split converts a string to the lines slice Parse expects (bytes.Split on "\n").
func split(s string) [][]byte { return bytes.Split([]byte(s), []byte("\n")) }

// TestHasMarkerToken covers the edge-case branches in hasMarkerToken.
func TestHasMarkerToken(t *testing.T) {
	// indent >= len(line): empty line reaches the guard.
	assert.False(t, hasMarkerToken([]byte{}, 0))
	// Thematic break at a bullet position is not a list marker.
	assert.False(t, hasMarkerToken([]byte("---"), 0))
	assert.False(t, hasMarkerToken([]byte("***"), 0))
	// Ordered marker is detected.
	assert.True(t, hasMarkerToken([]byte("1. item"), 0))
	// Bullet at col 0 is detected.
	assert.True(t, hasMarkerToken([]byte("- item"), 0))
}

// TestParseMarker covers the early-return branches in parseMarker.
func TestParseMarker(t *testing.T) {
	// Thematic break is not a marker.
	_, ok := parseMarker([]byte("---"), 0, 0)
	assert.False(t, ok, "thematic break must not parse as marker")
	// Bullet with no whitespace after it is not a marker.
	_, ok = parseMarker([]byte("-x"), 0, 0)
	assert.False(t, ok, "bullet without following space must not parse")
	// Indent 4+ past baseCol is not a marker (would be code).
	_, ok = parseMarker([]byte("    - item"), 0, 0)
	assert.False(t, ok, "4-space indent past baseCol is code, not marker")
}

// TestOrderedInfo covers the early-return branches in orderedInfo.
func TestOrderedInfo(t *testing.T) {
	// No digits at all.
	_, ok := orderedInfo([]byte("x."), 0)
	assert.False(t, ok, "no digits must fail")
	// More than 9 digits is invalid.
	_, ok = orderedInfo([]byte("1234567890."), 0)
	assert.False(t, ok, "10+ digit number must fail")
	// Digit not followed by '.' or ')'.
	_, ok = orderedInfo([]byte("1 item"), 0)
	assert.False(t, ok, "digit without delimiter must fail")
	// No whitespace after delimiter.
	_, ok = orderedInfo([]byte("1.x"), 0)
	assert.False(t, ok, "no space after delimiter must fail")
}

// TestParse_LazyParaContinuation checks that a bare continuation line at
// column 0 (below the item's content column) is absorbed as lazy paragraph
// text, keeping the item open — exercises the lazy-break in scanLine.
func TestParse_LazyParaContinuation(t *testing.T) {
	src := "- item text\ncontinuation at col 0\n- next\n"
	lists, items := Parse(split(src))
	assert.Len(t, lists, 1)
	assert.Len(t, items, 2)
}

// TestParse_UnclosedFence covers consumeFence returning i-1 when the
// source ends without a matching closing fence, and the trailing-empty
// break inside the same loop.
func TestParse_UnclosedFence(t *testing.T) {
	src := "- item\n  ```\n  code line\n"
	lists, _ := Parse(split(src))
	assert.Len(t, lists, 1, "list containing unclosed fence must be recorded")
}

// TestParse_ThematicBreakSplitsList checks that a thematic break after a
// list item ends the list — exercises lint.StartsInterruptingBlock
// returning true inside scanLine's lazy check.
func TestParse_ThematicBreakSplitsList(t *testing.T) {
	src := "- a\n- b\n---\ntext\n"
	lists, _ := Parse(split(src))
	assert.Len(t, lists, 1, "thematic break closes the list")
	assert.Equal(t, 2, len(lists[0].Items))
}

// TestParser_IsSetextUnderline covers each condition: a paragraph open
// in the line's own container, no blank before it, within three columns
// of that container, and a run of one of '=' or '-'.
func TestParser_IsSetextUnderline(t *testing.T) {
	root := &parser{topInParagraph: true}
	for _, line := range []string{"=", "===", "-", "--", "   ---  "} {
		assert.True(t, root.isSetextUnderline([]byte(line), astutil.CountLeadingSpaces([]byte(line))), "%q", line)
	}
	for _, line := range []string{"= =", "=-", "--x", "    ==="} {
		assert.False(t, root.isSetextUnderline([]byte(line), astutil.CountLeadingSpaces([]byte(line))), "%q", line)
	}
	assert.False(t, (&parser{}).isSetextUnderline([]byte("==="), 0), "no paragraph is open")
	assert.False(t, (&parser{topInParagraph: true, blankRun: 1}).isSetextUnderline([]byte("==="), 0))

	item := &parser{stack: []frame{{contentCol: 2, inParagraph: true}}}
	assert.True(t, item.isSetextUnderline([]byte("  --"), 2))
	assert.False(t, item.isSetextUnderline([]byte("--"), 0), "a lazy line is no underline")
	assert.False(t, item.isSetextUnderline([]byte("      --"), 6))
}

// ParseLists must return the same lists as Parse without building the
// flat item slice (docs/development/high-performance-go.md, "Skip work
// you don't need").
func TestParseLists_MatchesParseLists(t *testing.T) {
	src := "- a\n  - b\n  - c\n- d\n  - e\n1. x\n2. y\n"
	lines := bytes.Split([]byte(src), []byte("\n"))
	want, _ := Parse(lines)
	got := ParseLists(lines)
	require.Equal(t, want, got)
}

// A flat item slice over nested lists must stay in document order.
func TestParse_FlatItemsInDocumentOrder(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("- a\n  - b\n")
	}
	_, items := Parse(bytes.Split([]byte(sb.String()), []byte("\n")))
	require.Len(t, items, 100)
	for i := 1; i < len(items); i++ {
		require.Less(t, items[i-1].Line, items[i].Line)
	}
}

func BenchmarkParse_NestedList(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("- a\n  - b\n")
	}
	lines := bytes.Split([]byte(sb.String()), []byte("\n"))
	b.ReportAllocs()
	for b.Loop() {
		Parse(lines)
	}
}
