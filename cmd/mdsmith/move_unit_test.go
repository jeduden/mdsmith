package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMoveFlags(t *testing.T) {
	opts, pos, err := parseMoveFlags([]string{"--dry-run", "a.md", "b.md"})
	require.NoError(t, err)
	assert.True(t, opts.dryRun)
	assert.Equal(t, []string{"a.md", "b.md"}, pos)

	_, _, err = parseMoveFlags([]string{"--unknown"})
	require.Error(t, err)
}

func TestRunMove_ArgValidation(t *testing.T) {
	renameWorkspace(t)
	// --help is a pflag ErrHelp → exit 0.
	assert.Equal(t, 0, runMove([]string{"--help"}))
	// An unknown flag is a non-help parse error → exit 2.
	assert.Equal(t, 2, runMove([]string{"--bogus", "a.md", "b.md"}))
	// Wrong positional count.
	assert.Equal(t, 2, runMove([]string{"a.md"}))
	// Not workspace-relative.
	assert.Equal(t, 2, runMove([]string{"a.md", "/abs/b.md"}))
	assert.Equal(t, 2, runMove([]string{"../evil.md", "b.md"}))
}

func TestRunMove_Success(t *testing.T) {
	dir := renameWorkspace(t)
	var code int
	out := captureStdout(func() {
		code = runMove([]string{"a.md", "docs/a.md"})
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "moved a.md -> docs/a.md")
	assert.NoFileExists(t, filepath.Join(dir, "a.md"))
	assert.FileExists(t, filepath.Join(dir, "docs", "a.md"))
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "docs/a.md#setup")
}

func TestRunMove_JSON(t *testing.T) {
	renameWorkspace(t)
	var code int
	out := captureStdout(func() {
		code = runMove([]string{"--format", "json", "a.md", "docs/a.md"})
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, out, `"from": "a.md"`)
	assert.Contains(t, out, `"to": "docs/a.md"`)
}

func TestRunMove_DryRunChangesNothing(t *testing.T) {
	dir := renameWorkspace(t)
	var code int
	out := captureStdout(func() {
		code = runMove([]string{"--dry-run", "a.md", "docs/a.md"})
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "would move a.md -> docs/a.md")
	assert.FileExists(t, filepath.Join(dir, "a.md"))
}

func TestRunMove_ExitCodes(t *testing.T) {
	renameWorkspace(t)
	// Destination already exists → exit 2.
	assert.Equal(t, 2, runMove([]string{"a.md", "b.md"}))
	// Missing source → exit 1.
	assert.Equal(t, 1, runMove([]string{"ghost.md", "x.md"}))
}

func TestRunMove_WorkspaceBuildFailure(t *testing.T) {
	renameWorkspace(t)
	// A missing config makes buildWorkspace return 2, which runMove
	// propagates.
	assert.Equal(t, 2, runMove([]string{"--config", "/no/such/.mdsmith.yml", "a.md", "b.md"}))
}

func TestRunMove_BasenameChangeRewritesWikilink(t *testing.T) {
	dir := renameWorkspace(t)
	// A vault-style wikilink to a.md's stem; moving changes the basename,
	// so the stem is rewritten (exercising the workspace wikilink query).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"), []byte("See [[a]] here.\n"), 0o644))
	assert.Equal(t, 0, runMove([]string{"a.md", "service.md"}))
	c, _ := os.ReadFile(filepath.Join(dir, "c.md"))
	assert.Contains(t, string(c), "[[service]]")
}

func TestApplyPlan_SkipsEmptyEditEntries(t *testing.T) {
	renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	// A keyed entry with no edits is skipped; with no FileOp the plan is a
	// clean no-op → exit 0.
	plan := refactor.Plan{Edits: map[string][]refactor.Edit{"a.md": {}}}
	assert.Equal(t, 0, applyPlan(io.Discard, ws, plan, "text", false))
}

func TestApplyPlan_FileOpFailureExits2(t *testing.T) {
	renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	// The destination does not exist, so the pre-flight passes, but the
	// source is missing, so FileOp.Execute's os.Rename fails → exit 2.
	plan := refactor.Plan{FileOp: &refactor.FileOp{From: "ghost.md", To: "moved.md"}}
	assert.Equal(t, 2, applyPlan(io.Discard, ws, plan, "text", false))
}

// TestApplyPlan_PreflightAbortsBeforeWritingEdits locks the finding-#1
// data-safety fix: when the destination already exists on disk (a
// collision the planner's read-based check can miss), applyPlan must
// abort before writing any reference edit, so a move that could never
// succeed does not leave the workspace with rewritten links pointing at
// a file that was never created.
func TestApplyPlan_PreflightAbortsBeforeWritingEdits(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "occupied.md"), []byte("# Keep\n"), 0o644))
	before, err := os.ReadFile(filepath.Join(dir, "b.md"))
	require.NoError(t, err)

	// The plan would rewrite b.md and move a.md onto the existing
	// occupied.md; the pre-flight must abort first.
	plan := refactor.Plan{
		Edits: map[string][]refactor.Edit{"b.md": {{
			Range: refactor.Range{
				Start: refactor.Position{Line: 0, Character: 0},
				End:   refactor.Position{Line: 0, Character: 3},
			},
			NewText: "XXX",
		}}},
		FileOp: &refactor.FileOp{From: "a.md", To: "occupied.md"},
	}
	assert.Equal(t, 2, applyPlan(io.Discard, ws, plan, "text", false))

	after, err := os.ReadFile(filepath.Join(dir, "b.md"))
	require.NoError(t, err)
	assert.Equal(t, before, after, "no reference edit is written when the pre-flight aborts")
}
