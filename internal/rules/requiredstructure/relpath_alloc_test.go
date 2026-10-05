package requiredstructure

import (
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
)

// TestWorkspaceRelPathSkipsWorkingDirectory guards the "memoize
// per-input computations" rule in docs/development/high-performance-go.md:
// a relative RootDir and Path share a base, so workspaceRelPath needs no
// filepath.Abs (one os.Getwd syscall each, twice per file of a kind with
// path-patterns). filepath.Abs("x") alone costs several allocations.
func TestWorkspaceRelPathSkipsWorkingDirectory(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not stable under -race")
	}
	f := &lint.File{
		RootDir: "sub",
		Path:    filepath.Join("sub", "docs", "a.md"),
	}
	assert.Equal(t, "docs/a.md", workspaceRelPath(f))

	got := testing.AllocsPerRun(50, func() { _ = workspaceRelPath(f) })
	assert.LessOrEqual(t, got, 2.0, "working directory resolved per path")
}

// TestWorkspaceRelPathUnrelatableRelativePair covers a relative pair
// that Rel alone cannot relate (a root of ".."): it falls back to Abs.
func TestWorkspaceRelPathUnrelatableRelativePair(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	f := &lint.File{RootDir: "..", Path: "a.md"}
	assert.Equal(t, filepath.ToSlash(filepath.Join(filepath.Base(root), "a.md")),
		workspaceRelPath(f))
}

// TestWorkspaceRelPathMixedAbsoluteAndRelative covers an absolute root
// with a relative path, which needs Abs to share a base.
func TestWorkspaceRelPathMixedAbsoluteAndRelative(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	f := &lint.File{RootDir: root, Path: filepath.Join("docs", "a.md")}
	assert.Equal(t, "docs/a.md", workspaceRelPath(f))
}
