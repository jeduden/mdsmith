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
	// Invalid --as value: the message lists every valid kind.
	var code int
	stderr := captureStderr(func() {
		code = runRename([]string{"--as", "bogus", "a.md", "O", "N"})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, `mdsmith: --as must be heading or label, got "bogus"`)
	// Wrong positional count.
	assert.Equal(t, 2, runRename([]string{"--as", "heading", "a.md", "Old"}))
	// Not workspace-relative.
	assert.Equal(t, 2, runRename([]string{"--as", "heading", "/abs/a.md", "Old", "New"}))
	assert.Equal(t, 2, runRename([]string{"--as", "heading", `sub\..\..\a.md`, "Old", "New"}))
}

// TestRunRename_BackslashEscape_LeavesOutsideFileUntouched pins that a
// target climbing out of the workspace with `\` separators is rejected
// before any read or write. Resolve reads `\` as a separator, so on a
// POSIX host a validator that did not would let `sub\..\..\a.md` edit
// the a.md that sits next to the workspace root.
func TestRunRename_BackslashEscape_LeavesOutsideFileUntouched(t *testing.T) {
	dir := renameWorkspace(t)
	outside := filepath.Join(filepath.Dir(dir), "a.md")
	const body = "# Setup\n\nOutside the workspace.\n"
	require.NoError(t, os.WriteFile(outside, []byte(body), 0o644))

	code := runRename([]string{"--as", "heading", `sub\..\..\a.md`, "Setup", "Hacked"})
	assert.Equal(t, 2, code)
	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
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
		assert.Equal(t, 1, code, "buildWorkspace's exit 1 propagates")
	})
	t.Run("unreadable target exits 2", func(t *testing.T) {
		renameWorkspace(t)
		_, _, code := buildRenameWorkspace(renameOptions{}, "missing.md")
		assert.Equal(t, 2, code)
	})
}

func TestBuildWorkspace(t *testing.T) {
	t.Run("missing config exits 2", func(t *testing.T) {
		renameWorkspace(t)
		_, code := buildWorkspace(renameOptions{configPath: "/no/such/.mdsmith.yml"})
		assert.Equal(t, 2, code)
	})
	t.Run("empty workspace exits 1", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
			[]byte("files:\n  - \"nope/*.md\"\n"), 0o644))
		t.Chdir(dir)
		_, code := buildWorkspace(renameOptions{})
		assert.Equal(t, 1, code)
	})
	t.Run("bad max-input-size exits 2", func(t *testing.T) {
		renameWorkspace(t)
		_, code := buildWorkspace(renameOptions{maxInputSize: "notabytes"})
		assert.Equal(t, 2, code)
	})
	t.Run("success indexes every workspace file", func(t *testing.T) {
		dir := renameWorkspace(t)
		ws, code := buildWorkspace(renameOptions{})
		require.Equal(t, -1, code)
		assert.ElementsMatch(t, []string{"a.md", "b.md"}, ws.Files())
		assert.Contains(t, ws.relToAbs, "a.md")
		assert.Equal(t, dir, ws.rootDir)
		assert.Positive(t, ws.maxBytes)
	})
}

func TestLooksLikePath(t *testing.T) {
	assert.True(t, looksLikePath("docs/a.md"))
	assert.True(t, looksLikePath("a.md"))
	assert.True(t, looksLikePath("a.markdown"))
	assert.True(t, looksLikePath("dir/name"))
	assert.False(t, looksLikePath("Setup"))
	assert.False(t, looksLikePath("a.txt"))
	assert.False(t, looksLikePath(""))
}

func TestFirstPathish(t *testing.T) {
	assert.Equal(t, "a.md", firstPathish("a.md", "b.md"), "a wins when both look like paths")
	assert.Equal(t, "b.md", firstPathish("Setup", "b.md"))
	assert.Equal(t, "Other", firstPathish("Setup", "Other"), "falls back to b")
}

func TestResolveWriteMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits and symlinks are not portable to Windows")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f.md")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	assert.Equal(t, os.FileMode(0o600), resolveWriteMode(f))

	// A missing path falls back to 0o644.
	assert.Equal(t, os.FileMode(0o644), resolveWriteMode(filepath.Join(dir, "none.md")))

	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(f, link))
	assert.Equal(t, os.FileMode(0o600), resolveWriteMode(link), "symlink follows to its target")

	dangling := filepath.Join(dir, "dangling.md")
	require.NoError(t, os.Symlink(filepath.Join(dir, "gone.md"), dangling))
	assert.Equal(t, os.FileMode(0o644), resolveWriteMode(dangling))
}

// TestComputeRenamePlan pins every exit path computeRenamePlan maps
// from refactor.Rename: -1 with a plan on success, 1 when an explicit
// kind finds nothing (or a rename changes no byte), 2 on an ambiguous
// or absent auto-detect, a path-shaped request, or an engine conflict.
func TestComputeRenamePlan(t *testing.T) {
	renameWorkspace(t)
	ws, src, code := buildRenameWorkspace(renameOptions{}, "a.md")
	require.Equal(t, -1, code)

	plan, c := computeRenamePlan(ws, "a.md", src, "Setup", "Install", refactor.KindHeading)
	assert.Equal(t, -1, c)
	assert.Contains(t, plan.Edits, "a.md")
	// The anchor link in b.md is rewritten too.
	assert.Contains(t, plan.Edits, "b.md")

	plan, c = computeRenamePlan(ws, "a.md", src, "Setup", "Install", "")
	assert.Equal(t, -1, c, "auto-detects the heading")
	assert.Contains(t, plan.Edits, "b.md")

	labelSrc := []byte("# Setup\n\nSee [docs].\n\n[docs]: u\n")
	plan, c = computeRenamePlan(ws, "a.md", labelSrc, "docs", "manual", "")
	assert.Equal(t, -1, c, "auto-detects the label")
	assert.Len(t, plan.Edits["a.md"], 2)

	cases := []struct {
		name     string
		src      []byte
		old, neu string
		kind     refactor.RenameKind
		code     int
		stderr   string
	}{
		{"no-op heading", src, "Setup", "Setup", "", 1, `nothing to rename for heading "Setup"`},
		{"no-op label", labelSrc, "docs", "docs", "", 1, `nothing to rename for label "docs"`},
		{"same-bytes heading", []byte("# *Setup*\n"), "Setup", "*Setup*", "", 1,
			`nothing to rename for heading "Setup"`},
		{"missing label, colliding new name", labelSrc, "ghost", "docs", refactor.KindLabel, 1,
			`no link reference "ghost" in a.md`},
		{"missing heading", src, "Ghost", "X", refactor.KindHeading, 1, `no heading "Ghost" in a.md`},
		{"missing label", labelSrc, "ghost", "x", refactor.KindLabel, 1, `no link reference "ghost" in a.md`},
		{"ambiguous", []byte("# docs\n\nSee [docs].\n\n[docs]: u\n"), "docs", "x", "", 2,
			"matches both a heading and a link-ref label in a.md; pass --as heading or --as label"},
		{"neither", labelSrc, "ghost", "x", "", 2, `no heading or link-ref label "ghost" in a.md`},
		{"path-shaped", labelSrc, "old.md", "new.md", "", 2, "mdsmith move old.md new.md"},
		{"heading collision", []byte("# Setup\n\n# Other\n"), "Setup", "Other", refactor.KindHeading, 2, "collide"},
		{"invalid label rune", labelSrc, "docs", "bad]name", refactor.KindLabel, 2, "label cannot contain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got int
			stderr := captureStderr(func() {
				_, got = computeRenamePlan(ws, "a.md", tc.src, tc.old, tc.neu, tc.kind)
			})
			assert.Equal(t, tc.code, got)
			assert.Contains(t, stderr, tc.stderr)
		})
	}
}

// TestRenameExitCode maps each refactor.Rename outcome straight to its
// exit code and message, without a workspace.
func TestRenameExitCode(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		old, neu string
		code     int
		stderr   string
	}{
		{"ambiguous", refactor.ErrAmbiguousRename, "docs", "x", 2,
			`"docs" matches both a heading and a link-ref label in a.md; pass --as heading or --as label`},
		{"neither", refactor.ErrNoRenameTarget, "ghost", "x", 2,
			`no heading or link-ref label "ghost" in a.md (to relocate a file, use mdsmith move)`},
		{"neither, new name path-shaped", refactor.ErrNoRenameTarget, "ghost", "docs/new.md", 2,
			`"docs/new.md" looks like a file path; to relocate a file use: mdsmith move ghost docs/new.md`},
		{"nothing to rename", refactor.NothingToRenameError{Kind: refactor.KindHeading, Name: "Setup"},
			"Setup", "Setup", 1, `nothing to rename for heading "Setup"`},
		{"nothing to rename label", refactor.NothingToRenameError{Kind: refactor.KindLabel, Name: "docs"},
			"docs", "docs", 1, `nothing to rename for label "docs"`},
		{"missing symbol", refactor.MissingSymbolError{Kind: refactor.KindLabel, Name: "ghost"}, "ghost", "x", 1,
			`no link reference "ghost" in a.md`},
		{"engine error", refactor.ErrEmptyLabel, "docs", "", 2, "mdsmith: label cannot be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got int
			stderr := captureStderr(func() {
				got = renameExitCode(tc.err, "a.md", tc.old, tc.neu)
			})
			assert.Equal(t, tc.code, got)
			assert.Contains(t, stderr, tc.stderr)
		})
	}
}

func TestApplyPlan_Errors(t *testing.T) {
	renameWorkspace(t)
	ws, _, code := buildRenameWorkspace(renameOptions{}, "a.md")
	require.Equal(t, -1, code)

	// An edit keyed at an unreadable path → exit 2.
	got := applyPlan(&bytes.Buffer{}, ws,
		refactor.Plan{Edits: map[string][]refactor.Edit{"missing.md": {{NewText: "x"}}}}, "text", false)
	assert.Equal(t, 2, got)

	// refactor.ApplyEdits fails on an out-of-range line → exit 2.
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
	// (ok), but computePlanWrite's raw-key lookup misses and falls back
	// to rootDir/sub — a directory — so renaming the staged temp over
	// it fails → exit 2. This drives the write-error arm without
	// relying on permission bits (the test runs as root).
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

// TestCliRenameWorkspace_Stat locks that Stat finds the file Resolve
// would read without reading it: a file over the size limit is
// present, a directory and a missing path are not.
func TestCliRenameWorkspace_Stat(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(abs, []byte("# A, too long\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hero.png"), []byte("png"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	ws := cliRenameWorkspace{relToAbs: map[string]string{"a.md": abs}, rootDir: dir, maxBytes: 4}

	_, _, readable := ws.Resolve("a.md")
	require.False(t, readable, "over the size limit")
	info, ok := ws.Stat("a.md")
	require.True(t, ok, "present though unreadable")
	assert.Equal(t, "a.md", info.Name())
	_, ok = ws.Stat("./hero.png")
	assert.True(t, ok, "a path not in relToAbs falls back to rootDir join")
	_, ok = ws.Stat("sub")
	assert.False(t, ok, "a directory is no file")
	_, ok = ws.Stat("missing.md")
	assert.False(t, ok)
}

func TestStageFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	require.NoError(t, os.WriteFile(p, []byte("old"), 0o640))
	tmp, err := stageFile(p, []byte("new"))
	require.NoError(t, err)
	assert.Equal(t, dir, filepath.Dir(tmp), "the temp is a sibling of path")
	got, err := os.ReadFile(tmp)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	info, err := os.Stat(tmp)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "the temp carries path's mode")
	}
	orig, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "old", string(orig), "staging leaves path alone")

	// A failed fill removes the temp it created.
	injectWriteFileFn(t, &writeFileSyncFnMu, &writeFileSyncFn,
		func(*os.File) error { return os.ErrPermission })
	_, err = stageFile(p, []byte("new2"))
	require.Error(t, err)
	left, err := filepath.Glob(filepath.Join(dir, "f.md.*.tmp"))
	require.NoError(t, err)
	assert.Equal(t, []string{tmp}, left, "only the first, successful stage remains")
}

func TestFillTemp(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "f.*.tmp")
	require.NoError(t, err)
	require.NoError(t, fillTemp(tmp, 0o600, []byte("data")))
	got, err := os.ReadFile(tmp.Name())
	require.NoError(t, err)
	assert.Equal(t, "data", string(got))
	// fillTemp closed the file.
	require.Error(t, tmp.Close())
}

func TestReplaceWithStaged(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	require.NoError(t, os.WriteFile(p, []byte("old"), 0o644))
	tmp, err := stageFile(p, []byte("new"))
	require.NoError(t, err)
	require.NoError(t, replaceWithStaged(tmp, p))
	got, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	assert.NoFileExists(t, tmp)

	// A failed rename wraps the error and removes the temp.
	tmp, err = stageFile(p, []byte("newer"))
	require.NoError(t, err)
	injectWriteFileFn(t, &writeFileRenameFnMu, &writeFileRenameFn,
		func(string, string) error { return os.ErrPermission })
	err = replaceWithStaged(tmp, p)
	require.ErrorIs(t, err, os.ErrPermission)
	assert.Contains(t, err.Error(), "committing f.md")
	assert.NoFileExists(t, tmp)
	got, err = os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

// TestBuildWorkspace_IndexesOnFirstUse pins that buildWorkspace only
// discovers files: the index is built when an engine call first needs
// it, so a label rename (which never asks) reads no other file. A
// link written after buildWorkspace returns shows up in the index only
// because the index is built after it.
func TestBuildWorkspace_IndexesOnFirstUse(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"),
		[]byte("See [go](a.md#setup).\n\nAnd [again](a.md#setup).\n"), 0o644))
	assert.Len(t, ws.IncomingAnchorEdges("a.md", "setup"), 2, "indexed after the edit")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("no links\n"), 0o644))
	assert.Len(t, ws.IncomingAnchorEdges("a.md", "setup"), 2, "built once, then reused")
	assert.ElementsMatch(t, []string{"a.md", "b.md"}, ws.Files())
}

// TestCliRenameWorkspace_EdgePassThroughs pins that the path and
// wikilink edge queries answer from the same lazily built index as the
// anchor query.
func TestCliRenameWorkspace_EdgePassThroughs(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"),
		[]byte("See [go](a.md) and [[a]].\n"), 0o644))
	assert.Len(t, ws.IncomingPathEdges("a.md"), 1)
	assert.Len(t, ws.IncomingWikilinkEdges("a"), 1)
}

// A cliRenameWorkspace built without an index (as several unit tests
// construct it) answers edge queries with nothing instead of panicking,
// as the nil *index.Index it replaced did.
func TestCliRenameWorkspace_ZeroValue(t *testing.T) {
	var ws cliRenameWorkspace
	assert.Empty(t, ws.IncomingAnchorEdges("a.md", "x"))
	assert.Empty(t, ws.IncomingPathEdges("a.md"))
	assert.Empty(t, ws.IncomingWikilinkEdges("a"))
	assert.Empty(t, ws.Files())
}
