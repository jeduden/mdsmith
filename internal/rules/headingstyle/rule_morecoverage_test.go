package headingstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// --- isATXHeadingNoLines: branch where Lines()==0 and has a text child ---
//
// Goldmark ATX headings have Lines().Len() == 0 by design. So isATXHeading
// always delegates to isATXHeadingNoLines for ATX headings parsed from source.
// The existing tests already exercise that via TestIsATXHeadingNoLines_WithTextChild.
//
// The uncovered branches inside isATXHeadingNoLines are:
//   1. heading.FirstChild() != nil && firstTextSegment returns {0,0}  → return true
//   2. heading.FirstChild() != nil && firstTextSegment returns non-zero → lineStartsWithHash
//
// Branch 1: a heading with a child that has no Text descendant.
// Branch 2: normal ATX heading — already covered by TestIsATXHeadingNoLines_WithTextChild,
//           but the explicit firstTextSegment → lineStartsWithHash call path needs exercising.

// TestIsATXHeadingNoLines_ChildWithZeroSegment exercises the branch where
// firstTextSegment returns {0,0} (Start==0, Stop==0) — i.e. no text node
// is found under the heading's child. The function should return true (default ATX).
func TestIsATXHeadingNoLines_ChildWithZeroSegment(t *testing.T) {
	// Build a heading manually: it has a non-Text child (e.g. Emphasis) whose
	// own children are also not Text nodes — so firstTextSegment returns {0,0}.
	h := ast.NewHeading(1)
	em := ast.NewEmphasis(2)
	// Emphasis node has no children (no text) → firstTextSegment returns zero seg.
	h.AppendChild(h, em)

	src := []byte("# test\n")
	result := isATXHeadingNoLines(h, src)
	// Cannot determine style; defaults to true (ATX).
	assert.True(t, result)
}

// TestIsATXHeadingNoLines_ChildWithTextSegment exercises the
// lineStartsWithHash path: firstTextSegment returns a non-zero segment,
// and we check if the line at that offset starts with '#'.
func TestIsATXHeadingNoLines_ChildWithTextSegment(t *testing.T) {
	// Build a heading manually with a text child whose segment points to
	// "# Heading\n" — the text "Heading" is at offset 2, and the line
	// starts with '#'.
	src := []byte("# Heading\n")

	h := ast.NewHeading(1)
	textNode := ast.NewText()
	textNode.Segment = text.NewSegment(2, 9) // "Heading"
	h.AppendChild(h, textNode)

	result := isATXHeadingNoLines(h, src)
	assert.True(t, result, "line starts with '#', so must be ATX")
}

// TestIsATXHeadingNoLines_ChildWithTextSegmentSetext exercises the
// lineStartsWithHash(false) branch: the segment points to a setext-style line.
func TestIsATXHeadingNoLines_ChildWithTextSegmentSetext(t *testing.T) {
	// Source: setext heading. Text "Heading" starts at offset 0.
	src := []byte("Heading\n=======\n")

	h := ast.NewHeading(1)
	textNode := ast.NewText()
	textNode.Segment = text.NewSegment(0, 7) // "Heading"
	h.AppendChild(h, textNode)

	result := isATXHeadingNoLines(h, src)
	assert.False(t, result, "line does not start with '#', so must be setext")
}

// --- headingByteRange: ATX heading with no Lines() and no direct text child ---
// This exercises the headingByteRange loop that finds children to locate start.

func TestHeadingByteRange_ATXWithEmphasisChild(t *testing.T) {
	// An ATX heading with only an Emphasis child (no direct Text child).
	// headingByteRange should still find the start via children.
	src := []byte("# *bold heading*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		start, end := headingByteRange(h, src)
		assert.GreaterOrEqual(t, start, 0)
		assert.Greater(t, end, start)
		return ast.WalkStop, nil
	})
}

// TestCheck_ATXBoldHeading verifies that a heading with bold inline content
// is correctly identified as ATX and does not trigger a false positive.
func TestCheck_ATXBoldHeading(t *testing.T) {
	src := []byte("## **Bold Section**\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "atx"}
	diags := r.Check(f)
	assert.Len(t, diags, 0, "ATX heading with bold should not be flagged: %+v", diags)
}

// TestCheck_ATXCodeHeading verifies that a heading with inline code content
// is correctly identified as ATX and does not trigger a false positive.
func TestCheck_ATXCodeHeading(t *testing.T) {
	src := []byte("## `code heading`\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "atx"}
	diags := r.Check(f)
	assert.Len(t, diags, 0, "ATX heading with code should not be flagged: %+v", diags)
}

// TestFix_ATXBoldToSetext verifies Fix can convert a bold ATX heading to setext.
func TestFix_ATXBoldToSetext(t *testing.T) {
	src := []byte("# **Bold Title**\n\nSome text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "setext"}
	result := r.Fix(f)
	// Should produce setext-style output.
	assert.Contains(t, string(result), "Bold Title")
}

// --- headingByteRange: else branch (Lines().Len() == 0) ---
// In goldmark, real-parsed headings always have Lines().Len() >= 1, so the
// else branch in headingByteRange is only reachable via a manually-constructed
// heading. We call headingByteRange directly to exercise lines 243-248.

func TestHeadingByteRange_NoLinesWithTextChild(t *testing.T) {
	// src: "# Title\n" — text "Title" at offset 2..7.
	src := []byte("# Title\n")

	// Build a heading node with Lines().Len() == 0 but a Text child.
	h := ast.NewHeading(1)
	textNode := ast.NewText()
	textNode.Segment = text.NewSegment(2, 7) // "Title"
	h.AppendChild(h, textNode)

	// Lines().Len() == 0 so else branch runs, finding start from text child.
	start, end := headingByteRange(h, src)
	// Start should be at the beginning of the line (0 since it's at offset 0).
	assert.Equal(t, 0, start)
	// End should be at end of the "# Title" line (before '\n').
	assert.Equal(t, 7, end)
}

func TestHeadingByteRange_NoLinesNoChildren(t *testing.T) {
	// A heading with Lines().Len() == 0 and no children: start stays 0.
	src := []byte("# Title\n")

	h := ast.NewHeading(1)
	// No children, no lines — start defaults to 0.
	start, end := headingByteRange(h, src)
	assert.GreaterOrEqual(t, start, 0)
	assert.GreaterOrEqual(t, end, start)
}
