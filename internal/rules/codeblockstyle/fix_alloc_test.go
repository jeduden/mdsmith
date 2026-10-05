package codeblockstyle

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixIndentedToFencedAllocBudget guards the "stay in []byte" rule in
// docs/development/high-performance-go.md: Fix must not copy every
// source line into a string and join them again. A 3000-line file with
// one indented block should cost a handful of allocations, not one per
// line.
func TestFixIndentedToFencedAllocBudget(t *testing.T) {
	src := "# T\n\n    indented()\n    more()\n\n" +
		strings.Repeat("plain prose line\n\n", 1500)
	f, err := lint.NewFile("t.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	out := r.Fix(f)
	require.Contains(t, string(out), "```text\nindented()\nmore()\n```")

	allocs := testing.AllocsPerRun(20, func() { _ = r.Fix(f) })
	assert.LessOrEqual(t, allocs, 6.0,
		"Fix allocated per source line")
}
