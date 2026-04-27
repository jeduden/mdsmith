package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripSectionConflicts_Diff3CatalogConflict(t *testing.T) {
	// diff3-style conflict markers include a ||||||| base section
	// between <<<<<<< and =======. The merge driver must strip all
	// four marker types inside regenerable sections.
	input := "# Doc\n\n" +
		"<?catalog\nglob: \"plans/*.md\"\nsort: title\n" +
		"header: |\n  | Title |\n  |-------|\nrow: \"| [{title}]({filename}) |\"\n?>\n" +
		"<<<<<<< ours\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"| [Beta](plans/beta.md) |\n" +
		"||||||| base\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"=======\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"| [Gamma](plans/gamma.md) |\n" +
		">>>>>>> theirs\n" +
		"<?/catalog?>\n"

	result := string(stripSectionConflicts([]byte(input)))

	assert.NotContains(t, result, "<<<<<<<", "expected <<<<<<< marker stripped")
	assert.NotContains(t, result, "|||||||", "expected ||||||| base marker stripped")
	assert.NotContains(t, result, "=======", "expected ======= separator stripped")
	assert.NotContains(t, result, ">>>>>>>", "expected >>>>>>> marker stripped")
}

func TestStripSectionConflicts_Diff3OutsideSection_Preserved(t *testing.T) {
	// diff3 conflict markers outside regenerable sections must be
	// preserved so the user can resolve them manually.
	input := "# Doc\n\n" +
		"<<<<<<< ours\n" +
		"ours text\n" +
		"||||||| base\n" +
		"base text\n" +
		"=======\n" +
		"theirs text\n" +
		">>>>>>> theirs\n"

	result := string(stripSectionConflicts([]byte(input)))

	assert.Contains(t, result, "<<<<<<<", "expected <<<<<<< marker preserved outside section")
	assert.Contains(t, result, "|||||||", "expected ||||||| marker preserved outside section")
	assert.Contains(t, result, "=======", "expected ======= separator preserved outside section")
	assert.Contains(t, result, ">>>>>>>", "expected >>>>>>> marker preserved outside section")
}

// --- isConflictOpen ---

func TestIsConflictOpen_True(t *testing.T) {
	assert.True(t, isConflictOpen([]byte("<<<<<<< HEAD")))
	assert.True(t, isConflictOpen([]byte("<<<<<<<extra")))
}

func TestIsConflictOpen_False(t *testing.T) {
	assert.False(t, isConflictOpen([]byte("normal text")))
	assert.False(t, isConflictOpen([]byte("<<<<<< only six")))
}

// --- isConflictBase ---

func TestIsConflictBase_True(t *testing.T) {
	assert.True(t, isConflictBase([]byte("||||||| base")))
	assert.True(t, isConflictBase([]byte("|||||||")))
}

func TestIsConflictBase_False(t *testing.T) {
	assert.False(t, isConflictBase([]byte("normal")))
	assert.False(t, isConflictBase([]byte("<<<<<<< HEAD")))
}

// --- isConflictSeparator ---

func TestIsConflictSeparator_True(t *testing.T) {
	assert.True(t, isConflictSeparator([]byte("=======")))
	assert.True(t, isConflictSeparator([]byte("======= extra")))
}

func TestIsConflictSeparator_False(t *testing.T) {
	assert.False(t, isConflictSeparator([]byte("<<<<<<< HEAD")))
	assert.False(t, isConflictSeparator([]byte("======")))
}

// --- isConflictClose ---

func TestIsConflictClose_True(t *testing.T) {
	assert.True(t, isConflictClose([]byte(">>>>>>> theirs")))
	assert.True(t, isConflictClose([]byte(">>>>>>>")))
}

func TestIsConflictClose_False(t *testing.T) {
	assert.False(t, isConflictClose([]byte("normal")))
	assert.False(t, isConflictClose([]byte("<<<<<<< HEAD")))
}

// --- hasConflictMarkers ---

func TestHasConflictMarkers_WithOpenClose(t *testing.T) {
	content := []byte("line one\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> theirs\n")
	assert.True(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_OpenOnly(t *testing.T) {
	content := []byte("<<<<<<< HEAD\nours\n")
	assert.True(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_CloseOnly(t *testing.T) {
	content := []byte("some text\n>>>>>>> theirs\n")
	assert.True(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_None(t *testing.T) {
	content := []byte("# Clean file\n\nSome content.\n")
	assert.False(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_SetextHeading_NotConflict(t *testing.T) {
	// "=======" on its own is a setext heading underline, not a conflict
	content := []byte("Heading\n=======\n\nContent.\n")
	assert.False(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_Empty(t *testing.T) {
	assert.False(t, hasConflictMarkers(nil))
}

// --- matchesAnyStart / matchesAnyEnd ---

func TestMatchesAnyStart_Match(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.True(t, matchesAnyStart([]byte("<?catalog glob: \"**/*.md\" ?>"), names))
	assert.True(t, matchesAnyStart([]byte("<?include file: foo.md ?>"), names))
}

func TestMatchesAnyStart_NoMatch(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.False(t, matchesAnyStart([]byte("regular line"), names))
	assert.False(t, matchesAnyStart([]byte("<?/catalog?>"), names))
}

func TestMatchesAnyEnd_Match(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.True(t, matchesAnyEnd([]byte("<?/catalog?>"), names))
	assert.True(t, matchesAnyEnd([]byte("<?/include?>"), names))
}

func TestMatchesAnyEnd_NoMatch(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.False(t, matchesAnyEnd([]byte("regular line"), names))
	assert.False(t, matchesAnyEnd([]byte("<?catalog glob: \"*\" ?>"), names))
}

// --- ensureGitattributes ---

func TestEnsureGitattributes_CreatesFileWithEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	err := ensureGitattributes(path, []string{"PLAN.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
}

func TestEnsureGitattributes_AppendsMissingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(path, []byte("*.md text\n"), 0644))

	err := ensureGitattributes(path, []string{"PLAN.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "*.md text")
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
}

func TestEnsureGitattributes_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	require.NoError(t, ensureGitattributes(path, []string{"PLAN.md"}))
	require.NoError(t, ensureGitattributes(path, []string{"PLAN.md"}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// Entry should appear exactly once.
	assert.Equal(t, 1, strings.Count(string(data), "PLAN.md merge=mdsmith"))
}

func TestEnsureGitattributes_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	err := ensureGitattributes(path, []string{"PLAN.md", "README.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
	assert.Contains(t, string(data), "README.md merge=mdsmith")
}

func TestEnsureGitattributes_AddsOnlyMissingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(path, []byte("PLAN.md merge=mdsmith\n"), 0644))

	err := ensureGitattributes(path, []string{"PLAN.md", "README.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), "PLAN.md merge=mdsmith"))
	assert.Contains(t, string(data), "README.md merge=mdsmith")
}

func TestEnsureGitattributes_NoTrailingNewlineInExisting_Handled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	// File without trailing newline.
	require.NoError(t, os.WriteFile(path, []byte("*.md text"), 0644))

	err := ensureGitattributes(path, []string{"PLAN.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// Trailing newline should be added before the new entry.
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
}

// --- runMergeDriver dispatch ---

func TestRunMergeDriver_NoArgs_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriver(nil)
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriver_HelpLong_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriver([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriver_HelpShort_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriver([]string{"-h"})
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriver_UnknownSubcommand_ExitsTwo(t *testing.T) {
	got := captureStderr(func() {
		code := runMergeDriver([]string{"unknown"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "unknown subcommand")
}

func TestRunMergeDriverRun_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriverRun([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriverRun_TooFewArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriverRun([]string{"base", "ours"})
		assert.Equal(t, 2, code)
	})
}

func TestRunMergeDriverInstall_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriverInstall([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

// --- resolveInstalledBinary ---

func TestResolveInstalledBinary_NonTemporaryExe(t *testing.T) {
	// Override executableFunc to return a path that is NOT under os.TempDir()
	// so isTemporaryBinary returns false.  resolveInstalledBinary should use
	// that path directly without falling through to the PATH/GOPATH lookup.
	fakePermanent := "/usr/local/bin-test-fake/mdsmith"

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return fakePermanent, nil }

	got, err := resolveInstalledBinary()
	require.NoError(t, err)
	assert.Equal(t, fakePermanent, got)
}

func TestResolveInstalledBinary_FromPATH(t *testing.T) {
	// Place a fake "mdsmith" binary in a directory added to PATH.
	// resolveInstalledBinary should find it after the temp-binary fallback.
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "mdsmith")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755))

	// Point executableFunc at a temporary path so the exe-based path is skipped.
	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	got, err := resolveInstalledBinary()
	require.NoError(t, err)
	assert.Equal(t, fakeBin, got)
}

func TestResolveInstalledBinary_FromGopathBin(t *testing.T) {
	// When the current exe is a transient go-run binary and "mdsmith" is
	// not in PATH, resolveInstalledBinary must fall back to $GOPATH/bin.
	// Limit PATH to the directory containing "go" so goEnvPath succeeds
	// but exec.LookPath("mdsmith") fails (no other dirs to search).
	goBin, err := exec.LookPath("go")
	require.NoError(t, err)

	gopathDir := t.TempDir()
	gopathBinDir := filepath.Join(gopathDir, "bin")
	require.NoError(t, os.MkdirAll(gopathBinDir, 0o755))
	fakeBin := filepath.Join(gopathBinDir, "mdsmith")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}

	t.Setenv("PATH", filepath.Dir(goBin))
	t.Setenv("GOPATH", gopathDir)

	got, err := resolveInstalledBinary()
	require.NoError(t, err)
	assert.Equal(t, fakeBin, got)
}

func TestResolveInstalledBinary_GopathListWithEmptyEntries(t *testing.T) {
	// A multi-entry GOPATH where the second entry contains the binary
	// must be searched after the first entry comes up empty. An empty
	// component in the list (resulting from leading/trailing/double
	// separators) must be skipped instead of producing "/bin/mdsmith".
	goBin, err := exec.LookPath("go")
	require.NoError(t, err)

	emptyGopath := t.TempDir() // no bin/ subdir → first lookup fails
	realGopath := t.TempDir()
	realBinDir := filepath.Join(realGopath, "bin")
	require.NoError(t, os.MkdirAll(realBinDir, 0o755))
	fakeBin := filepath.Join(realBinDir, "mdsmith")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}

	t.Setenv("PATH", filepath.Dir(goBin))
	sep := string(os.PathListSeparator)
	t.Setenv("GOPATH", emptyGopath+sep+sep+realGopath)

	got, err := resolveInstalledBinary()
	require.NoError(t, err)
	assert.Equal(t, fakeBin, got)
}

func TestResolveInstalledBinary_NotFound(t *testing.T) {
	// When the exe is temporary, mdsmith is not in PATH, and GOPATH/bin has
	// no mdsmith, resolveInstalledBinary should return an error.
	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}

	// Empty PATH so LookPath("mdsmith") fails and go env GOPATH also fails.
	t.Setenv("PATH", "")

	_, err := resolveInstalledBinary()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith not found")
}

// --- goEnvPath ---

func TestGoEnvPath_GoNotInPATH(t *testing.T) {
	// When PATH is empty "go" cannot be found, so goEnvPath returns an error.
	t.Setenv("PATH", "")
	_, err := goEnvPath()
	require.Error(t, err)
}

// --- isTemporaryBinary ---

func TestIsTemporaryBinary_NonTempPath(t *testing.T) {
	// A path outside os.TempDir() should NOT be considered temporary.
	// Use a path that is definitely not under /tmp.
	assert.False(t, isTemporaryBinary("/usr/local/bin/mdsmith"))
}

func TestIsTemporaryBinary_TempPath(t *testing.T) {
	// A binary under a go-run* subdirectory of os.TempDir() IS transient.
	tmp := os.TempDir()
	assert.True(t, isTemporaryBinary(filepath.Join(tmp, "go-run-123", "exe", "main")))
	assert.True(t, isTemporaryBinary(filepath.Join(tmp, "go-build456", "b001", "mdsmith")))
}

func TestIsTemporaryBinary_TempPathNotGoToolchain(t *testing.T) {
	// A binary downloaded to TempDir but NOT in a go-run/go-build subdirectory
	// must NOT be treated as transient — a user may have intentionally placed
	// a release binary there.
	tmp := os.TempDir()
	assert.False(t, isTemporaryBinary(filepath.Join(tmp, "my-tools", "mdsmith")))
	assert.False(t, isTemporaryBinary(filepath.Join(tmp, "mdsmith")))
}

func TestIsTemporaryBinary_RelativePath_RelErrorReturnsFalse(t *testing.T) {
	// filepath.Rel returns an error when basepath is absolute (os.TempDir
	// is always absolute) and targpath is relative — filepath.Clean does
	// not promote a relative path to absolute. The function must treat
	// that as "not temporary" rather than panicking or returning true.
	assert.False(t, isTemporaryBinary("relative/path/mdsmith"))
}

// --- ensurePreMergeCommitHook ---

func TestEnsurePreMergeCommitHook_CreatesExecutableHook(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755))

	// Stub binary resolution so the hook content is deterministic.
	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	err := ensurePreMergeCommitHook(dir, []string{"PLAN.md", "README.md"})
	require.NoError(t, err)

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-merge-commit")
	info, err := os.Stat(hookPath)
	require.NoError(t, err, "hook must exist at %s", hookPath)
	// Hook must be executable for git to invoke it.
	assert.NotZero(t, info.Mode()&0o111, "hook must have an execute bit set")

	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, preMergeCommitHookMarker)
	assert.Contains(t, content, "'/usr/local/bin/mdsmith' fix",
		"hook must invoke the resolved mdsmith binary with fix")
	assert.Contains(t, content, "'PLAN.md'")
	assert.Contains(t, content, "'README.md'")
}

func TestEnsurePreMergeCommitHook_OverwritesManagedHook(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	// Pre-existing hook with our marker — install must replace it.
	old := "#!/bin/sh\n" + preMergeCommitHookMarker + "\n# stale content\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(old), 0o755))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	require.NoError(t, ensurePreMergeCommitHook(dir, []string{"PLAN.md"}))

	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "stale content",
		"managed hook must be replaced, not preserved")
	assert.Contains(t, string(data), "'PLAN.md'")
}

func TestEnsurePreMergeCommitHook_RefusesUnmanagedHook(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	// User-authored hook without our marker — must be left intact.
	user := "#!/bin/sh\necho user hook\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(user), 0o755))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	err := ensurePreMergeCommitHook(dir, []string{"PLAN.md"})
	require.Error(t, err, "must fail when an unmanaged hook is present")

	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, user, string(data), "unmanaged hook content must be untouched")
}

func TestEnsurePreMergeCommitHook_BinaryNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}
	t.Setenv("PATH", "")

	err := ensurePreMergeCommitHook(dir, []string{"PLAN.md"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot locate mdsmith binary")
}

// --- registerMergeDriver ---

func TestRegisterMergeDriver_BinaryNotFound_ReturnsError(t *testing.T) {
	// When resolveInstalledBinary cannot locate a binary, registerMergeDriver
	// must surface that error instead of writing a broken git config entry.
	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}
	t.Setenv("PATH", "")

	err := registerMergeDriver()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot locate mdsmith binary")
}

// --- shellQuote ---

func TestShellQuote_NoSpecialChars(t *testing.T) {
	assert.Equal(t, "'/usr/local/bin/mdsmith'", shellQuote("/usr/local/bin/mdsmith"))
}

func TestShellQuote_ContainsSingleQuote(t *testing.T) {
	// A single quote in the path must be escaped as '\''.
	assert.Equal(t, "'/path/it'\\''s/mdsmith'", shellQuote("/path/it's/mdsmith"))
}

func TestShellQuote_PathWithSpaces(t *testing.T) {
	assert.Equal(t, "'/home/my user/bin/mdsmith'", shellQuote("/home/my user/bin/mdsmith"))
}
