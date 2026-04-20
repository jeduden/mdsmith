package archetypes

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList_ContainsBuiltins(t *testing.T) {
	names := List()
	assert.Contains(t, names, "story-file")
	assert.Contains(t, names, "prd")
	assert.Contains(t, names, "agent-definition")
	assert.Contains(t, names, "claude-md")
}

func TestList_Sorted(t *testing.T) {
	names := List()
	for i := 1; i < len(names); i++ {
		assert.Less(t, names[i-1], names[i])
	}
}

func TestLookup_ReturnsSchemaBytes(t *testing.T) {
	data, err := Lookup("story-file")
	require.NoError(t, err)
	assert.Contains(t, string(data), "## Background")
	assert.Contains(t, string(data), "## Acceptance Criteria")
}

func TestLookup_EmptyName(t *testing.T) {
	_, err := Lookup("")
	require.Error(t, err)
}

func TestLookup_UnknownNameListsAvailable(t *testing.T) {
	_, err := Lookup("not-real")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "story-file")
	assert.Contains(t, err.Error(), "unknown archetype")
}

func TestClassifyLookupError_MissingEntry(t *testing.T) {
	err := classifyLookupError("foo", fs.ErrNotExist, []string{"a", "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown archetype")
	assert.Contains(t, err.Error(), "a, b")
}

func TestClassifyLookupError_UnexpectedError(t *testing.T) {
	boom := errors.New("io failure")
	err := classifyLookupError("foo", boom, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading archetype")
	assert.True(t, errors.Is(err, boom))
}
