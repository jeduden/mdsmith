package include

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =====================================================================
// walkExtractPath: dotted-path navigation through extract JSON trees
// =====================================================================

func TestWalkExtractPath_TopLevelScalar(t *testing.T) {
	data := map[string]any{
		"tagline": map[string]any{"text": "Hello world"},
	}
	v, err := walkExtractPath(data, "tagline.text")
	require.NoError(t, err)
	assert.Equal(t, "Hello world", v)
}

func TestWalkExtractPath_FrontmatterScalar(t *testing.T) {
	data := map[string]any{
		"frontmatter": map[string]any{
			"title": "mdsmith product messaging",
		},
	}
	v, err := walkExtractPath(data, "frontmatter.title")
	require.NoError(t, err)
	assert.Equal(t, "mdsmith product messaging", v)
}

func TestWalkExtractPath_NestedObject(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "deep",
			},
		},
	}
	v, err := walkExtractPath(data, "a.b.c")
	require.NoError(t, err)
	assert.Equal(t, "deep", v)
}

func TestWalkExtractPath_MissingKey(t *testing.T) {
	data := map[string]any{
		"tagline": map[string]any{"text": "Hello"},
	}
	_, err := walkExtractPath(data, "missing.x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestWalkExtractPath_KeyOnNonObject(t *testing.T) {
	data := map[string]any{
		"a": "scalar",
	}
	_, err := walkExtractPath(data, "a.b")
	require.Error(t, err)
}

func TestWalkExtractPath_ObjectWithSingleContentKey(t *testing.T) {
	// A leaf object that carries a single well-known content key
	// (text/code/items/rows) splices the inner value.
	data := map[string]any{
		"tagline": map[string]any{"text": "Hello world"},
	}
	v, err := walkExtractPath(data, "tagline")
	require.NoError(t, err)
	assert.Equal(t, "Hello world", v)
}

func TestWalkExtractPath_ObjectWithCodeKey(t *testing.T) {
	data := map[string]any{
		"headline": map[string]any{"code": "Mark*down*, smithed."},
	}
	v, err := walkExtractPath(data, "headline")
	require.NoError(t, err)
	assert.Equal(t, "Mark*down*, smithed.", v)
}

func TestWalkExtractPath_AmbiguousObject(t *testing.T) {
	// An object with multiple keys and no single content key is
	// ambiguous; the caller must drill in further.
	data := map[string]any{
		"hero": map[string]any{
			"text": "main",
			"code": "extra",
		},
	}
	_, err := walkExtractPath(data, "hero")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestWalkExtractPath_EmptyPath(t *testing.T) {
	data := map[string]any{"a": "b"}
	_, err := walkExtractPath(data, "")
	require.Error(t, err)
}

func TestWalkExtractPath_NonStringScalar(t *testing.T) {
	// Numbers, bools, and similar scalars stringify via fmt.
	data := map[string]any{"count": 42}
	v, err := walkExtractPath(data, "count")
	require.NoError(t, err)
	assert.Equal(t, 42, v)
}
