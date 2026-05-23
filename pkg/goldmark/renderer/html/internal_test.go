package html

// Internal unit tests for unexported helpers. Drives the method
// receivers that the public test files cannot reach because
// they live in package html_test.

import (
	"testing"
)

func TestSoftLineBreak_AllEnumValues(t *testing.T) {
	// softLineBreak's switch covers None, Simple, CSS3Draft, and
	// a default fallthrough that returns false. The caller in
	// renderText guards on `EastAsianLineBreaks != None` so the
	// None branch is unreachable through Convert, and the default
	// is unreachable for any valid enum value. A direct unit test
	// drives both.
	cases := []struct {
		mode EastAsianLineBreaks
		a, b rune
		want bool
	}{
		{EastAsianLineBreaksNone, 'A', 'B', false},
		{EastAsianLineBreaksSimple, 'A', 'B', true},                                 // narrow + narrow
		{EastAsianLineBreaksSimple, 0x4E00, 0x4E01, false},                          // wide + wide
		{EastAsianLineBreaksCSS3Draft, 'A', 'B', true},                              // Rule 4 default
		{EastAsianLineBreaks(99), 'A', 'B', false},                                  // default arm of switch
	}
	for _, c := range cases {
		if got := c.mode.softLineBreak(c.a, c.b); got != c.want {
			t.Errorf("softLineBreak(%d, %U, %U) = %v, want %v", c.mode, c.a, c.b, got, c.want)
		}
	}
}
