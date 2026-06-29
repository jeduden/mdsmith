package orderedlistnumbering

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Equal(t, tc.want, isBlank(tc.in), "isBlank(%q)", tc.in)
	}
}

func TestParseListItemNumber_ZeroAllocs(t *testing.T) {
	line := []byte("42. item text")
	allocs := testing.AllocsPerRun(100, func() {
		parseListItemNumber(line)
	})
	assert.Zero(t, allocs, "parseListItemNumber must not allocate")
}

func TestParseListItemNumber_LongDigitRun(t *testing.T) {
	line := []byte("1234567890. item")
	_, _, _, _, ok := parseListItemNumber(line)
	assert.False(t, ok, "digit run > commonMarkMaxOrderedDigits must be rejected")
}

func TestParseListItemNumber_NineDigits(t *testing.T) {
	line := []byte("999999999. item")
	n, _, _, _, ok := parseListItemNumber(line)
	require.True(t, ok, "9-digit marker must be accepted")
	assert.Equal(t, 999999999, n)
}
