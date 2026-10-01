package include

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

func TestStepFence(t *testing.T) {
	steps := func(lines ...string) []bool {
		var tr mdfence.Tracker
		got := make([]bool, len(lines))
		for i, ln := range lines {
			got[i] = stepFence(&tr, ln)
		}
		return got
	}
	assert.Equal(t, []bool{true, true, true, false},
		steps("```", "## x", "```", "## y"), "opens, content, closes")
	assert.Equal(t, []bool{true, true, true, false},
		steps("      ```", "x", "\t  ````  ", "y"),
		"any leading spaces or tabs are stripped on open and close")
	assert.Equal(t, []bool{true, true, true, false},
		steps("```", "x", "```\r", "y"), "a CRLF closer closes")
	assert.Equal(t, []bool{true, true, true, false},
		steps("```\n", "x\n", "```\n", "y\n"), "lines keeping their LF")
	assert.Equal(t, []bool{true, true, true, true},
		steps("```", "``", "```go", "~~~"), "short, info, and other-char lines do not close")
	assert.Equal(t, []bool{false, false}, steps("```x``` is code.", "## h"),
		"a backtick in a backtick info string is not a fence")
}

// TestStepFence_ZeroAllocs pins that the string-to-bytes view does
// not copy: a line longer than the compiler's 32-byte stack buffer
// would otherwise allocate on every call.
func TestStepFence_ZeroAllocs(t *testing.T) {
	const long = "  ```go title=\"a fairly long info string here\""
	allocs := testing.AllocsPerRun(100, func() {
		var tr mdfence.Tracker
		_ = stepFence(&tr, long)
	})
	assert.Zero(t, allocs)
}
