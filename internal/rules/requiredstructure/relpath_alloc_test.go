package requiredstructure

import (
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
)

// TestWorkspaceRelPathResolvesWorkingDirectoryOnce guards the "memoize
// per-input computations" rule in docs/development/high-performance-go.md:
// with a relative RootDir and Path, workspaceRelPath ran filepath.Abs
// twice, and each call asks the OS for the working directory. It runs
// for every file of a kind with path-patterns.
func TestWorkspaceRelPathResolvesWorkingDirectoryOnce(t *testing.T) {
	f := &lint.File{
		RootDir: "sub",
		Path:    filepath.Join("sub", "docs", "a.md"),
	}
	assert.Equal(t, "docs/a.md", workspaceRelPath(f))

	one := testing.AllocsPerRun(50, func() {
		_, _ = filepath.Abs("x")
	})
	got := testing.AllocsPerRun(50, func() { _ = workspaceRelPath(f) })
	// One working-directory lookup (plus the joins and the result),
	// not two.
	assert.LessOrEqual(t, got, one+3, "working directory resolved per path")
}

func TestAbsPair(t *testing.T) {
	root := t.TempDir()
	a, b := absPair(root, filepath.Join(root, "x", "..", "y.md"))
	assert.Equal(t, root, a)
	assert.Equal(t, filepath.Join(root, "y.md"), b)

	t.Chdir(root)
	a, b = absPair(".", "d/y.md")
	assert.Equal(t, filepath.Clean(root), a)
	assert.Equal(t, filepath.Join(root, "d", "y.md"), b)
}
