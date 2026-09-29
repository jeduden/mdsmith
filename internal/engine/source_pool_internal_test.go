package engine

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
)

func TestDetachSourceLines(t *testing.T) {
	src := []byte("alpha\nbravo")
	diags := []lint.Diagnostic{
		{Line: 1, RuleID: "TST001"},
		{Line: 2, RuleID: "TST001", SourceLines: []string{
			lint.BytesView(src[0:5]), lint.BytesView(src[6:11]),
		}},
	}
	detachSourceLines(diags)
	copy(src, "zzzzz\nyyyyy")

	assert.Nil(t, diags[0].SourceLines)
	assert.Equal(t, []string{"alpha", "bravo"}, diags[1].SourceLines)
}

func TestCloneLines(t *testing.T) {
	src := []byte("alpha\n\ncharlie")
	lines := []string{
		lint.BytesView(src[0:5]), lint.BytesView(src[6:6]), lint.BytesView(src[7:14]),
	}
	got := cloneLines(lines)
	copy(src, "zzzzz\n\nyyyyyyy")
	assert.Equal(t, []string{"alpha", "", "charlie"}, got)

	if raceEnabled {
		return // the race detector's bookkeeping inflates the count
	}
	// One allocation for the joined text and one for the string slice,
	// however many lines the window holds.
	allocs := testing.AllocsPerRun(100, func() { _ = cloneLines(lines) })
	assert.LessOrEqual(t, allocs, 2.0)
}
