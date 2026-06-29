package noreferencestyle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, tc.want, isBlankLine(tc.in), "isBlankLine(%q)", tc.in)
	}
}
