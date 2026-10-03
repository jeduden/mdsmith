package main

import (
	"bytes"
	"errors"
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

// TestRunMove_GitignoredStemSiblingKeepsWikilink locks that the CLI's
// move guard reads a gitignored same-stem file, which discovery skips
// but `[[guide]]` still resolves to (archive/ sorts before docs/), so
// the link is left as written.
func TestRunMove_GitignoredStemSiblingKeepsWikilink(t *testing.T) {
	dir := renameWorkspace(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("archive/\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "archive", "guide.md"), []byte("# Old\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("# Guide\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"), []byte("See [[guide]] here.\n"), 0o644))
	assert.Equal(t, 0, runMove([]string{"docs/guide.md", "docs/manual.md"}))
	c, _ := os.ReadFile(filepath.Join(dir, "c.md"))
	assert.Equal(t, "See [[guide]] here.\n", string(c))
}

// TestRunMove_GitignoredLaterStemSiblingRewritesWikilink locks the other
// side: when the moved file sorts before a gitignored same-stem file,
// `[[a]]` resolves to the moved file, so it follows it to the new name.
func TestRunMove_GitignoredLaterStemSiblingRewritesWikilink(t *testing.T) {
	dir := renameWorkspace(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("archive/\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "archive", "a.md"), []byte("# Old\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"), []byte("See [[a]] here.\n"), 0o644))
	assert.Equal(t, 0, runMove([]string{"a.md", "service.md"}))
	c, _ := os.ReadFile(filepath.Join(dir, "c.md"))
	assert.Equal(t, "See [[service]] here.\n", string(c))
}

// TestCLIRenameWorkspace_WikilinkIndex locks that the CLI's wikilink
// index walks the whole root, gitignored files included, and that a
// workspace with no root builds none.
func TestCLIRenameWorkspace_WikilinkIndex(t *testing.T) {
	dir := renameWorkspace(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("archive/\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "archive", "a.md"), []byte("# Old\n"), 0o644))
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	assert.Equal(t, []string{"a.md", "archive/a.md"}, ws.WikilinkIndex().StemPaths("a"))
	assert.Nil(t, cliRenameWorkspace{}.WikilinkIndex())
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

// lineEdit is a single-line edit replacing UTF-16 units [start, end) on
// line (0-based) with text.
func lineEdit(line, start, end int, text string) refactor.Edit {
	return refactor.Edit{
		Range: refactor.Range{
			Start: refactor.Position{Line: line, Character: start},
			End:   refactor.Position{Line: line, Character: end},
		},
		NewText: text,
	}
}

// snapshotFiles reads each rel under dir and returns rel → contents, so
// a test can assert a failed apply left every file byte-identical.
func snapshotFiles(t *testing.T, dir string, rels ...string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(rels))
	for _, rel := range rels {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err)
		out[rel] = string(b)
	}
	return out
}

// assertNoTempFiles fails when a staged `*.tmp` sibling is left in dir.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, left, "no staged temp file may be left behind")
}

// twoFilePlan rewrites a.md's heading and b.md's first word; a.md sorts
// first, so it is the file a half-applied refactor would leave changed.
func twoFilePlan() refactor.Plan {
	return refactor.Plan{Edits: map[string][]refactor.Edit{
		"a.md": {lineEdit(0, 2, 7, "Install")},
		"b.md": {lineEdit(0, 0, 3, "Read")},
	}}
}

// TestApplyPlan_LaterSpliceFailureWritesNothing pins phase one of the
// all-or-nothing apply: every file's edits are spliced in memory before
// any write, so a splice error in a later file (b.md sorts after a.md)
// leaves the earlier file byte-identical instead of half-applying the
// refactor.
func TestApplyPlan_LaterSpliceFailureWritesNothing(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	before := snapshotFiles(t, dir, "a.md", "b.md")

	plan := twoFilePlan()
	// Line 99 is past b.md's end, so ApplyEdits rejects the edit.
	plan.Edits["b.md"] = []refactor.Edit{lineEdit(99, 0, 0, "x")}
	assert.Equal(t, 2, applyPlan(io.Discard, ws, plan, "text", false))
	assert.Equal(t, before, snapshotFiles(t, dir, "a.md", "b.md"))
	assertNoTempFiles(t, dir)
}

// TestApplyPlan_DryRunReportsSpliceFailure pins that --dry-run runs the
// same in-memory splice phase as a real run, so a plan that would fail
// fails the dry run too instead of reporting success.
func TestApplyPlan_DryRunReportsSpliceFailure(t *testing.T) {
	renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)

	plan := twoFilePlan()
	plan.Edits["b.md"] = []refactor.Edit{lineEdit(99, 0, 0, "x")}
	var out bytes.Buffer
	assert.Equal(t, 2, applyPlan(&out, ws, plan, "text", true))
	assert.Empty(t, out.String(), "a failed dry run prints no plan")
}

// TestApplyPlan_DryRunRunsDestinationPreflight pins that --dry-run also
// runs the existing-destination pre-flight, so it reports the collision
// a real run would abort on.
func TestApplyPlan_DryRunRunsDestinationPreflight(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "occupied.md"), []byte("# Keep\n"), 0o644))

	plan := refactor.Plan{FileOp: &refactor.FileOp{From: "a.md", To: "occupied.md"}}
	assert.Equal(t, 2, applyPlan(io.Discard, ws, plan, "text", true))
}

// TestApplyPlan_StageFailureWritesNothing pins the write phase's first
// half: every file's new bytes are staged to a temp sibling before any
// original is replaced, so a write error on a later file (a full disk,
// say) leaves every file byte-identical and removes the staged temps.
func TestApplyPlan_StageFailureWritesNothing(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	before := snapshotFiles(t, dir, "a.md", "b.md")

	calls := 0
	injectWriteFileFn(t, &writeFileWriteFnMu, &writeFileWriteFn,
		func(f *os.File, b []byte) (int, error) {
			calls++
			if calls == 2 {
				return 0, errors.New("no space left on device")
			}
			return f.Write(b)
		})
	var got int
	stderr := captureStderr(func() {
		got = applyPlan(io.Discard, ws, twoFilePlan(), "text", false)
	})
	assert.Equal(t, 2, got)
	assert.Contains(t, stderr, "b.md")
	assert.Contains(t, stderr, "no space left on device")
	assert.Contains(t, stderr, "no file was changed")
	assert.Equal(t, before, snapshotFiles(t, dir, "a.md", "b.md"))
	assertNoTempFiles(t, dir)
}

// TestApplyPlan_FileOpFailureRollsBackEdits pins the move ordering: the
// file moves only after every text edit is in place, and a failed move
// restores the rewritten files, so no link is left pointing at a file
// that never moved.
func TestApplyPlan_FileOpFailureRollsBackEdits(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	before := snapshotFiles(t, dir, "a.md", "b.md")

	// ghost.md does not exist, so the pre-flight passes but the move's
	// os.Rename fails after both edits were written.
	plan := twoFilePlan()
	plan.FileOp = &refactor.FileOp{From: "ghost.md", To: "moved.md"}
	var got int
	stderr := captureStderr(func() {
		got = applyPlan(io.Discard, ws, plan, "text", false)
	})
	assert.Equal(t, 2, got)
	assert.Contains(t, stderr, "ghost.md")
	assert.Contains(t, stderr, "restored 2 file(s)")
	assert.Equal(t, before, snapshotFiles(t, dir, "a.md", "b.md"))
	assertNoTempFiles(t, dir)
}

// TestApplyPlan_SwapFailureRollsBack pins the rollback: when renaming a
// later file's staged temp into place fails, the files already replaced
// get their original bytes back and stderr says so.
func TestApplyPlan_SwapFailureRollsBack(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	before := snapshotFiles(t, dir, "a.md", "b.md")

	failed := false
	injectWriteFileFn(t, &writeFileRenameFnMu, &writeFileRenameFn,
		func(from, to string) error {
			if filepath.Base(to) == "b.md" && !failed {
				failed = true
				return errors.New("sharing violation")
			}
			return os.Rename(from, to)
		})
	var got int
	stderr := captureStderr(func() {
		got = applyPlan(io.Discard, ws, twoFilePlan(), "text", false)
	})
	assert.Equal(t, 2, got)
	assert.Contains(t, stderr, "writing b.md")
	assert.Contains(t, stderr, "sharing violation")
	assert.Contains(t, stderr, "restored 1 file(s)")
	assert.Equal(t, before, snapshotFiles(t, dir, "a.md", "b.md"))
	assertNoTempFiles(t, dir)
}

// TestApplyPlan_RollbackFailureNamesRewrittenFiles pins the last-resort
// report: when the rollback cannot restore a file either, stderr names
// every file that keeps its rewritten content, and those are exactly
// the files left changed on disk.
func TestApplyPlan_RollbackFailureNamesRewrittenFiles(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	before := snapshotFiles(t, dir, "a.md", "b.md")

	// Rename 1 puts a.md in place, rename 2 (b.md) fails, and rename 3
	// (restoring a.md) fails too.
	calls := 0
	injectWriteFileFn(t, &writeFileRenameFnMu, &writeFileRenameFn,
		func(from, to string) error {
			calls++
			if calls > 1 {
				return errors.New("read-only file system")
			}
			return os.Rename(from, to)
		})
	var got int
	stderr := captureStderr(func() {
		got = applyPlan(io.Discard, ws, twoFilePlan(), "text", false)
	})
	assert.Equal(t, 2, got)
	assert.Contains(t, stderr, "writing b.md")
	assert.Contains(t, stderr, "restoring a.md")
	assert.Contains(t, stderr, "1 file(s) keep the rewritten content: a.md")
	after := snapshotFiles(t, dir, "a.md", "b.md")
	assert.Equal(t, "# Install\n\nBody.\n", after["a.md"], "a.md keeps the rewrite stderr names")
	assert.Equal(t, before["b.md"], after["b.md"])
	assertNoTempFiles(t, dir)
}

func TestPreflightDestination(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "taken.md"), []byte("x"), 0o644))
	assert.Equal(t, 0, preflightDestination(dir, nil), "no file move → nothing to check")
	assert.Equal(t, 0, preflightDestination(dir, &refactor.FileOp{From: "a.md", To: "free.md"}))
	var got int
	stderr := captureStderr(func() {
		got = preflightDestination(dir, &refactor.FileOp{From: "a.md", To: "taken.md"})
	})
	assert.Equal(t, 2, got)
	assert.Contains(t, stderr, "destination already exists: taken.md")
}
