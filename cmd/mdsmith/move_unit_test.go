package main

import (
	"bytes"
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
	assert.Equal(t, 2, runMove([]string{`sub\..\..\evil.md`, "b.md"}))
}

// TestRunMove_BackslashDestination pins that a backslash destination
// is moved to the same forward-slash path it was validated as, on
// every host, rather than to a file literally named `docs\a.md`.
func TestRunMove_BackslashDestination(t *testing.T) {
	dir := renameWorkspace(t)
	var code int
	out := captureStdout(func() {
		code = runMove([]string{"a.md", `docs\a.md`})
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "moved a.md -> docs/a.md")
	assert.FileExists(t, filepath.Join(dir, "docs", "a.md"))
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

// badSecondFilePlan edits a.md validly and b.md with two overlapping
// edits, which refactor.ApplyEdits rejects. Keys apply in sorted order,
// so a.md is the file a non-atomic apply would already have rewritten
// when b.md fails.
func badSecondFilePlan() refactor.Plan {
	edit := func(start, end int, text string) refactor.Edit {
		return refactor.Edit{
			Range: refactor.Range{
				Start: refactor.Position{Line: 0, Character: start},
				End:   refactor.Position{Line: 0, Character: end},
			},
			NewText: text,
		}
	}
	return refactor.Plan{Edits: map[string][]refactor.Edit{
		"a.md": {edit(2, 7, "Install")},
		"b.md": {edit(0, 3, "X"), edit(1, 4, "Y")},
	}}
}

// TestApplyPlan_RejectedEditWritesNothing pins that applyPlan splices
// every file before writing any: when ApplyEdits rejects one file's
// edits, the command exits 2 and no file — not even one that sorts
// before the bad one — is rewritten.
func TestApplyPlan_RejectedEditWritesNothing(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	beforeA, err := os.ReadFile(filepath.Join(dir, "a.md"))
	require.NoError(t, err)
	beforeB, err := os.ReadFile(filepath.Join(dir, "b.md"))
	require.NoError(t, err)

	assert.Equal(t, 2, applyPlan(io.Discard, ws, badSecondFilePlan(), "text", false))

	afterA, err := os.ReadFile(filepath.Join(dir, "a.md"))
	require.NoError(t, err)
	afterB, err := os.ReadFile(filepath.Join(dir, "b.md"))
	require.NoError(t, err)
	assert.Equal(t, beforeA, afterA, "a.md is not rewritten when b.md's edits are rejected")
	assert.Equal(t, beforeB, afterB)
}

// TestApplyPlan_DryRunReportsRejectedEdit pins that --dry-run splices
// the edits too, so it fails on a plan the real run would reject
// instead of reporting edits that could never be written.
func TestApplyPlan_DryRunReportsRejectedEdit(t *testing.T) {
	renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	var out bytes.Buffer
	assert.Equal(t, 2, applyPlan(&out, ws, badSecondFilePlan(), "text", true))
	assert.Empty(t, out.String(), "no edit summary is printed for a rejected plan")
}

func TestSpliceFileEdits(t *testing.T) {
	renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	plan := badSecondFilePlan()

	t.Run("splices the edits into the file bytes", func(t *testing.T) {
		out, code := spliceFileEdits(ws, "a.md", plan.Edits["a.md"])
		require.Equal(t, 0, code)
		assert.Equal(t, "# Install\n\nBody.\n", string(out))
	})
	t.Run("unreadable file exits 2", func(t *testing.T) {
		out, code := spliceFileEdits(ws, "missing.md", plan.Edits["a.md"])
		assert.Equal(t, 2, code)
		assert.Nil(t, out)
	})
	t.Run("rejected edits exit 2", func(t *testing.T) {
		out, code := spliceFileEdits(ws, "b.md", plan.Edits["b.md"])
		assert.Equal(t, 2, code)
		assert.Nil(t, out)
	})
}

func TestWriteRelFile(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "mapped.md")
	require.NoError(t, os.WriteFile(abs, []byte("old\n"), 0o640))
	ws := cliRenameWorkspace{relToAbs: map[string]string{"a.md": abs}, rootDir: dir}

	t.Run("writes through relToAbs keeping the mode", func(t *testing.T) {
		require.Equal(t, 0, writeRelFile(ws, "a.md", []byte("new\n")))
		got, err := os.ReadFile(abs)
		require.NoError(t, err)
		assert.Equal(t, "new\n", string(got))
		info, err := os.Stat(abs)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	})
	t.Run("unmapped rel falls back to rootDir", func(t *testing.T) {
		require.Equal(t, 0, writeRelFile(ws, "b.md", []byte("b\n")))
		got, err := os.ReadFile(filepath.Join(dir, "b.md"))
		require.NoError(t, err)
		assert.Equal(t, "b\n", string(got))
	})
	t.Run("write failure exits 2", func(t *testing.T) {
		// The rootDir fallback names a directory, so the rename over it
		// fails without relying on permission bits (tests run as root).
		require.NoError(t, os.Mkdir(filepath.Join(dir, "adir"), 0o755))
		assert.Equal(t, 2, writeRelFile(ws, "adir", []byte("x\n")))
	})
}
