package archetypes

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fsWith(files map[string]string) fs.FS {
	m := fstest.MapFS{}
	for p, body := range files {
		m[p] = &fstest.MapFile{Data: []byte(body)}
	}
	return m
}

func TestResolver_ListDefaultRoot(t *testing.T) {
	r := &Resolver{FS: fsWith(map[string]string{
		"archetypes/story.md": "# ?",
		"archetypes/prd.md":   "# ?",
	})}
	entries := r.List()
	require.Len(t, entries, 2)
	assert.Equal(t, "prd", entries[0].Name)
	assert.Equal(t, "archetypes/prd.md", entries[0].Path)
	assert.Equal(t, "story", entries[1].Name)
}

func TestResolver_ListMultipleRootsEarlierShadows(t *testing.T) {
	r := &Resolver{
		Roots: []string{"custom", "archetypes"},
		FS: fsWith(map[string]string{
			"archetypes/prd.md":   "# default",
			"custom/prd.md":       "# custom",
			"archetypes/story.md": "# story",
		}),
	}
	entries := r.List()
	require.Len(t, entries, 2)
	assert.Equal(t, "prd", entries[0].Name)
	assert.Equal(t, "custom/prd.md", entries[0].Path)
	assert.Equal(t, "story", entries[1].Name)
	assert.Equal(t, "archetypes/story.md", entries[1].Path)
}

func TestResolver_ListSkipsNonMarkdownAndDirs(t *testing.T) {
	r := &Resolver{FS: fsWith(map[string]string{
		"archetypes/README.txt":    "notes",
		"archetypes/story.md":      "# story",
		"archetypes/sub/nested.md": "# nested",
	})}
	entries := r.List()
	require.Len(t, entries, 1)
	assert.Equal(t, "story", entries[0].Name)
}

func TestResolver_LookupReturnsEntry(t *testing.T) {
	r := &Resolver{FS: fsWith(map[string]string{
		"archetypes/story.md": "# story",
	})}
	e, err := r.Lookup("story")
	require.NoError(t, err)
	assert.Equal(t, "story", e.Name)
	assert.Equal(t, "archetypes/story.md", e.Path)
}

func TestResolver_LookupEmptyName(t *testing.T) {
	r := &Resolver{}
	_, err := r.Lookup("")
	require.Error(t, err)
}

func TestResolver_LookupMissingListsRoots(t *testing.T) {
	r := &Resolver{Roots: []string{"a", "b"}, FS: fsWith(map[string]string{})}
	_, err := r.Lookup("none")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no archetypes found")
	assert.Contains(t, err.Error(), "a, b")
}

func TestResolver_LookupMissingListsSiblings(t *testing.T) {
	r := &Resolver{FS: fsWith(map[string]string{
		"archetypes/story.md": "# story",
		"archetypes/prd.md":   "# prd",
	})}
	_, err := r.Lookup("missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "available under roots")
	assert.Contains(t, err.Error(), "prd")
	assert.Contains(t, err.Error(), "story")
}

func TestResolver_Content(t *testing.T) {
	r := &Resolver{FS: fsWith(map[string]string{
		"archetypes/story.md": "# story body",
	})}
	b, err := r.Content("story")
	require.NoError(t, err)
	assert.Equal(t, "# story body", string(b))
}

func TestResolver_ContentMissing(t *testing.T) {
	r := &Resolver{}
	_, err := r.Content("x")
	require.Error(t, err)
}

func TestResolver_AbsPathJoinsRootDir(t *testing.T) {
	r := &Resolver{
		RootDir: "/home/me/proj",
		FS: fsWith(map[string]string{
			"archetypes/story.md": "# story",
		}),
	}
	p, err := r.AbsPath("story")
	require.NoError(t, err)
	assert.Equal(t, "/home/me/proj/archetypes/story.md", p)
}

func TestResolver_AbsPathNoRootDir(t *testing.T) {
	r := &Resolver{FS: fsWith(map[string]string{
		"archetypes/story.md": "# story",
	})}
	p, err := r.AbsPath("story")
	require.NoError(t, err)
	assert.Equal(t, "archetypes/story.md", p)
}

func TestDefaultRoot(t *testing.T) {
	assert.Equal(t, "archetypes", DefaultRoot)
	r := &Resolver{FS: fsWith(map[string]string{
		"archetypes/x.md": "# x",
	})}
	entries := r.List()
	require.Len(t, entries, 1)
}

func TestResolver_LookupSkipsDirectoryMatch(t *testing.T) {
	// A directory named "story.md" under the root must not be treated
	// as the archetype file.
	m := fstest.MapFS{
		"archetypes/story.md/placeholder": &fstest.MapFile{Data: []byte("x")},
	}
	r := &Resolver{FS: m}
	_, err := r.Lookup("story")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unknown archetype")
}
