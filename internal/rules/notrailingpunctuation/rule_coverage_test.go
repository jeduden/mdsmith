package notrailingpunctuation

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Check with setext heading having trailing punctuation ---

func TestCheck_SetextWithPunctuation(t *testing.T) {
	src := []byte("Title.\n======\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS017", diags[0].RuleID)
	assert.Equal(t, 1, diags[0].Line)
}

// --- Check with empty heading ---

func TestCheck_EmptyHeading_NoDiagnostic(t *testing.T) {
	src := []byte("#\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "heading", r.Category())
}
