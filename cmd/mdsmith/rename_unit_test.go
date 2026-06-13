package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/rename"
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
	opts, pos, err := parseRenameFlags([]string{"--heading", "a.md", "Old", "New"})
	require.NoError(t, err)
	assert.True(t, opts.heading)
	assert.Equal(t, []string{"a.md", "Old", "New"}, pos)

	_, _, err = parseRenameFlags([]string{"--unknown"})
	require.Error(t, err)
}

func TestRunRename_FlagAndArgValidation(t *testing.T) {
	renameWorkspace(t)
	// --help is a pflag ErrHelp: reportFlagParseErr returns 0.
	assert.Equal(t, 0, runRename([]string{"--help"}))
	// Neither mode flag.
	assert.Equal(t, 2, runRename([]string{"a.md", "Old", "New"}))
	// Both mode flags.
	assert.Equal(t, 2, runRename([]string{"--heading", "--link-ref", "a.md", "O", "N"}))
	// Wrong positional count.
	assert.Equal(t, 2, runRename([]string{"--heading", "a.md", "Old"}))
	// Not workspace-relative.
	assert.Equal(t, 2, runRename([]string{"--heading", "/abs/a.md", "Old", "New"}))
}

func TestRunRename_HeadingSuccess(t *testing.T) {
	dir := renameWorkspace(t)
	code := runRename([]string{"--heading", "a.md", "Setup", "Install"})
	assert.Equal(t, 0, code)
	a, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	assert.Contains(t, string(a), "# Install")
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "a.md#install")
}

func TestRunRename_LinkRefSuccess(t *testing.T) {
	dir := renameWorkspace(t)
	code := runRename([]string{"--link-ref", "b.md", "docs", "rfc"})
	assert.Equal(t, 0, code)
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "[the docs][rfc]")
	assert.Contains(t, string(b), "[rfc]: https://x.example")
}

func TestRunRename_NoMatchAndConflict(t *testing.T) {
	dir := renameWorkspace(t)
	// Heading text not present → exit 1.
	assert.Equal(t, 1, runRename([]string{"--heading", "a.md", "Ghost", "X"}))
	// Link-ref label not present → exit 1.
	assert.Equal(t, 1, runRename([]string{"--link-ref", "a.md", "ghost", "x"}))
	// Heading collision → exit 2.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"),
		[]byte("# Alpha\n\n## Beta\n"), 0o644))
	assert.Equal(t, 2, runRename([]string{"--heading", "c.md", "Alpha", "Beta"}))
	// Heading no-op (same text) → empty changes → exit 1.
	assert.Equal(t, 1, runRename([]string{"--heading", "a.md", "Setup", "Setup"}))
	// Link-ref invalid rune → exit 2.
	assert.Equal(t, 2, runRename([]string{"--link-ref", "b.md", "docs", "bad]label"}))
}

func TestRunRename_JSONFormat(t *testing.T) {
	renameWorkspace(t)
	var code int
	out := captureStdout(func() {
		code = runRename([]string{"--heading", "--format", "json", "a.md", "Setup", "Install"})
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

func TestComputeRenameChanges(t *testing.T) {
	renameWorkspace(t)
	ws, src, code := buildRenameWorkspace(renameOptions{}, "a.md")
	require.Equal(t, -1, code)

	changes, c := computeRenameChanges(ws, "a.md", src, "Setup", "Install", true)
	assert.Equal(t, -1, c)
	assert.Contains(t, changes, "a.md")

	_, c = computeRenameChanges(ws, "a.md", src, "Ghost", "X", true)
	assert.Equal(t, 1, c)
}

func TestApplyAndReport_Errors(t *testing.T) {
	renameWorkspace(t)
	ws, _, code := buildRenameWorkspace(renameOptions{}, "a.md")
	require.Equal(t, -1, code)

	// A change keyed at an unreadable path → exit 2.
	got := applyAndReport(&bytes.Buffer{}, ws,
		map[string][]rename.Edit{"missing.md": {{NewText: "x"}}}, "text")
	assert.Equal(t, 2, got)

	// applyEdits fails on an out-of-range line → exit 2.
	bad := map[string][]rename.Edit{"a.md": {{
		Range:   rename.Range{Start: rename.Position{Line: 99}, End: rename.Position{Line: 99}},
		NewText: "x",
	}}}
	assert.Equal(t, 2, applyAndReport(&bytes.Buffer{}, ws, bad, "text"))
}

func TestEmitRenameSummary(t *testing.T) {
	sums := []renameSummary{{File: "a.md", Edits: 2}}

	var buf bytes.Buffer
	assert.Equal(t, 0, emitRenameSummary(&buf, sums, "text"))
	assert.Contains(t, buf.String(), "a.md: 2 edit(s)")

	buf.Reset()
	assert.Equal(t, 0, emitRenameSummary(&buf, sums, "json"))
	assert.Contains(t, buf.String(), `"edits": 2`)

	assert.Equal(t, 2, emitRenameSummary(&buf, sums, "yaml"))

	// A writer that always errors drives the json and text write-error
	// arms.
	ew := &errWriter{err: errors.New("boom")}
	assert.Equal(t, 2, emitRenameSummary(ew, sums, "json"))
	assert.Equal(t, 2, emitRenameSummary(ew, sums, "text"))
}

// mkEdit builds a single-line rename.Edit, keeping the table-style
// test cases below readable.
func mkEdit(line, startCh, endCh int, text string) rename.Edit {
	return rename.Edit{
		Range: rename.Range{
			Start: rename.Position{Line: line, Character: startCh},
			End:   rename.Position{Line: line, Character: endCh},
		},
		NewText: text,
	}
}

func TestApplyEdits(t *testing.T) {
	t.Run("single edit", func(t *testing.T) {
		out, err := applyEdits([]byte("# Setup\n"), []rename.Edit{mkEdit(0, 2, 7, "Install")})
		require.NoError(t, err)
		assert.Equal(t, "# Install\n", string(out))
	})
	t.Run("two edits same line apply right-to-left", func(t *testing.T) {
		// `[a](#x) [b](#y)` → rewrite both fragments.
		out, err := applyEdits([]byte("[a](#x) [b](#y)\n"), []rename.Edit{
			mkEdit(0, 5, 6, "X"),
			mkEdit(0, 13, 14, "Y"),
		})
		require.NoError(t, err)
		assert.Equal(t, "[a](#X) [b](#Y)\n", string(out))
	})
	t.Run("CRLF preserved", func(t *testing.T) {
		out, err := applyEdits([]byte("# Setup\r\n"), []rename.Edit{mkEdit(0, 2, 7, "X")})
		require.NoError(t, err)
		assert.Equal(t, "# X\r\n", string(out))
	})
	t.Run("multi-line edit rejected", func(t *testing.T) {
		_, err := applyEdits([]byte("a\nb\n"), []rename.Edit{{
			Range: rename.Range{Start: rename.Position{Line: 0}, End: rename.Position{Line: 1}},
		}})
		require.Error(t, err)
	})
	t.Run("line out of range", func(t *testing.T) {
		_, err := applyEdits([]byte("a\n"), []rename.Edit{mkEdit(9, 0, 0, "")})
		require.Error(t, err)
	})
	t.Run("offset out of range", func(t *testing.T) {
		// Start past End after mapping → the s>en guard fires.
		_, err := applyEdits([]byte("abcd\n"), []rename.Edit{mkEdit(0, 3, 1, "x")})
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
		"--heading", "--config", "/no/such/.mdsmith.yml", "a.md", "Setup", "Install",
	}))
}

func TestApplyAndReport_WriteErrorAndFallback(t *testing.T) {
	dir := t.TempDir()
	rel := "a.md"
	abs := filepath.Join(dir, rel)
	require.NoError(t, os.WriteFile(abs, []byte("# Setup\n"), 0o644))

	// relToAbs is empty so Resolve + applyAndReport take the
	// rootDir-join fallback; the edit applies and the file is
	// rewritten.
	ws := cliRenameWorkspace{relToAbs: map[string]string{}, rootDir: dir}
	edit := rename.Edit{
		Range: rename.Range{
			Start: rename.Position{Line: 0, Character: 2},
			End:   rename.Position{Line: 0, Character: 7},
		},
		NewText: "Install",
	}
	var buf bytes.Buffer
	require.Equal(t, 0, applyAndReport(&buf, ws,
		map[string][]rename.Edit{rel: {edit}}, "text"))
	got, _ := os.ReadFile(abs)
	assert.Contains(t, string(got), "# Install")

	// Resolve normalizes "./sub" → "sub" and reads the mapped file
	// (ok), but applyAndReport's raw-key lookup misses and falls back
	// to rootDir/sub — a directory — so writeFilePreservingMode fails
	// → exit 2. This drives the write-error arm without relying on
	// permission bits (the test runs as root).
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	ws2 := cliRenameWorkspace{relToAbs: map[string]string{"sub": abs}, rootDir: dir}
	code := applyAndReport(&bytes.Buffer{}, ws2,
		map[string][]rename.Edit{"./sub": {edit}}, "text")
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
