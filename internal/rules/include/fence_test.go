package include

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

func TestFenceBytes(t *testing.T) {
	assert.Equal(t, []byte("```go"), fenceBytes("```go"))
	assert.Empty(t, fenceBytes(""))
	assert.Zero(t, testing.AllocsPerRun(100, func() { _ = fenceBytes("```go") }))
}

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

func TestOpensFence(t *testing.T) {
	assert.True(t, opensFence("```go"))
	assert.True(t, opensFence("~~~~ `x`"))
	assert.True(t, opensFence("    ```"), "any indentation is stripped")
	assert.True(t, opensFence("\t~~~"), "tab indentation is stripped")
	assert.False(t, opensFence("```x```"), "backtick in backtick info string")
	assert.False(t, opensFence("``"))
	assert.False(t, opensFence("text"))
	assert.False(t, opensFence(""))
}
