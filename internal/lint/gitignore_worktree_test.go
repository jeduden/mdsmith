package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewGitignoreMatcher_StopsAtWorktreeBoundary pins the worktree
// catalog-emptying bug at the gitignore layer.
//
// Layout mirrors a Git worktree nested under an ignored path of its
// superproject:
//
//	<repo>/.git                         (dir → repo is a working tree)
//	<repo>/.gitignore                   contains ".claude/worktrees/"
//	<repo>/.claude/worktrees/agent-x/   the worktree root
//	<repo>/.claude/worktrees/agent-x/.git   (file → worktree boundary)
//	<repo>/.claude/worktrees/agent-x/plan/foo.md
//
// A matcher built at the worktree root must NOT classify files inside
// the worktree as ignored just because the worktree's own absolute path
// contains a segment (".claude/worktrees") that the superproject's
// .gitignore matches. Git itself does not apply the superproject's
// ignore rules across the working-tree boundary, and neither must
// mdsmith — otherwise a `plan/*.md` catalog glob resolves to zero files
// and `fix` empties the section.
func TestNewGitignoreMatcher_StopsAtWorktreeBoundary(t *testing.T) {
	repo := t.TempDir()
	// Repo is a working tree: .git is a directory.
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"),
		[]byte(".claude/worktrees/\n"), 0o644))

	wt := filepath.Join(repo, ".claude", "worktrees", "agent-x")
	require.NoError(t, os.MkdirAll(filepath.Join(wt, "plan"), 0o755))
	// Worktree boundary: .git is a file ("gitdir: ...").
	require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"),
		[]byte("gitdir: "+filepath.Join(repo, ".git", "worktrees", "agent-x")+"\n"),
		0o644))

	planFile := filepath.Join(wt, "plan", "foo.md")
	require.NoError(t, os.WriteFile(planFile, []byte("# foo\n"), 0o644))

	m := NewGitignoreMatcher(wt)
	require.NotNil(t, m)

	// The file itself must not be ignored.
	assert.False(t, m.IsIgnored(planFile, false),
		"plan/foo.md inside the worktree must not be ignored by the superproject's .claude/worktrees/ rule")

	// No ancestor directory of the file (within the worktree, and the
	// worktree root and its ancestors) may be reported as ignored,
	// because catalog's gitignore check walks ancestors with isDir=true.
	for dir := filepath.Dir(planFile); ; dir = filepath.Dir(dir) {
		assert.False(t, m.IsIgnored(dir, true),
			"ancestor %q must not be ignored from a worktree-rooted matcher", dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
}

// TestNewGitignoreMatcher_AncestorGitignoreStillAppliesWithoutBoundary
// guards the normal (non-worktree) case: when the matcher root is an
// ordinary subdirectory of a repo — with NO intervening .git marker —
// an ancestor .gitignore must still apply. This is the behavior the
// worktree fix must preserve, so the fix cannot simply drop all
// ancestor .gitignore collection.
func TestNewGitignoreMatcher_AncestorGitignoreStillAppliesWithoutBoundary(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"),
		[]byte("*.log\n"), 0o644))

	sub := filepath.Join(repo, "docs", "deep")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	logFile := filepath.Join(sub, "out.log")
	require.NoError(t, os.WriteFile(logFile, []byte("log"), 0o644))

	// Matcher rooted at the subdirectory; the ancestor .gitignore's
	// "*.log" rule must still be collected and applied.
	m := NewGitignoreMatcher(sub)
	require.NotNil(t, m)
	assert.True(t, m.IsIgnored(logFile, false),
		"ancestor .gitignore *.log rule should still apply when no working-tree boundary intervenes")
}

// TestNewGitignoreMatcher_StopsAtAncestorWorktreeBoundary covers the
// ancestor-walk `break`: the matcher root is an ordinary subdirectory,
// but an ancestor between it and the superproject is itself a worktree
// root (a `.git` file). The walk must stop at that inner worktree root,
// so the superproject's .gitignore above it is never collected.
func TestNewGitignoreMatcher_StopsAtAncestorWorktreeBoundary(t *testing.T) {
	super := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(super, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(super, ".gitignore"),
		[]byte("*.log\n"), 0o644))

	// Inner worktree root nested below the superproject: .git is a file.
	wt := filepath.Join(super, "nested", "wt")
	require.NoError(t, os.MkdirAll(wt, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"),
		[]byte("gitdir: /elsewhere\n"), 0o644))

	// Matcher root is a plain subdirectory of the inner worktree.
	sub := filepath.Join(wt, "a", "b")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	logFile := filepath.Join(sub, "out.log")
	require.NoError(t, os.WriteFile(logFile, []byte("log"), 0o644))

	m := NewGitignoreMatcher(sub)
	require.NotNil(t, m)
	assert.False(t, m.IsIgnored(logFile, false),
		"the ancestor walk must stop at the inner worktree root, so the "+
			"superproject's *.log rule is never inherited across it")
}
