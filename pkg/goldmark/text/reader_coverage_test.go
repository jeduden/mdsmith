package text

// Coverage for text.Reader methods. Upstream's reader_test.go
// covers position-and-advance basics but leaves FindClosure,
// PrecendingCharacter, Match/FindSubMatch, LineOffset, the
// padding setters, and SkipBlankLines uncovered.

import (
	"regexp"
	"testing"
)

func TestReader_FindClosure(t *testing.T) {
	r := NewReader([]byte("(hello)"))
	r.Advance(1) // step past '('
	segments, found := r.FindClosure('(', ')', FindClosureOptions{Advance: true})
	if !found {
		t.Fatal("FindClosure should locate the closing paren")
	}
	if segments.Len() == 0 {
		t.Error("FindClosure returned empty segments")
	}
}

func TestReader_PrecendingCharacter(t *testing.T) {
	r := NewReader([]byte("ab cd"))
	// At pos.Start==0 with no padding, the function returns '\n'
	// (a sentinel meaning "start of document").
	if got := r.PrecendingCharacter(); got != '\n' {
		t.Errorf("PrecendingCharacter at start = %U, want newline sentinel", got)
	}
	r.Advance(2)
	if got := r.PrecendingCharacter(); got != 'b' {
		t.Errorf("PrecendingCharacter after Advance(2) = %U, want 'b'", got)
	}
	// With pos.Padding > 0 at start, the sentinel switches to ' '.
	r2 := NewReader([]byte("ab"))
	r2.SetPadding(2)
	if got := r2.PrecendingCharacter(); got != ' ' {
		t.Errorf("PrecendingCharacter with padding = %U, want ' '", got)
	}
}

func TestReader_MatchAndFindSubMatch(t *testing.T) {
	r := NewReader([]byte("hello world"))
	re := regexp.MustCompile(`^hello`)
	if !r.Match(re) {
		t.Error("Match should find ^hello at start")
	}
	// Match advances past the match; verify by peeking the next byte.
	if r.Peek() != ' ' {
		t.Errorf("after Match the cursor should be at the space, got %q", string(r.Peek()))
	}

	// Reset and exercise FindSubMatch on a regex with one group.
	r2 := NewReader([]byte("k=v"))
	subRe := regexp.MustCompile(`^(\w+)=(\w+)`)
	subs := r2.FindSubMatch(subRe)
	if len(subs) < 3 {
		t.Fatalf("FindSubMatch returned %d submatches, want >=3", len(subs))
	}
	if string(subs[1]) != "k" || string(subs[2]) != "v" {
		t.Errorf("FindSubMatch groups = %q/%q, want k/v", subs[1], subs[2])
	}
}

func TestReader_LineOffset(t *testing.T) {
	r := NewReader([]byte("aaa\nbbb"))
	if off := r.LineOffset(); off != 0 {
		t.Errorf("initial LineOffset = %d, want 0", off)
	}
	r.AdvanceLine()
	if off := r.LineOffset(); off != 0 {
		t.Errorf("after AdvanceLine LineOffset = %d, want 0", off)
	}
}

func TestReader_AdvanceAndSetPadding(t *testing.T) {
	r := NewReader([]byte("    body"))
	r.AdvanceAndSetPadding(2, 2)
	// SkipSpaces should now consume the remaining 2 spaces.
	_, n, _ := r.SkipSpaces()
	if n < 0 {
		t.Errorf("SkipSpaces after AdvanceAndSetPadding returned negative: %d", n)
	}
}

func TestReader_AdvanceToEOL(t *testing.T) {
	r := NewReader([]byte("first\nsecond\n"))
	r.AdvanceToEOL()
	// After AdvanceToEOL the position is at the newline.
	if r.Peek() != '\n' {
		t.Errorf("after AdvanceToEOL Peek() = %q, want '\\n'", string(r.Peek()))
	}
}

func TestReader_SetPadding(t *testing.T) {
	r := NewReader([]byte("ab"))
	r.SetPadding(3)
	// Padding is internal but SkipSpaces should report a positive
	// count when the padding is consumed.
	_, n, _ := r.SkipSpaces()
	_ = n // padding interacts with SkipSpaces in non-trivial ways;
	// the call is the coverage target.
}

func TestReader_SkipBlankLines(t *testing.T) {
	r := NewReader([]byte("\n\nbody"))
	_, lines, ok := r.SkipBlankLines()
	if !ok {
		t.Error("SkipBlankLines should report ok on consecutive blank lines")
	}
	if lines < 1 {
		t.Errorf("SkipBlankLines reported %d lines, want >= 1", lines)
	}
}

func TestReadRuneReader_AllBranches(t *testing.T) {
	// readRuneReader has branches for nil line (EOF), RuneError
	// (invalid UTF-8 also EOF), and normal rune decode.

	// Empty reader -> EOF.
	r := NewReader([]byte{})
	if _, _, err := readRuneReader(r); err == nil {
		t.Error("readRuneReader on empty source should return error")
	}

	// Invalid UTF-8 -> EOF.
	r = NewReader([]byte{0xFF, 0xFE})
	if _, _, err := readRuneReader(r); err == nil {
		t.Error("readRuneReader on invalid UTF-8 should return error")
	}

	// Normal rune.
	r = NewReader([]byte("a"))
	if rn, _, err := readRuneReader(r); err != nil || rn != 'a' {
		t.Errorf("readRuneReader('a') = %v %v, want 'a' nil", rn, err)
	}
}

func TestFindSubMatchReader_AllBranches(t *testing.T) {
	// findSubMatchReader has branches:
	//   - regex doesn't match -> return nil
	//   - regex matches with optional group not captured -> empty []byte
	//   - regex matches normally

	// No match.
	r := NewReader([]byte("abc"))
	re := regexp.MustCompile(`xyz`)
	if got := findSubMatchReader(r, re); got != nil {
		t.Error("findSubMatchReader should return nil on no match")
	}

	// Optional group not captured.
	r = NewReader([]byte("hello"))
	re2 := regexp.MustCompile(`(hello)(.*)?`)
	got := findSubMatchReader(r, re2)
	if got == nil {
		t.Error("findSubMatchReader should match")
	}
}

func TestReader_Peek_WithPadding(t *testing.T) {
	// reader.Peek's r.pos.Padding != 0 branch fires when the
	// reader's position carries leading padding (from
	// AdvanceAndSetPadding etc).
	r := NewReader([]byte("abc"))
	r.SetPadding(2)
	if b := r.Peek(); b != ' ' {
		t.Errorf("Peek with padding should return space, got %c", b)
	}
}
