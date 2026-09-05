package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renameWorkspace creates a minimal project (.git + .mdsmith.yml +
// linked docs) and chdirs into it so runRename's discovery resolves
// against it, mirroring depsWorkspace.
func renameWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wf := func(rel, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	wf(".mdsmith.yml", "files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n")
	wf("a.md", "# Setup\n\nBody.\n")
	wf("b.md", "See [go](a.md#setup) and [the docs][docs].\n\n[docs]: https://x.example\n")
	t.Chdir(dir)
	return dir
}

func TestParseRenameFlags(t *testing.T) {
	opts, pos, err := parseRenameFlags([]string{"--as", "heading", "a.md", "Old", "New"})
	require.NoError(t, err)
	assert.Equal(t, "heading", opts.as)
	assert.Equal(t, []string{"a.md", "Old", "New"}, pos)

	_, _, err = parseRenameFlags([]string{"--unknown"})
	require.Error(t, err)
}

func TestRunRename_FlagAndArgValidation(t *testing.T) {
	renameWorkspace(t)
	// --help is a pflag ErrHelp: reportFlagParseErr returns 0.
	assert.Equal(t, 0, runRename([]string{"--help"}))
	// Invalid --as value.
	assert.Equal(t, 2, runRename([]string{"--as", "bogus", "a.md", "O", "N"}))
	// Wrong positional count.
	assert.Equal(t, 2, runRename([]string{"--as", "heading", "a.md", "Old"}))
	// Not workspace-relative.
	assert.Equal(t, 2, runRename([]string{"--as", "heading", "/abs/a.md", "Old", "New"}))
}

func TestRunRename_HeadingSuccess(t *testing.T) {
	dir := renameWorkspace(t)
	code := runRename([]string{"--as", "heading", "a.md", "Setup", "Install"})
	assert.Equal(t, 0, code)
	a, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	assert.Contains(t, string(a), "# Install")
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "a.md#install")
}

func TestRunRename_AutoDetects(t *testing.T) {
	dir := renameWorkspace(t)
	// No --as: "Setup" matches a heading in a.md, so a heading rename.
	assert.Equal(t, 0, runRename([]string{"a.md", "Setup", "Install"}))
	a, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	assert.Contains(t, string(a), "# Install")
	// No --as: "docs" matches a link-ref label in b.md, so a label rename.
	assert.Equal(t, 0, runRename([]string{"b.md", "docs", "rfc"}))
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "[rfc]: https://x.example")
}

func TestRunRename_MoveIntentGuard(t *testing.T) {
	renameWorkspace(t)
	// A markdown-suffixed old/new that matches no symbol steers to `move`.
	var code int
	stderr := captureStderr(func() {
		code = runRename([]string{"a.md", "old.md", "new.md"})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "mdsmith move")

	// A slash-bearing new (path-shaped) with a plain old also steers to
	// move — firstPathish picks the path-shaped argument for the hint.
	stderr = captureStderr(func() {
		code = runRename([]string{"a.md", "PlainOld", "sub/new.md"})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "sub/new.md")
}

func TestRunRename_NeitherMatchNonPath(t *testing.T) {
	renameWorkspace(t)
	// Neither a heading nor a label, and not path-shaped → exit 2 naming
	// the missing symbol and pointing at move.
	var code int
	stderr := captureStderr(func() {
		code = runRename([]string{"a.md", "GhostWord", "OtherWord"})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "no heading or link-ref label")
}

func TestRunRename_AmbiguousNeedsAs(t *testing.T) {
	dir := renameWorkspace(t)
	// A file where the same text is both a heading and a link-ref label.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "d.md"),
		[]byte("# Spec\n\nSee [Spec].\n\n[Spec]: https://x.example\n"), 0o644))
	assert.Equal(t, 2, runRename([]string{"d.md", "Spec", "Rfc"}))
	// Forcing --as resolves it.
	assert.Equal(t, 0, runRename([]string{"d.md", "--as", "heading", "Spec", "Rfc"}))
}

func TestRunRename_LinkRefSuccess(t *testing.T) {
	dir := renameWorkspace(t)
	code := runRename([]string{"--as", "label", "b.md", "docs", "rfc"})
	assert.Equal(t, 0, code)
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "[the docs][rfc]")
	assert.Contains(t, string(b), "[rfc]: https://x.example")
}

func TestRunRename_DryRunChangesNothing(t *testing.T) {
	dir := renameWorkspace(t)
	before, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	assert.Equal(t, 0, runRename([]string{"--as", "heading", "--dry-run", "a.md", "Setup", "Install"}))
	after, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	assert.Equal(t, string(before), string(after))
}

func TestRunRename_NoMatchAndConflict(t *testing.T) {
	dir := renameWorkspace(t)
	// Heading text not present → exit 1.
	assert.Equal(t, 1, runRename([]string{"--as", "heading", "a.md", "Ghost", "X"}))
	// Link-ref label not present → exit 1.
	assert.Equal(t, 1, runRename([]string{"--as", "label", "a.md", "ghost", "x"}))
	// Heading collision → exit 2.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"),
		[]byte("# Alpha\n\n## Beta\n"), 0o644))
	assert.Equal(t, 2, runRename([]string{"--as", "heading", "c.md", "Alpha", "Beta"}))
	// Heading no-op (same text) → empty changes → exit 1.
	assert.Equal(t, 1, runRename([]string{"--as", "heading", "a.md", "Setup", "Setup"}))
	// Link-ref invalid rune → exit 2.
	assert.Equal(t, 2, runRename([]string{"--as", "label", "b.md", "docs", "bad]label"}))
}

func TestRunRename_JSONFormat(t *testing.T) {
	renameWorkspace(t)
	var code int
	out := captureStdout(func() {
		code = runRename([]string{"--as", "heading", "--format", "json", "a.md", "Setup", "Install"})
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, out, `"file": "a.md"`)
	assert.Contains(t, out, `"edits": 1`)
}

func TestBuildRenameWorkspace_DiscoveryPaths(t *testing.T) {
	t.Run("missing config exits 2", func(t *testing.T) {
		renameWorkspace(t)
		opts := renameOptions{configPath: "/no/such/.mdsmith.yml"}
		_, _, code := buildRenameWorkspace(opts, "a.md")
		assert.Equal(t, 2, code)
	})
	t.Run("empty workspace exits 1", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
			[]byte("files:\n  - \"nope/*.md\"\n"), 0o644))
		t.Chdir(dir)
		_, _, code := buildRenameWorkspace(renameOptions{}, "a.md")
		assert.Equal(t, 1, code)
	})
	t.Run("bad max-input-size exits 2", func(t *testing.T) {
		renameWorkspace(t)
		_, _, code := buildRenameWorkspace(renameOptions{maxInputSize: "notabytes"}, "a.md")
		assert.Equal(t, 2, code)
	})
	t.Run("unreadable target exits 2", func(t *testing.T) {
		renameWorkspace(t)
		_, _, code := buildRenameWorkspace(renameOptions{}, "missing.md")
		assert.Equal(t, 2, code)
	})
}

func TestComputeRenamePlan(t *testing.T) {
	renameWorkspace(t)
	ws, src, code := buildRenameWorkspace(renameOptions{}, "a.md")
	require.Equal(t, -1, code)

	plan, c := computeRenamePlan(ws, "a.md", src, "Setup", "Install", "heading")
	assert.Equal(t, -1, c)
	assert.Contains(t, plan.Edits, "a.md")

	_, c = computeRenamePlan(ws, "a.md", src, "Ghost", "X", "heading")
	assert.Equal(t, 1, c)
}

func TestApplyPlan_Errors(t *testing.T) {
	renameWorkspace(t)
	ws, _, code := buildRenameWorkspace(renameOptions{}, "a.md")
	require.Equal(t, -1, code)

	// An edit keyed at an unreadable path → exit 2.
	got := applyPlan(&bytes.Buffer{}, ws,
		refactor.Plan{Edits: map[string][]refactor.Edit{"missing.md": {{NewText: "x"}}}}, "text", false)
	assert.Equal(t, 2, got)

	// applyEdits fails on an out-of-range line → exit 2.
	bad := refactor.Plan{Edits: map[string][]refactor.Edit{"a.md": {{
		Range:   refactor.Range{Start: refactor.Position{Line: 99}, End: refactor.Position{Line: 99}},
		NewText: "x",
	}}}}
	assert.Equal(t, 2, applyPlan(&bytes.Buffer{}, ws, bad, "text", false))
}

func TestEmitPlanReport(t *testing.T) {
	sums := []renameSummary{{File: "a.md", Edits: 2}}
	op := &refactor.FileOp{From: "a.md", To: "docs/a.md"}

	var buf bytes.Buffer
	assert.Equal(t, 0, emitPlanReport(&buf, sums, op, "text", false))
	assert.Contains(t, buf.String(), "a.md: 2 edit(s)")
	assert.Contains(t, buf.String(), "moved a.md -> docs/a.md")

	buf.Reset()
	assert.Equal(t, 0, emitPlanReport(&buf, sums, op, "text", true))
	assert.Contains(t, buf.String(), "would move a.md -> docs/a.md")

	buf.Reset()
	assert.Equal(t, 0, emitPlanReport(&buf, sums, op, "json", false))
	assert.Contains(t, buf.String(), `"edits": 2`)
	assert.Contains(t, buf.String(), `"to": "docs/a.md"`)

	assert.Equal(t, 2, emitPlanReport(&buf, sums, nil, "yaml", false))

	// A writer that always errors drives the json and text write-error
	// arms.
	ew := &errWriter{err: errors.New("boom")}
	assert.Equal(t, 2, emitPlanReport(ew, sums, op, "json", false))
	assert.Equal(t, 2, emitPlanReport(ew, sums, op, "text", false))
	// With no file summaries the first failing write is the move line,
	// covering that write-error arm.
	assert.Equal(t, 2, emitPlanReport(ew, nil, op, "text", false))
}

// mkEdit builds a single-line refactor.Edit, keeping the table-style
// test cases below readable.
func mkEdit(line, startCh, endCh int, text string) refactor.Edit {
	return refactor.Edit{
		Range: refactor.Range{
			Start: refactor.Position{Line: line, Character: startCh},
			End:   refactor.Position{Line: line, Character: endCh},
		},
		NewText: text,
	}
}

func TestApplyEdits(t *testing.T) {
	t.Run("single edit", func(t *testing.T) {
		out, err := applyEdits([]byte("# Setup\n"), []refactor.Edit{mkEdit(0, 2, 7, "Install")})
		require.NoError(t, err)
		assert.Equal(t, "# Install\n", string(out))
	})
	t.Run("two edits same line apply right-to-left", func(t *testing.T) {
		// `[a](#x) [b](#y)` → rewrite both fragments.
		out, err := applyEdits([]byte("[a](#x) [b](#y)\n"), []refactor.Edit{
			mkEdit(0, 5, 6, "X"),
			mkEdit(0, 13, 14, "Y"),
		})
		require.NoError(t, err)
		assert.Equal(t, "[a](#X) [b](#Y)\n", string(out))
	})
	t.Run("CRLF preserved", func(t *testing.T) {
		out, err := applyEdits([]byte("# Setup\r\n"), []refactor.Edit{mkEdit(0, 2, 7, "X")})
		require.NoError(t, err)
		assert.Equal(t, "# X\r\n", string(out))
	})
	t.Run("multi-line edit rejected", func(t *testing.T) {
		_, err := applyEdits([]byte("a\nb\n"), []refactor.Edit{{
			Range: refactor.Range{Start: refactor.Position{Line: 0}, End: refactor.Position{Line: 1}},
		}})
		require.Error(t, err)
	})
	t.Run("line out of range", func(t *testing.T) {
		_, err := applyEdits([]byte("a\n"), []refactor.Edit{mkEdit(9, 0, 0, "")})
		require.Error(t, err)
	})
	t.Run("offset out of range", func(t *testing.T) {
		// Start past End after mapping → the s>en guard fires.
		_, err := applyEdits([]byte("abcd\n"), []refactor.Edit{mkEdit(0, 3, 1, "x")})
		require.Error(t, err)
	})
}

func TestSplitKeepCRAndJoinLF(t *testing.T) {
	src := []byte("a\r\nb\nc")
	segs := splitKeepCR(src)
	assert.Equal(t, [][]byte{[]byte("a\r"), []byte("b"), []byte("c")}, segs)
	assert.Equal(t, src, joinLF(segs))
	// Trailing newline yields a trailing empty segment that round-trips.
	assert.Equal(t, []byte("x\n"), joinLF(splitKeepCR([]byte("x\n"))))
}

func TestRunRename_FlagParseError(t *testing.T) {
	renameWorkspace(t)
	// An unknown flag is a non-help parse error → exit 2.
	assert.Equal(t, 2, runRename([]string{"--bogus", "a.md", "O", "N"}))
}

func TestRunRename_WorkspaceBuildFailure(t *testing.T) {
	renameWorkspace(t)
	// A missing config makes buildRenameWorkspace return 2, which
	// runRename propagates.
	assert.Equal(t, 2, runRename([]string{
		"--as", "heading", "--config", "/no/such/.mdsmith.yml", "a.md", "Setup", "Install",
	}))
}

func TestApplyPlan_WriteErrorAndFallback(t *testing.T) {
	dir := t.TempDir()
	rel := "a.md"
	abs := filepath.Join(dir, rel)
	require.NoError(t, os.WriteFile(abs, []byte("# Setup\n"), 0o644))

	// relToAbs is empty so Resolve + applyPlan take the rootDir-join
	// fallback; the edit applies and the file is rewritten.
	ws := cliRenameWorkspace{relToAbs: map[string]string{}, rootDir: dir}
	edit := refactor.Edit{
		Range: refactor.Range{
			Start: refactor.Position{Line: 0, Character: 2},
			End:   refactor.Position{Line: 0, Character: 7},
		},
		NewText: "Install",
	}
	var buf bytes.Buffer
	require.Equal(t, 0, applyPlan(&buf, ws,
		refactor.Plan{Edits: map[string][]refactor.Edit{rel: {edit}}}, "text", false))
	got, _ := os.ReadFile(abs)
	assert.Contains(t, string(got), "# Install")

	// Resolve normalizes "./sub" → "sub" and reads the mapped file
	// (ok), but applyEditsToFile's raw-key lookup misses and falls back
	// to rootDir/sub — a directory — so writeFilePreservingMode fails
	// → exit 2. This drives the write-error arm without relying on
	// permission bits (the test runs as root).
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	ws2 := cliRenameWorkspace{relToAbs: map[string]string{"sub": abs}, rootDir: dir}
	code := applyPlan(&bytes.Buffer{}, ws2,
		refactor.Plan{Edits: map[string][]refactor.Edit{"./sub": {edit}}}, "text", false)
	assert.Equal(t, 2, code)
}

func TestWriteFilePreservingMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	require.NoError(t, os.WriteFile(p, []byte("old"), 0o640))
	require.NoError(t, writeFilePreservingMode(p, []byte("new")))
	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())

	// Non-existent file: stat fails, default mode, write succeeds.
	np := filepath.Join(dir, "new.md")
	require.NoError(t, writeFilePreservingMode(np, []byte("x")))

	// Writing to a directory path fails.
	require.Error(t, writeFilePreservingMode(dir, []byte("x")))
}

// TestWriteFilePreservingMode_SymlinkDoesNotWriteThrough is the RED test for
// S007: writeFilePreservingMode must not follow a symlink to an external file.
// After the fix (atomic rename), os.Rename replaces the symlink itself on
// POSIX rather than following it to the external target.
func TestWriteFilePreservingMode_SymlinkDoesNotWriteThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks behave differently on Windows")
	}
	// Create the external file outside the workspace.
	external := t.TempDir()
	extFile := filepath.Join(external, "target.md")
	const originalContent = "# Original\n"
	require.NoError(t, os.WriteFile(extFile, []byte(originalContent), 0o644))

	// Create a workspace directory with a symlink pointing to the external file.
	workspace := t.TempDir()
	symlink := filepath.Join(workspace, "linked.md")
	require.NoError(t, os.Symlink(extFile, symlink))

	// Call writeFilePreservingMode on the symlink path.
	require.NoError(t, writeFilePreservingMode(symlink, []byte("# Rewritten\n")))

	// The external file must NOT have been modified.
	got, err := os.ReadFile(extFile)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(got),
		"writeFilePreservingMode must not write through symlinks to external files")

	// The symlink itself must have been replaced with a regular file by
	// os.Rename (not merely redirected to a different symlink target).
	linfo, err := os.Lstat(symlink)
	require.NoError(t, err)
	assert.Zero(t, linfo.Mode()&os.ModeSymlink, "symlink must be replaced by a regular file")

	// The symlink path itself should now read the new content.
	gotLinked, err := os.ReadFile(symlink)
	require.NoError(t, err)
	assert.Equal(t, "# Rewritten\n", string(gotLinked))
}

// TestWriteFilePreservingMode_DanglingSymlink verifies that writing through a
// dangling symlink (target does not exist) falls back to 0o644 permissions and
// replaces the symlink with a new regular file.
func TestWriteFilePreservingMode_DanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks behave differently on Windows")
	}
	workspace := t.TempDir()
	symlink := filepath.Join(workspace, "dangling.md")
	// Point symlink at a path that does not exist — os.Stat will fail.
	require.NoError(t, os.Symlink(filepath.Join(workspace, "nonexistent.md"), symlink))

	require.NoError(t, writeFilePreservingMode(symlink, []byte("# New\n")))

	// Symlink should have been replaced with a regular file containing the data.
	info, err := os.Lstat(symlink)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0), info.Mode()&os.ModeSymlink, "symlink should be replaced by regular file")
	got, err := os.ReadFile(symlink)
	require.NoError(t, err)
	assert.Equal(t, "# New\n", string(got))
}

// injectWriteFileFn swaps fn into the given var+mutex pair for the duration of
// the test and restores the original on cleanup. This follows the chmodFile
// injection pattern from internal/fix/fix.go.
func injectWriteFileFn[T any](t *testing.T, mu *sync.Mutex, slot *T, fn T) {
	t.Helper()
	mu.Lock()
	orig := *slot
	*slot = fn
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		*slot = orig
		mu.Unlock()
	})
}

func TestWriteFilePreservingMode_CreateTempError(t *testing.T) {
	dir := t.TempDir()
	injectWriteFileFn(t, &writeFileTempFnMu, &writeFileTempFn,
		func(string, string) (*os.File, error) { return nil, os.ErrPermission })
	err := writeFilePreservingMode(filepath.Join(dir, "f.md"), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating temp file")
}

func TestWriteFilePreservingMode_ChmodError(t *testing.T) {
	dir := t.TempDir()
	injectWriteFileFn(t, &writeFileChmodFnMu, &writeFileChmodFn,
		func(string, os.FileMode) error { return os.ErrPermission })
	err := writeFilePreservingMode(filepath.Join(dir, "f.md"), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "setting temp file mode")
}

func TestWriteFilePreservingMode_WriteError(t *testing.T) {
	dir := t.TempDir()
	injectWriteFileFn(t, &writeFileWriteFnMu, &writeFileWriteFn,
		func(*os.File, []byte) (int, error) { return 0, os.ErrPermission })
	err := writeFilePreservingMode(filepath.Join(dir, "f.md"), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing temp file")
}

func TestWriteFilePreservingMode_SyncError(t *testing.T) {
	dir := t.TempDir()
	injectWriteFileFn(t, &writeFileSyncFnMu, &writeFileSyncFn,
		func(*os.File) error { return os.ErrPermission })
	err := writeFilePreservingMode(filepath.Join(dir, "f.md"), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "syncing temp file")
}

func TestWriteFilePreservingMode_CloseError(t *testing.T) {
	dir := t.TempDir()
	injectWriteFileFn(t, &writeFileCloseFnMu, &writeFileCloseFn,
		func(f *os.File) error { _ = f.Close(); return os.ErrPermission })
	err := writeFilePreservingMode(filepath.Join(dir, "f.md"), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closing temp file")
}

func TestCliRenameWorkspace_Resolve(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(abs, []byte("# A\n"), 0o644))
	ws := cliRenameWorkspace{
		relToAbs: map[string]string{"a.md": abs},
		rootDir:  dir,
		maxBytes: 0,
	}
	key, src, ok := ws.Resolve("a.md")
	require.True(t, ok)
	assert.Equal(t, "a.md", key)
	assert.Equal(t, "# A\n", string(src))

	// Path not in relToAbs falls back to rootDir join.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n"), 0o644))
	_, _, ok = ws.Resolve("b.md")
	assert.True(t, ok)

	// Unreadable file → ok=false.
	_, _, ok = ws.Resolve("missing.md")
	assert.False(t, ok)
}
