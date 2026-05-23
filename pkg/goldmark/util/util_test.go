package util

// Unit tests for the fork's bound-check additions in
// ResolveNumericReferences (plan-198 CodeQL fix). The decimal
// (`&#NNN;`) and hex (`&#xNN;`) entity paths must:
//
//   1. Decode in-range Unicode code points correctly.
//   2. Clamp out-of-range values to U+FFFD instead of risking an
//      uint64 -> int32 wraparound.
//
// The CodeQL rule that motivated the bound check is
// go/incorrect-integer-conversion; the test below pins the
// behaviour the fix gives the analyser.

import (
	"testing"
	"unicode/utf8"
)

func TestResolveNumericReferences_HexAndDecimalPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // exact expected output (full-string equality)
	}{
		{"decimal-A", "&#65;", "A"},
		{"hex-A", "&#x41;", "A"},
		{"hex-snowman", "&#x2603;", "☃"},
		{"decimal-cherry", "&#127826;", "\U0001F352"},
		// Adjacent literals to confirm the rest of the source
		// stays untouched.
		{"surrounded", "<&#65;>", "<A>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(ResolveNumericReferences([]byte(tc.in)))
			if got != tc.want {
				t.Errorf("ResolveNumericReferences(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveNumericReferences_HexOverflowClampedToReplacement(t *testing.T) {
	// Two cases. The first proves the unicode-range cap is honoured
	// for a value safely inside int32 range. The second drives the
	// explicit `v > math.MaxInt32` clamp by passing a 32-bit hex
	// value that exceeds it — ParseUint(_, 16, 32) returns up to
	// 0xFFFFFFFF (4,294,967,295), which is above math.MaxInt32.
	cases := []string{
		"&#x110000;",   // 1,114,112 — past Unicode max, inside int32.
		"&#xFFFFFFFF;", // 4,294,967,295 — exceeds math.MaxInt32; hits the explicit clamp.
		"&#x80000000;", // 2,147,483,648 — exactly math.MaxInt32+1; the boundary case.
	}
	for _, in := range cases {
		got := string(ResolveNumericReferences([]byte(in)))
		if got != "�" {
			t.Errorf("ResolveNumericReferences(%q) = %q, want U+FFFD", in, got)
		}
	}
}

func TestResolveNumericReferences_DecimalOverflowClampedToReplacement(t *testing.T) {
	// 7-digit decimal limit (i-start < 8) caps v at 9,999,999.
	// 9999999 is outside Unicode max (0x10FFFF = 1,114,111), so
	// ToValidRune turns it into U+FFFD via the downstream path.
	in := "&#9999999;"
	got := string(ResolveNumericReferences([]byte(in)))
	if got != "�" {
		t.Errorf("ResolveNumericReferences(%q) = %q, want \\uFFFD", in, got)
	}
}

func TestResolveNumericReferences_PreservesNonEntities(t *testing.T) {
	// Anything that isn't a complete &#...; / &#x...; entity
	// must pass through unchanged.
	cases := []string{
		"plain text",
		"&amp;",          // named entity, not numeric
		"&# missing ;",   // numeric path fails the IsNumeric check
		"&#;",            // empty body
		"&#xZZ;",         // not hex
		"unterminated &", // bare ampersand
	}
	for _, in := range cases {
		got := string(ResolveNumericReferences([]byte(in)))
		if got != in {
			t.Errorf("ResolveNumericReferences(%q) = %q, want passthrough", in, got)
		}
	}
}

func TestCopyOnWriteBuffer_WriteString(t *testing.T) {
	src := []byte("seed")
	buf := NewCopyOnWriteBuffer(src)
	buf.WriteString("xyz")
	if got := string(buf.Bytes()); got != "xyz" {
		t.Errorf("WriteString result = %q, want xyz", got)
	}
	if !buf.IsCopied() {
		t.Error("WriteString must mark buffer as copied")
	}
}

func TestIsEscapedPunctuation(t *testing.T) {
	cases := []struct {
		src  string
		i    int
		want bool
	}{
		{`\*`, 0, true},
		{`\a`, 0, false}, // 'a' is not punctuation
		{`a`, 0, false},
		{`\`, 0, false}, // backslash at end of source
	}
	for _, tc := range cases {
		if got := IsEscapedPunctuation([]byte(tc.src), tc.i); got != tc.want {
			t.Errorf("IsEscapedPunctuation(%q, %d) = %v, want %v", tc.src, tc.i, got, tc.want)
		}
	}
}

func TestVisualizeSpaces(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a b", "a[SPACE]b"},
		{"a\tb", "a[TAB]b"},
		{"a\nb", "a[NEWLINE]\nb"},
		{"a\rb", "a[CR]b"},
		{"a\vb", "a[VTAB]b"},
		{"a\x00b", "a[NUL]b"},
		{"a�b", "a[U+FFFD]b"},
	}
	for _, tc := range cases {
		if got := string(VisualizeSpaces([]byte(tc.in))); got != tc.want {
			t.Errorf("VisualizeSpaces(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDedentPosition(t *testing.T) {
	// Width=0 fast-return.
	pos, padding := DedentPosition([]byte("    body"), 0, 0)
	if pos != 0 || padding != 0 {
		t.Errorf("DedentPosition(width=0) = (%d, %d), want (0, 0)", pos, padding)
	}
	// 4 spaces consumed, w=4, w>=width=2 -> (i=4, padding=4-2=2).
	pos, padding = DedentPosition([]byte("    body"), 0, 2)
	if pos != 4 || padding != 2 {
		t.Errorf("DedentPosition(4-space, width=2) = (%d, %d), want (4, 2)", pos, padding)
	}
	// Insufficient indent: w=2 < width=4 -> (i=2, padding=0).
	pos, padding = DedentPosition([]byte("  body"), 0, 4)
	if pos != 2 || padding != 0 {
		t.Errorf("DedentPosition(2-space, width=4) = (%d, %d), want (2, 0)", pos, padding)
	}
	// Tab handling: '\t' at pos=0 expands to width 4.
	pos, padding = DedentPosition([]byte("\tbody"), 0, 2)
	if pos != 1 || padding != 2 {
		t.Errorf("DedentPosition(tab, width=2) = (%d, %d), want (1, 2)", pos, padding)
	}
}

func TestDedentPositionPadding(t *testing.T) {
	pos, padding := DedentPositionPadding([]byte("\tbody"), 0, 0, 2)
	if pos < 0 {
		t.Errorf("DedentPositionPadding pos = %d, want >= 0", pos)
	}
	_ = padding
}

func TestTrimRightLength(t *testing.T) {
	src := []byte("hello   ")
	got := TrimRightLength(src, []byte(" "))
	if got != 3 {
		t.Errorf("TrimRightLength stripped %d, want 3", got)
	}
}

func TestPrioritizedSlice_Remove(t *testing.T) {
	a := Prioritized("a", 1)
	b := Prioritized("b", 2)
	c := Prioritized("c", 3)
	s := PrioritizedSlice{a, b, c}
	s2 := s.Remove("b")
	if len(s2) != 2 {
		t.Fatalf("Remove len = %d, want 2", len(s2))
	}
	if s2[0].Value != "a" || s2[1].Value != "c" {
		t.Errorf("Remove order wrong: %v", s2)
	}
	// Removing a missing value returns the slice unchanged.
	s3 := s.Remove("zzz")
	if len(s3) != 3 {
		t.Errorf("Remove(missing) len = %d, want 3 unchanged", len(s3))
	}
}

func TestBytesFilter_Extend(t *testing.T) {
	base := NewBytesFilter([]byte("foo"))
	ext := base.Extend([]byte("bar"))
	if !ext.Contains([]byte("foo")) {
		t.Error("extended filter must keep base entries")
	}
	if !ext.Contains([]byte("bar")) {
		t.Error("extended filter must include new entries")
	}
}

// TestToValidRune pins the contract ResolveNumericReferences relies
// on for the post-clamp path: invalid runes (including the
// replacement code path through 0xFFFD itself) round-trip via
// utf8.EncodeRune to the U+FFFD glyph.
func TestToValidRune(t *testing.T) {
	cases := []struct {
		name string
		in   rune
		want rune
	}{
		{"zero-maps-to-replacement", 0, 0xFFFD},
		{"valid-ascii", 'A', 'A'},
		{"valid-bmp", '☃', '☃'},
		{"valid-astral", '\U0001F352', '\U0001F352'},
		{"surrogate-half-invalid", 0xD800, 0xFFFD},
		{"above-unicode-max-invalid", 0x110000, 0xFFFD},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToValidRune(tc.in)
			if got != tc.want {
				t.Errorf("ToValidRune(%U) = %U, want %U", tc.in, got, tc.want)
			}
			if !utf8.ValidRune(got) {
				t.Errorf("ToValidRune(%U) returned non-valid rune %U", tc.in, got)
			}
		})
	}
}
