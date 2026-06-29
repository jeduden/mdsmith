package listindent

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
