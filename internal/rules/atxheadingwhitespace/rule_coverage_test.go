package atxheadingwhitespace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Targeted coverage tests for internal helpers that the public API cannot
// easily exercise through Check/Fix alone.

func TestExtractContent_EmptyString(t *testing.T) {
	assert.Equal(t, "", extractContent(""))
}

func TestExtractContent_WhitespaceOnly(t *testing.T) {
	assert.Equal(t, "", extractContent("   "))
}

func TestNormalizeLine_TooManyHashes(t *testing.T) {
	// 7 hashes exceed the ATX limit; normalizeLine returns the line unchanged.
	result := normalizeLine([]byte("####### Heading"))
	assert.Equal(t, "####### Heading", result)
}

func TestCheckClosingATX_AllHashContent(t *testing.T) {
	// When trimmed content is entirely '#', there is no closing-suffix defect.
	r := &Rule{}
	diags := r.checkClosingATX("test.md", 1, 0, 1, []byte("##"))
	assert.Empty(t, diags)
}
