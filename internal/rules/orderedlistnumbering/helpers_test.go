package orderedlistnumbering

import "testing"

func TestCountLeadingSpaces(t *testing.T) {
	cases := []struct {
		in   []byte
		want int
	}{
		{[]byte(""), 0},
		{[]byte("abc"), 0},
		{[]byte("   abc"), 3},
		{[]byte("   "), 3},
		{[]byte(" \tabc"), 1}, // tab is not a space; counting stops
	}
	for _, tc := range cases {
		if got := countLeadingSpaces(tc.in); got != tc.want {
			t.Errorf("countLeadingSpaces(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestIsBlank(t *testing.T) {
	cases := []struct {
		in   []byte
		want bool
	}{
		{[]byte(""), true},
		{[]byte("   "), true},
		{[]byte("\t\t"), true},
		{[]byte(" \t "), true},
		{[]byte("a"), false},
		{[]byte("  a  "), false},
		{[]byte(" x"), false},
	}
	for _, tc := range cases {
		if got := isBlank(tc.in); got != tc.want {
			t.Errorf("isBlank(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestParseListItemNumber_ZeroAllocs verifies that parseListItemNumber does not
// allocate on the heap. The string([]byte) conversion in the old implementation
// allocated once per call; this test catches any regression back to that shape.
func TestParseListItemNumber_ZeroAllocs(t *testing.T) {
	line := []byte("42. item text")
	allocs := testing.AllocsPerRun(100, func() {
		parseListItemNumber(line)
	})
	if allocs != 0 {
		t.Fatalf("parseListItemNumber allocated %.0f times per call, want 0", allocs)
	}
}

func TestParseListItemNumber_LongDigitRun(t *testing.T) {
	// More than 9 digits must be rejected (overflow / non-CommonMark marker).
	line := []byte("1234567890. item")
	_, _, _, _, ok := parseListItemNumber(line)
	if ok {
		t.Error("expected ok=false for >9 digit run")
	}
}

func TestParseListItemNumber_NineDigits(t *testing.T) {
	// Exactly 9 digits is the CommonMark cap and must be accepted.
	line := []byte("999999999. item")
	n, _, _, _, ok := parseListItemNumber(line)
	if !ok {
		t.Fatal("expected ok=true for 9-digit marker")
	}
	if n != 999999999 {
		t.Errorf("got number %d, want 999999999", n)
	}
}
