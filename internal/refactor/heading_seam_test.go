package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/index"
)

// headingOnlyWorkspace answers only the questions a heading rename
// asks: no path or wikilink edges, and no WikilinkIndex.
type headingOnlyWorkspace struct{ files map[string]string }

func (w headingOnlyWorkspace) IncomingAnchorEdges(string, string) []index.Edge { return nil }

func (w headingOnlyWorkspace) Files() []string {
	out := make([]string, 0, len(w.files))
	for f := range w.files {
		out = append(out, f)
	}
	return out
}

func (w headingOnlyWorkspace) Resolve(file string) (string, []byte, bool) {
	src, ok := w.files[file]
	return file, []byte(src), ok
}

// TestHeadingAcceptsWorkspaceWithoutWikilinkIndex locks the seam split:
// refactor.Heading takes a workspace that has no WikilinkIndex (or any
// other move-only) method, so a heading-rename surface never has to
// build one.
func TestHeadingAcceptsWorkspaceWithoutWikilinkIndex(t *testing.T) {
	t.Parallel()
	src := "# Old\n"
	ws := headingOnlyWorkspace{files: map[string]string{"a.md": src}}
	p, err := Heading(ws, "a.md", "a.md", []byte(src), 1, "Old", "New")
	require.NoError(t, err)
	require.Len(t, p.Edits["a.md"], 1)
	assert.Equal(t, "New", p.Edits["a.md"][0].NewText)
}
