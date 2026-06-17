package listscan

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
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

// TestOpeningFenceRel covers short-fence and backtick-in-info branches.
func TestOpeningFenceRel(t *testing.T) {
	// Only 2 backticks — too short (need 3+).
	_, ok := openingFenceRel([]byte("``text"), 0, 0)
	assert.False(t, ok, "2-backtick fence must fail")
	// Backtick in info string invalidates a backtick fence.
	_, ok = openingFenceRel([]byte("```go`"), 0, 0)
	assert.False(t, ok, "backtick in info string must fail")
	// Tilde fence with 3 tildes is valid (exercises the tilde path).
	fi, ok := openingFenceRel([]byte("~~~"), 0, 0)
	assert.True(t, ok, "3-tilde fence must succeed")
	assert.Equal(t, byte('~'), fi.char)
	// indent >= len(line) returns false.
	_, ok = openingFenceRel([]byte("  "), 2, 0)
	assert.False(t, ok, "indent at EOL must fail")
}

// TestClosingFence covers the over-indented branch.
func TestClosingFence(t *testing.T) {
	fi := fenceInfo{char: '`', length: 3, baseCol: 0}
	// 4 spaces past baseCol is too far — not a closing fence.
	assert.False(t, closingFence([]byte("    ```"), fi))
	// Exact match closes.
	assert.True(t, closingFence([]byte("```"), fi))
}

// TestInterruptsParagraph covers the thematic-break return-true branch.
func TestInterruptsParagraph(t *testing.T) {
	assert.True(t, interruptsParagraph([]byte("---"), 0))
	assert.True(t, interruptsParagraph([]byte("***"), 0))
	// ATX heading interrupts.
	assert.True(t, interruptsParagraph([]byte("# Heading"), 0))
	// Plain text does not.
	assert.False(t, interruptsParagraph([]byte("plain text"), 0))
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
// list item ends the list — exercises interruptsParagraph returning true
// inside scanLine's lazy check.
func TestParse_ThematicBreakSplitsList(t *testing.T) {
	src := "- a\n- b\n---\ntext\n"
	lists, _ := Parse(split(src))
	assert.Len(t, lists, 1, "thematic break closes the list")
	assert.Equal(t, 2, len(lists[0].Items))
}
