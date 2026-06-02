package noreferencestyle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestScanRefDefLine exercises the byte scanner's reject branches that the
// rule-level fixtures don't reach: a missing/empty bracket, a missing colon,
// over-indentation, an empty destination, and a whitespace destination
// byte other than space or tab (e.g. a trailing carriage return).
func TestScanRefDefLine(t *testing.T) {
	tests := []struct {
		name               string
		source             string
		lineStart, lineEnd int
		wantStart, wantEnd int
		wantOK             bool
	}{
		{"valid definition", "[a]: x", 0, 6, 1, 2, true},
		{"valid with line offset", "xx\n[a]: y", 3, 9, 4, 5, true},
		{"missing closing bracket", "[abc", 0, 4, -1, -1, false},
		{"empty label", "[]:x", 0, 4, -1, -1, false},
		{"no colon after label", "[a] x", 0, 5, -1, -1, false},
		{"more than three leading spaces", "    [a]: x", 0, 10, -1, -1, false},
		{"nothing after colon", "[a]:   ", 0, 7, -1, -1, false},
		{"carriage-return destination", "[a]:\r", 0, 5, -1, -1, false},
		{"vertical-tab destination", "[a]:\vx", 0, 6, 1, 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := scanRefDefLine([]byte(tt.source), tt.lineStart, tt.lineEnd)
			assert.Equal(t, tt.wantOK, ok, "ok")
			assert.Equal(t, tt.wantStart, start, "labelStart")
			assert.Equal(t, tt.wantEnd, end, "labelEnd")
		})
	}
}

// TestLabelInRefsNoMatch covers the final `return false`: a normalized label
// that matches none of the supplied references (here, an empty reference set).
func TestLabelInRefsNoMatch(t *testing.T) {
	assert.False(t, labelInRefs([]byte("missing"), nil))
}

// TestStringEqualsBytes covers the alloc-free compare: equal, differing
// length, differing content, and the empty case.
func TestStringEqualsBytes(t *testing.T) {
	assert.True(t, stringEqualsBytes("abc", []byte("abc")))
	assert.False(t, stringEqualsBytes("abc", []byte("abd")))
	assert.False(t, stringEqualsBytes("ab", []byte("abc")))
	assert.True(t, stringEqualsBytes("", []byte("")))
}
