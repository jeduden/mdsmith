package engine

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterGeneratedDiags_EmptyRanges(t *testing.T) {
	diags := []lint.Diagnostic{
		{Line: 3, Message: "keep me"},
	}
	got := filterGeneratedDiags(diags, nil)
	assert.Len(t, got, 1, "no filtering with empty ranges")
}

func TestFilterGeneratedDiags_DropInRange(t *testing.T) {
	ranges := []lint.LineRange{{From: 5, To: 8}}
	diags := []lint.Diagnostic{
		{Line: 4, Message: "before"},
		{Line: 5, Message: "start of range"},
		{Line: 6, Message: "middle"},
		{Line: 8, Message: "end of range"},
		{Line: 9, Message: "after"},
	}
	got := filterGeneratedDiags(diags, ranges)
	require.Len(t, got, 2, "expected 2 diagnostics outside range")
	assert.Equal(t, "before", got[0].Message)
	assert.Equal(t, "after", got[1].Message)
}

func TestFilterGeneratedDiags_MultipleRanges(t *testing.T) {
	ranges := []lint.LineRange{{From: 3, To: 4}, {From: 8, To: 10}}
	diags := []lint.Diagnostic{
		{Line: 2, Message: "keep"},
		{Line: 3, Message: "drop"},
		{Line: 9, Message: "drop"},
		{Line: 11, Message: "keep"},
	}
	got := filterGeneratedDiags(diags, ranges)
	require.Len(t, got, 2)
	assert.Equal(t, "keep", got[0].Message)
	assert.Equal(t, "keep", got[1].Message)
}
