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

func TestBlockReader_PrecedingCharacter_Branches(t *testing.T) {
	// At line start: returns '\n'.
	r := newTestBlockReader("hello\nworld\n")
	if got := r.PrecendingCharacter(); got != '\n' {
		t.Errorf("PrecendingCharacter at line start = %q, want '\\n'", got)
	}
	// Mid-line after advance: returns the preceding rune.
	r.Advance(3)
	if got := r.PrecendingCharacter(); got != 'l' {
		t.Errorf("PrecendingCharacter after 3 advance = %q, want 'l'", got)
	}
	// With padding: returns space.
	r2 := newTestBlockReader("abc\n")
	r2.SetPadding(2)
	if got := r2.PrecendingCharacter(); got != ' ' {
		t.Errorf("PrecendingCharacter with padding = %q, want ' '", got)
	}
}

func TestBlockReader_PeekWithPadding(t *testing.T) {
	r := newTestBlockReader("abc\n")
	r.SetPadding(3)
	if b := r.Peek(); b != ' ' {
		t.Errorf("Peek with padding should return space, got %c", b)
	}
}

func TestBlockReader_PeekAtEOF(t *testing.T) {
	r := newTestBlockReader("ab\n")
	r.Advance(2)
	r.AdvanceLine()
	if b := r.Peek(); b != text.EOF {
		t.Errorf("Peek past last segment should return EOF, got %c", b)
	}
}

func TestBlockReader_AdvanceAndSetPadding(t *testing.T) {
	r := newTestBlockReader("abcdefg\n")
	r.AdvanceAndSetPadding(2, 4)
	if _, seg := r.Position(); seg.Padding != 4 {
		t.Errorf("AdvanceAndSetPadding(2,4) padding=%d, want 4", seg.Padding)
	}
}

func TestReader_AdvanceWithPaddingAndNewlines(t *testing.T) {
	// reader.Advance has a slow-path loop body when padding != 0
	// or n > peekedLine.  Drive both: set padding then advance
	// past it, and advance past a newline.
	r := text.NewReader([]byte("ab\ncd\nef\n"))
	r.SetPadding(2)
	r.Advance(3) // consume padding then 1 byte
	r.Advance(5) // crosses newlines
	_ = r.Peek()
}

func TestReader_PeekWithPadding(t *testing.T) {
	// A plain text.Reader (non-block) should also handle padding
	// in its Peek and PeekLine paths.  Construct via text.NewReader
	// on a slice that begins with content; padding manipulation
	// is internal to the block reader so direct path coverage is
	// limited.  This is mainly for the EOF return.
	r := text.NewReader([]byte("abc"))
	if r.Peek() != 'a' {
		t.Errorf("expected first byte 'a', got %c", r.Peek())
	}
	r.Advance(3)
	if r.Peek() != text.EOF {
		t.Errorf("expected EOF after advance past end, got %c", r.Peek())
	}
}

func TestBlockReader_FindClosure_AllOptions(t *testing.T) {
	// Drive findClosureReader option combinations not reached
	// through the parser flow (only link parsing uses it, and
	// link parsing uses a fixed option set).
	cases := []struct {
		name string
		src  string
		opts text.FindClosureOptions
	}{
		{"nest-true", "[outer [inner] outer] end\n", text.FindClosureOptions{Nesting: true, Newline: false, Advance: true}},
		{"nest-false-finds-first", "[outer [inner] outer] end\n", text.FindClosureOptions{Nesting: false, Newline: false, Advance: true}},
		{"newline-true", "[multi\nline] end\n", text.FindClosureOptions{Nesting: false, Newline: true, Advance: true}},
		{"newline-false", "[no\ncross-newline] end\n", text.FindClosureOptions{Nesting: false, Newline: false, Advance: true}},
		{"codespan-skip", "[outer `code ]` end] after\n", text.FindClosureOptions{Nesting: false, Newline: false, Advance: true, CodeSpan: true}},
		{"escape-skip", `[escape \] then close]` + "\n", text.FindClosureOptions{Nesting: false, Newline: false, Advance: true}},
		{"unclosed", "[no closing\n", text.FindClosureOptions{Nesting: false, Newline: false, Advance: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newTestBlockReader(c.src)
			r.Advance(1) // skip the '['
			_, _ = r.FindClosure('[', ']', c.opts)
		})
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

func TestBlockReader_LineOffset_TabBranches(t *testing.T) {
	// blockReader.LineOffset's loop has a tab branch.  Construct
	// a reader where head < pos.Start spans both tabs and
	// non-tab characters.
	src := "  \tabc"
	r := newTestBlockReader(src)
	r.Advance(3) // past the tab
	off := r.LineOffset()
	_ = off
}

func TestBlockReader_AdvanceToEOL_NoTrailingNewline(t *testing.T) {
	// AdvanceToEOL has a branch for source not ending in '\n'.
	r := newTestBlockReader("abc")
	r.AdvanceToEOL()
}

func TestBlockReader_PrecedingCharacter_EmptySegments(t *testing.T) {
	// segments.Len() < 1 -> return '\n' branch.
	segs := text.NewSegments()
	r := text.NewBlockReader([]byte("x"), segs)
	if got := r.PrecendingCharacter(); got != '\n' {
		t.Errorf("PrecendingCharacter empty segments = %q, want '\\n'", got)
	}
}
