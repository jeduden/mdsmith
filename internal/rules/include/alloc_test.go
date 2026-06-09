package include

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMinFenceLen_Correctness verifies the function returns the right length
// before and after the allocation-free rewrite.
func TestMinFenceLen_Correctness(t *testing.T) {
	assert.Equal(t, 3, minFenceLen("hello world"), "plain text → 3")
	assert.Equal(t, 4, minFenceLen("text with ``` backticks"), "3 backticks → 4")
	assert.Equal(t, 5, minFenceLen("a ```` run"), "4 backticks → 5")
	assert.Equal(t, 3, minFenceLen(""), "empty string → 3")
	assert.Equal(t, 4, minFenceLen("line1\n```line2\n"), "3 backticks across newline → 4")
	assert.Equal(t, 5, minFenceLen("first ``` line\nsecond ```` line\n"), "max across lines → 5")
}

// TestMinFenceLen_ZeroAllocs confirms the rewrite allocates nothing.
func TestMinFenceLen_ZeroAllocs(t *testing.T) {
	inputs := []string{
		"hello world",
		"text with ``` backticks",
		"line1\n```line2\nline3\n",
	}
	for _, s := range inputs {
		s := s
		allocs := testing.AllocsPerRun(100, func() {
			_ = minFenceLen(s)
		})
		assert.Equal(t, 0.0, allocs, "minFenceLen(%q) allocs: want 0, got %v", s, allocs)
	}
}
