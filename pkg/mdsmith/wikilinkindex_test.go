package mdsmith

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionWikilinkIndex locks that the session hands out the
// wikilink index its run cache holds — the one MDS027 built during a
// Check — rather than walking the root again, and that
// InvalidateWikilinks makes the next call walk afresh.
func TestSessionWikilinkIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(rel, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o600))
	}
	write("guide.md", "# Guide\n")
	write("old.md", "# Old\n")
	s, err := NewSession(SessionOptions{
		Workspace: OSWorkspace{Root: dir},
		Config: ConfigYAML("rules:\n  cross-file-reference-integrity:\n" +
			"    wikilinks: true\n"),
	})
	require.NoError(t, err)
	defer s.Dispose()

	_, err = s.Check("guide.md", []byte("# Guide\n\nSee [[old]].\n"))
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(dir, "old.md")))

	idx := s.WikilinkIndex()
	require.NotNil(t, idx)
	assert.Equal(t, []string{"old.md"}, idx.StemPaths("old"),
		"the index MDS027 cached is served without a fresh walk")
	assert.Same(t, idx, s.WikilinkIndex())

	s.InvalidateWikilinks()
	assert.Empty(t, s.WikilinkIndex().StemPaths("old"),
		"after invalidation the index is walked afresh")
}
