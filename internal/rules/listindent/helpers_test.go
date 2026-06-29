package listindent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, tc.want, countLeadingSpaces(tc.in), "countLeadingSpaces(%q)", tc.in)
	}
}
