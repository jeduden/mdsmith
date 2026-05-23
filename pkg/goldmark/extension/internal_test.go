package extension

// Internal unit tests for unexported helpers in the extension
// package: isTableDelim and related table-parsing internals.

import (
	"testing"
)

func TestIsTableDelim_AllBranches(t *testing.T) {
	// Drive each branch:
	//   - IndentWidth > 3 -> false
	//   - allSep (only dashes) -> false
	//   - invalid char -> false
	//   - valid -> true
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid-simple", "---|---", true},
		{"valid-with-colons", ":---|---:|:---:", true},
		{"valid-with-spaces", " --- | --- ", true},
		{"only-dashes-no-pipe", "------", false},  // allSep -> false
		{"invalid-char", "---|--x", false},        // x is not allowed
		{"too-indented", "    ---|---", false},    // IndentWidth > 3
		{"empty", "", false},                      // allSep stays true on empty -> false
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTableDelim([]byte(c.in)); got != c.want {
				t.Errorf("isTableDelim(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
