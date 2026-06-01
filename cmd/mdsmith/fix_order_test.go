package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrderFilesLeavesFirst verifies the CLI reorders the fix file list
// so an <?include?> chain is fixed leaves-first. The walk hands files
// in sorted order (a_top before m_mid before z_leaf), which is exactly
// the order that leaves the includer embedding a stale upstream; the
// dependency order must flip it to z_leaf, m_mid, a_top.
func TestOrderFilesLeavesFirst(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		return p
	}
	top := write("a_top.md", "# Top\n\n<?include\nfile: m_mid.md\n?>\n<?/include?>\n")
	mid := write("m_mid.md", "# Mid\n\n<?include\nfile: z_leaf.md\n?>\n<?/include?>\n")
	leaf := write("z_leaf.md", "# Leaf\n")

	in := []string{top, mid, leaf} // sorted (walk) order: includer first
	got := orderFilesLeavesFirst(in, dir, lint.DefaultMaxInputBytes)

	require.ElementsMatch(t, in, got, "must return the same set of paths")
	pos := make(map[string]int, len(got))
	for i, p := range got {
		pos[p] = i
	}
	assert.Less(t, pos[leaf], pos[mid], "leaf must be fixed before mid")
	assert.Less(t, pos[mid], pos[top], "mid must be fixed before top")
}

// TestOrderFilesLeavesFirst_ShortInput exercises the fast path: a
// single file has nothing to order and is returned unchanged.
func TestOrderFilesLeavesFirst_ShortInput(t *testing.T) {
	in := []string{"/ws/only.md"}
	assert.Equal(t, in, orderFilesLeavesFirst(in, "/ws", lint.DefaultMaxInputBytes))
}

// TestOrderFilesLeavesFirst_DuplicateWorkspacePath verifies the
// fallback when two inputs collapse to the same workspace-relative
// path: the helper returns the caller's list unchanged rather than
// dropping a file when mapping the ordered relatives back to absolutes.
func TestOrderFilesLeavesFirst_DuplicateWorkspacePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	in := []string{p, p} // both normalize to the same workspace path
	got := orderFilesLeavesFirst(in, dir, lint.DefaultMaxInputBytes)
	assert.Equal(t, in, got, "duplicate workspace path must fall back to input order")
}
