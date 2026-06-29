package noreferencestyle

import "testing"

func TestIsBlankLine(t *testing.T) {
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
		if got := isBlankLine(tc.in); got != tc.want {
			t.Errorf("isBlankLine(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
