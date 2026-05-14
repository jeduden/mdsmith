package fencedcodestyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{Style: "backtick"}
	assert.Equal(t, "code", r.Category())
}

// --- replaceFenceChars with leading spaces ---

func TestReplaceFenceChars_LeadingSpaces(t *testing.T) {
	// A fence line with leading spaces: "  ~~~go" -> "  ```go"
	line := []byte("  ~~~go")
	result := replaceFenceChars(line, '`')
	assert.Equal(t, []byte("  ```go"), result)
}

// --- Fix with empty block after paragraph (exercises previousSibling path) ---

func TestFix_EmptyTildeBlockAfterParagraph(t *testing.T) {
	src := []byte("paragraph\n\n~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	assert.Equal(t, "paragraph\n\n```\n```\n", string(result))
}
