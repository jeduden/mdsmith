package headingincrement

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Check with setext headings (exercises astutil.HeadingLine's Lines() branch) ---

func TestCheck_SetextHeadings_ProperIncrement(t *testing.T) {
	src := []byte("Title\n=====\n\nSection\n-------\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_SetextToATX_SkipsLevel(t *testing.T) {
	src := []byte("Title\n=====\n\n### H3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "incremented from 1 to 3")
}

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "heading", r.Category())
}
