package include

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
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

func TestWalkExtractPath_EmptySegment(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "c"}}
	_, err := walkExtractPath(data, "a..b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty segment")
}

func TestWalkExtractPath_SingleNonContentKey(t *testing.T) {
	// One key that isn't a content key — caller must drill in.
	data := map[string]any{
		"section": map[string]any{"slug": "intro"},
	}
	_, err := walkExtractPath(data, "section")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a recognised content key")
}

func TestWalkExtractPath_MultiKeyWithExactlyOneContentKey(t *testing.T) {
	// Two keys; only one is a content key. pickContentKey returns
	// the content value rather than the ambiguous-object error.
	data := map[string]any{
		"section": map[string]any{
			"slug": "intro",
			"text": "Hello",
		},
	}
	v, err := walkExtractPath(data, "section")
	require.NoError(t, err)
	assert.Equal(t, "Hello", v)
}

// =====================================================================
// projectExtractValue: wraps walkExtractPath with the host projector
// =====================================================================

func TestProjectExtractValue_NoProjectorInstalled(t *testing.T) {
	prev := projectExtract
	SetExtractProjector(nil)
	t.Cleanup(func() { SetExtractProjector(prev) })

	host := &lint.File{}
	_, err := projectExtractValue(host, fstest.MapFS{}, "x.md", nil, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no extract projector is installed")
}

func TestProjectExtractValue_ProjectorErrorBubbles(t *testing.T) {
	prev := projectExtract
	SetExtractProjector(func(*lint.File, fs.FS, string, []byte) (any, error) {
		return nil, errors.New("boom")
	})
	t.Cleanup(func() { SetExtractProjector(prev) })

	_, err := projectExtractValue(
		&lint.File{}, fstest.MapFS{}, "x.md", nil, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract: boom")
}

func TestProjectExtractValue_NonObjectRoot(t *testing.T) {
	prev := projectExtract
	SetExtractProjector(func(*lint.File, fs.FS, string, []byte) (any, error) {
		return "scalar-at-root", nil
	})
	t.Cleanup(func() { SetExtractProjector(prev) })

	_, err := projectExtractValue(
		&lint.File{}, fstest.MapFS{}, "x.md", nil, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "produced string at root")
}

func TestProjectExtractValue_Success(t *testing.T) {
	prev := projectExtract
	SetExtractProjector(func(*lint.File, fs.FS, string, []byte) (any, error) {
		return map[string]any{
			"tagline": map[string]any{"text": "Hello world"},
		}, nil
	})
	t.Cleanup(func() { SetExtractProjector(prev) })

	got, err := projectExtractValue(
		&lint.File{}, fstest.MapFS{}, "x.md", nil, "tagline.text")
	require.NoError(t, err)
	assert.Equal(t, "Hello world", got)
}

// =====================================================================
// formatExtractValue: rendering leaves for include block bodies
// =====================================================================

func TestFormatExtractValue_Nil(t *testing.T) {
	assert.Equal(t, "", formatExtractValue(nil))
}

func TestFormatExtractValue_String(t *testing.T) {
	assert.Equal(t, "hello", formatExtractValue("hello"))
}

func TestFormatExtractValue_List(t *testing.T) {
	got := formatExtractValue([]any{"one", "two", "three"})
	assert.Equal(t, "- one\n- two\n- three", got)
}

func TestFormatExtractValue_NumberFallthrough(t *testing.T) {
	assert.Equal(t, "42", formatExtractValue(42))
	assert.Equal(t, "true", formatExtractValue(true))
}
