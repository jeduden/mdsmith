package concepts_test

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookup_KnownConcept_ReturnsFrontMatterStripped(t *testing.T) {
	content, err := concepts.Lookup("placeholder-grammar")
	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(content, "---"),
		"front matter should be stripped")
	assert.Contains(t, content, "Placeholder grammar")
}

func TestLookup_UnknownConcept_ReturnsError(t *testing.T) {
	_, err := concepts.Lookup("no-such-concept")
	assert.ErrorContains(t, err, `unknown concept "no-such-concept"`)
}

func TestLookup_DoesNotStartWithBlankLine(t *testing.T) {
	content, err := concepts.Lookup("placeholder-grammar")
	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(content, "\n"),
		"content should not start with a blank line after front matter")
}
