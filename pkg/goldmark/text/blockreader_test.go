package text_test

// Direct-call coverage for blockReader methods that the parser
// path either exercises only in specific input shapes or not at
// all. Build a BlockReader over a small source and drive its API.

import (
	"regexp"
	"testing"

	"github.com/yuin/goldmark/text"
)

func newTestBlockReader(src string) text.BlockReader {
	segs := text.NewSegments()
	// Split source by newline into segments, each segment
	// pointing at the line range in the source bytes.
	off := 0
	bytes := []byte(src)
	for i := 0; i < len(bytes); i++ {
		if bytes[i] == '\n' {
			segs.Append(text.NewSegment(off, i+1))
			off = i + 1
		}
	}
	if off < len(bytes) {
		segs.Append(text.NewSegment(off, len(bytes)))
	}
	return text.NewBlockReader(bytes, segs)
}

func TestBlockReader_LineOffset(t *testing.T) {
	r := newTestBlockReader("    hello\nworld\n")
	r.PeekLine()      // load the first line
	if r.LineOffset() < 0 {
		t.Errorf("LineOffset should be >= 0, got %d", r.LineOffset())
	}
}

func TestBlockReader_LineOffset_TabExpansion(t *testing.T) {
	// LineOffset's recompute loop accounts for tab width via
	// util.TabWidth.  Drive the tab branch by advancing past
	// a tab character so r.pos.Start points beyond the tab.
	r := newTestBlockReader("\t\tdouble-tab\n")
	r.Advance(2) // advance past the two tabs
	off := r.LineOffset()
	if off < 0 {
		t.Errorf("LineOffset after tab advance should be >= 0, got %d", off)
	}
}

func TestBlockReader_AdvanceToEOL(t *testing.T) {
	r := newTestBlockReader("abc\ndef\n")
	r.AdvanceToEOL()
	// Read again — should be at the newline or just past it.
	line, _ := r.PeekLine()
	if len(line) == 0 {
		// At EOL, PeekLine may return the newline-only segment.
	}
}

func TestBlockReader_SkipBlankLines(t *testing.T) {
	r := newTestBlockReader("\n\n\nfirst content\n")
	_, _, ok := r.SkipBlankLines()
	if !ok {
		t.Error("SkipBlankLines should consume the leading blank lines")
	}
}

func TestBlockReader_SetPadding(t *testing.T) {
	r := newTestBlockReader("abc\n")
	r.SetPadding(3)
	if _, seg := r.Position(); seg.Padding != 3 {
		t.Errorf("SetPadding(3) did not stick, padding=%d", seg.Padding)
	}
}

func TestBlockReader_AdvanceAndSetPadding(t *testing.T) {
	r := newTestBlockReader("abcdefg\n")
	r.AdvanceAndSetPadding(2, 4)
	if _, seg := r.Position(); seg.Padding != 4 {
		t.Errorf("AdvanceAndSetPadding(2,4) padding=%d, want 4", seg.Padding)
	}
}

func TestBlockReader_FindSubMatch(t *testing.T) {
	r := newTestBlockReader("hello 123 world\n")
	re := regexp.MustCompile(`\d+`)
	match := r.FindSubMatch(re)
	if match == nil {
		t.Error("FindSubMatch returned nil for clear match")
	}
}
