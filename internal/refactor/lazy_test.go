package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLazyWorkspace_BuildsOnceOnFirstUse(t *testing.T) {
	builds := 0
	lw := NewLazyWorkspace(func() Workspace {
		builds++
		return newMemWorkspace(map[string]string{
			"a.md": "# Setup\n",
			"b.md": "See [go](a.md#setup) and [[a]].\n",
		})
	})
	assert.Zero(t, builds, "nothing is built up front")
	assert.ElementsMatch(t, []string{"a.md", "b.md"}, lw.Files())
	assert.Len(t, lw.IncomingAnchorEdges("a.md", "setup"), 1)
	assert.Len(t, lw.IncomingPathEdges("a.md"), 1)
	assert.Len(t, lw.IncomingWikilinkEdges("a"), 1)
	key, src, ok := lw.Resolve("a.md")
	assert.True(t, ok)
	assert.Equal(t, "a.md", key)
	assert.Equal(t, "# Setup\n", string(src))
	assert.Equal(t, 1, builds, "every method shares one build")
}

// A label rename never consults the workspace, so a lazy one is never
// built.
func TestRename_LabelNeverBuildsLazyWorkspace(t *testing.T) {
	lw := NewLazyWorkspace(func() Workspace {
		t.Fatal("a label rename built the workspace")
		return nil
	})
	_, err := Rename(lw, "a.md", []byte(dispatchSrc), "", "docs", "rfc")
	assert.NoError(t, err)
}
