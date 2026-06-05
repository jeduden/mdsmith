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
	// (text/code/inline/items/rows) splices the inner value.
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

func TestWalkExtractPath_ObjectWithInlineKey(t *testing.T) {
	// An inline-projected paragraph wraps its spans under `inline`;
	// walkExtractPath unwraps it like the other content keys, so
	// `<?include extract: headline?>` behaves like a text projection.
	spans := []any{
		map[string]any{"span": "text", "value": "Mark"},
		map[string]any{"span": "text", "value": ", smithed."},
	}
	data := map[string]any{
		"headline": map[string]any{"inline": spans},
	}
	v, err := walkExtractPath(data, "headline")
	require.NoError(t, err)
	assert.Equal(t, spans, v)
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

func TestWalkExtractPath_TextAndInlineSiblingsAmbiguous(t *testing.T) {
	// A scope with both a text-projected and an inline-projected
	// paragraph yields {text, inline}; both are content keys (plan
	// 212), so `extract: <section>` without a leaf is ambiguous and
	// the user must spell out .text or .inline rather than silently
	// getting the text sibling.
	data := map[string]any{
		"section": map[string]any{
			"text":   "Hello",
			"inline": []any{map[string]any{"span": "text", "value": "Hi"}},
		},
	}
	_, err := walkExtractPath(data, "section")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

// =====================================================================
// projectExtractValue: wraps walkExtractPath with the host projector
// =====================================================================

func TestProjectExtractValue_NoProjectorInstalled(t *testing.T) {
	prev := getExtractProjector()
	SetExtractProjector(nil)
	t.Cleanup(func() { SetExtractProjector(prev) })

	host := &lint.File{}
	_, err := projectExtractValue(host, fstest.MapFS{}, "x.md", nil, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no extract projector is installed")
}

func TestProjectExtractValue_ProjectorErrorBubbles(t *testing.T) {
	prev := getExtractProjector()
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
	prev := getExtractProjector()
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
	prev := getExtractProjector()
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
	got, err := formatExtractValue(nil)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestFormatExtractValue_String(t *testing.T) {
	got, err := formatExtractValue("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", got)
}

func TestFormatExtractValue_List(t *testing.T) {
	got, err := formatExtractValue([]any{"one", "two", "three"})
	require.NoError(t, err)
	assert.Equal(t, "- one\n- two\n- three", got)
}

func TestFormatExtractValue_NumberFallthrough(t *testing.T) {
	gotInt, err := formatExtractValue(42)
	require.NoError(t, err)
	assert.Equal(t, "42", gotInt)
	gotBool, err := formatExtractValue(true)
	require.NoError(t, err)
	assert.Equal(t, "true", gotBool)
}

// TestFormatExtractValue_MapRejected guards against a future
// regression where a map leaf would render via fmt.Sprint as Go
// syntax ("map[k:v]") and be spliced into a host README. A leaf at
// the path the user requested must be a scalar or list; objects are
// the user's signal to add another path segment.
func TestFormatExtractValue_MapRejected(t *testing.T) {
	_, err := formatExtractValue(map[string]any{"a": 1, "b": 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "leaf is an object")
	// Keys are sorted so the diagnostic is deterministic across runs.
	assert.Contains(t, err.Error(), "[a b]")
}

func TestFormatExtractValue_UnsupportedType(t *testing.T) {
	type unknown struct{ X int }
	_, err := formatExtractValue(unknown{X: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}

// TestFormatExtractValue_ListOfMapsRejected makes sure list items
// that are themselves objects are refused with a clear "list item N
// is an object" diagnostic — splicing them would otherwise emit Go-
// syntax garbage ("map[k:v]") into the host body.
func TestFormatExtractValue_ListOfMapsRejected(t *testing.T) {
	_, err := formatExtractValue([]any{
		"first",
		map[string]any{"k": "v"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list item 1 is an object")
}

// TestFormatExtractValue_NestedListRejected guards the malformed-
// Markdown case where a nested []any would render as `- - inner`
// (a bullet whose body starts with `- ` rather than a child list).
// The rule refuses the shape; the user must drill into a more
// specific path or expose the inner list at a different leaf.
func TestFormatExtractValue_NestedListRejected(t *testing.T) {
	_, err := formatExtractValue([]any{
		"top",
		[]any{"sub1", "sub2"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list item 1 is a nested list")
}

// TestFormatExtractValue_ListItemUnsupportedTypeBubbles drives the
// per-item recursion's error-return path: a list whose entry is a
// struct (or any other type the leaf-format switch refuses) must
// surface as a "list item N" wrapped error rather than silently
// stringifying via fmt.Sprint.
func TestFormatExtractValue_ListItemUnsupportedTypeBubbles(t *testing.T) {
	type unknown struct{ X int }
	_, err := formatExtractValue([]any{"first", unknown{X: 1}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list item 1")
	assert.Contains(t, err.Error(), "unsupported type")
}

// TestProjectExtractValue_MapLeafBubblesError drives the
// projectExtractValue error site that wraps formatExtractValue's
// error with the resolvedFile + dottedPath context. We feed a
// projector that returns a tree whose path resolves to a map leaf
// — formatExtractValue then refuses to splice the map and the
// wrapper makes the diagnostic actionable.
func TestProjectExtractValue_MapLeafBubblesError(t *testing.T) {
	prev := getExtractProjector()
	t.Cleanup(func() { SetExtractProjector(prev) })
	SetExtractProjector(func(
		_ *lint.File, _ fs.FS, _ string, _ []byte,
	) (any, error) {
		// "text" is a content key, so walkExtractPath unwraps via
		// pickContentKey and hands formatExtractValue the inner map
		// (which it then refuses).
		return map[string]any{
			"section": map[string]any{
				"text": map[string]any{"a": 1, "b": 2},
			},
		}, nil
	})

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)

	_, err = projectExtractValue(
		host, fstest.MapFS{}, "docs/target.md",
		[]byte(""), "section")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `extract "docs/target.md" at "section"`)
	assert.Contains(t, err.Error(), "leaf is an object")
}
