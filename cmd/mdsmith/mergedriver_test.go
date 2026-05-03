package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/githooks"
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

func TestRunMergeDriverInstall_NotInRepo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"),
		[]byte("not a real gitdir"), 0o644))
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriverInstall(nil))
	})
	assert.Contains(t, got, "not in a git repository")
}

func TestRunMergeDriverInstall_LoadConfigError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("not: [valid: yaml\n"), 0o644))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriverInstall(nil))
	})
	assert.Contains(t, got, "loading config")
}

func TestRunMergeDriverInstall_RejectsWhitespacePath(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriverInstall([]string{"bad name.md"}))
	})
	assert.Contains(t, got, "whitespace")
}

func TestRunMergeDriverInstall_NoArgsWritesCanonicalGlobs(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	// .mdsmith.yml ignore patterns become -merge overrides.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("ignore:\n  - \"vendor/**\"\n"), 0o644))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 0, runMergeDriverInstall(nil))
	})
	assert.Contains(t, got, "merge driver 'mdsmith' installed")
	assert.Contains(t, got, "git-hook-sync: true")

	attrs, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	content := string(attrs)
	assert.Contains(t, content, "*.md merge=mdsmith")
	assert.Contains(t, content, "*.markdown merge=mdsmith")
	assert.Contains(t, content, "vendor/** -merge",
		"ignore patterns from .mdsmith.yml must appear as -merge overrides")
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

	err := ensurePreMergeCommitHook(dir)
	require.NoError(t, err)

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-merge-commit")
	info, err := os.Stat(hookPath)
	require.NoError(t, err, "hook must exist at %s", hookPath)
	// Hook must be executable for git to invoke it (POSIX only).
	if runtime.GOOS != "windows" {
		assert.NotZero(t, info.Mode()&0o111, "hook must have an execute bit set")
	}

	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, githooks.BuildHookScript("/usr/local/bin/mdsmith"), string(data),
		"installed hook must match the canonical template")
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

	require.NoError(t, ensurePreMergeCommitHook(dir))

	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "stale content",
		"managed hook must be replaced, not preserved")
	assert.Equal(t, githooks.BuildHookScript("/usr/local/bin/mdsmith"), string(data),
		"replaced hook must match the canonical template")
}

func TestEnsurePreMergeCommitHook_SetsExecutableBitOnExistingHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics not applicable on Windows")
	}
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	// Pre-existing hook with our marker but NO execute permissions.
	old := "#!/bin/sh\n" + preMergeCommitHookMarker + "\n# old\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(old), 0o644))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	require.NoError(t, ensurePreMergeCommitHook(dir))

	info, err := os.Stat(hookPath)
	require.NoError(t, err)
	// Verify execute bit is set despite the file existing without it.
	assert.NotZero(t, info.Mode()&0o111,
		"hook must have execute bit set even when overwriting non-executable file")
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

	err := ensurePreMergeCommitHook(dir)
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

	err := ensurePreMergeCommitHook(dir)
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

func TestEnsurePreMergeCommitHook_UnreadableHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics not applicable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission checks don't apply")
	}
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	// Write-only: os.ReadFile returns a non-ENOENT error.
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o200))
	t.Cleanup(func() { _ = os.Chmod(hookPath, 0o755) })

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	err := ensurePreMergeCommitHook(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading existing hook")
}

func TestEnsurePreMergeCommitHook_MkdirAllFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics not applicable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission checks don't apply")
	}
	dir := t.TempDir()
	// .git exists but is not writable, so MkdirAll(.git/hooks) fails.
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(gitDir, 0o755) })

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	err := ensurePreMergeCommitHook(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating")
}

func TestEnsurePreMergeCommitHook_WriteFileFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics not applicable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: permission checks don't apply")
	}
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	// Remove write permission so os.WriteFile on the hook file fails.
	require.NoError(t, os.Chmod(hooksDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(hooksDir, 0o755) })

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	err := ensurePreMergeCommitHook(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}

func TestEnsurePreMergeCommitHook_ChmodFails(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	origExe := executableFunc
	t.Cleanup(func() { executableFunc = origExe })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origChmod := chmodFunc
	t.Cleanup(func() { chmodFunc = origChmod })
	chmodFunc = func(string, os.FileMode) error {
		return os.ErrPermission
	}

	err := ensurePreMergeCommitHook(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "setting permissions")
}

// --- resolveHooksDir ---

func TestResolveHooksDir_NotGitRepo(t *testing.T) {
	// Not a git repo: git fails, falls back to .git/hooks.
	dir := t.TempDir()
	got := resolveHooksDir(dir)
	assert.Equal(t, filepath.Join(dir, ".git", "hooks"), got)
}

func TestResolveHooksDir_DefaultGitRepo(t *testing.T) {
	// Derive expected path from git itself so the test is resilient
	// against a global core.hooksPath set in the developer's git config.
	dir := t.TempDir()
	initTestRepo(t, dir)
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-path", "hooks").Output()
	require.NoError(t, err)
	expected := strings.TrimSpace(string(out))
	if !filepath.IsAbs(expected) {
		expected = filepath.Join(dir, expected)
	}
	got := resolveHooksDir(dir)
	assert.Equal(t, filepath.Clean(expected), got)
}

func TestResolveHooksDir_CustomRelativeHooksPath(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	require.NoError(t, exec.Command("git", "-C", dir, "config",
		"core.hooksPath", "custom-hooks").Run())
	got := resolveHooksDir(dir)
	assert.Equal(t, filepath.Join(dir, "custom-hooks"), got)
}

func TestResolveHooksDir_CustomAbsoluteHooksPath(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	absPath := filepath.Join(dir, "abs-hooks")
	require.NoError(t, exec.Command("git", "-C", dir, "config",
		"core.hooksPath", absPath).Run())
	got := resolveHooksDir(dir)
	assert.Equal(t, absPath, got)
}

func TestRunMergeDriverInstall_DropsAndWarnsForUnrepresentableIgnore(t *testing.T) {
	// .mdsmith.yml ignore patterns containing whitespace or `!`
	// negation cannot be represented in a .gitattributes managed
	// block. The install command drops them but warns on stderr
	// so the operator notices the divergence between the merge
	// driver scope and `mdsmith fix`'s ignore semantics.
	dir := t.TempDir()
	initTestRepo(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("ignore:\n  - \"with space.md\"\n  - \"!negated.md\"\n  - \"vendor/**\"\n"), 0o644))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	stderr := captureStderr(func() {
		assert.Equal(t, 0, runMergeDriverInstall(nil))
	})
	assert.Contains(t, stderr, "skipped unsupported ignore patterns")
	assert.Contains(t, stderr, "with space.md")
	assert.Contains(t, stderr, "!negated.md")

	attrs, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	content := string(attrs)
	assert.Contains(t, content, "vendor/** -merge",
		"representable ignore patterns survive")
	assert.NotContains(t, content, "with space.md",
		"unrepresentable ignore patterns are dropped from the managed block")
	assert.NotContains(t, content, "!negated.md",
		"negation patterns are dropped from the managed block")
}

func TestRunMergeDriverInstall_FailsWhenGitattributesIsDir(t *testing.T) {
	// .gitattributes is a directory, so WriteGitattributes returns
	// an error. The install command must surface it with exit 2,
	// not silently succeed.
	dir := t.TempDir()
	initTestRepo(t, dir)
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".gitattributes"), 0o755))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriverInstall(nil))
	})
	assert.Contains(t, got, "updating .gitattributes")
}

func TestRunMergeDriverInstall_FailsWhenHooksDirNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filesystem semantics differ on Windows")
	}
	// Replace .git/hooks with a regular file so MkdirAll inside
	// ensurePreMergeCommitHook fails specifically at the hook step
	// (registerMergeDriver and WriteGitattributes still succeed).
	// The error must be surfaced with a clear
	// "installing pre-merge-commit hook" prefix.
	dir := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.RemoveAll(hooksDir))
	require.NoError(t, os.WriteFile(hooksDir, []byte("not a directory"), 0o644))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriverInstall(nil))
	})
	assert.Contains(t, got, "installing pre-merge-commit hook")
}

func TestRunMergeDriverInstall_CustomIncludeGlobs(t *testing.T) {
	// Explicit args replace the default include set so callers can
	// scope the merge driver to a custom pattern. The .gitattributes
	// managed block must use the supplied globs verbatim.
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	captureStderr(func() {
		assert.Equal(t, 0, runMergeDriverInstall([]string{"docs/**/*.md", "CHANGELOG.md"}))
	})

	attrs, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	content := string(attrs)
	assert.Contains(t, content, "docs/**/*.md merge=mdsmith")
	assert.Contains(t, content, "CHANGELOG.md merge=mdsmith")
	assert.NotContains(t, content, "*.md merge=mdsmith\n*.markdown",
		"default include set must be replaced when custom globs are given")
}

func TestMergeAndClean_DashPrefixedFilenames_NoOptionInjection(t *testing.T) {
	// Regression test: file paths starting with "-" must not be
	// interpreted as git options. The "--" separator added to the
	// git merge-file call prevents option injection.
	// Use relative paths so the argv elements passed to git actually
	// start with "-" (absolute paths like /tmp/dir/-base.md do not).
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	content := "# Hello\n"
	require.NoError(t, os.WriteFile("-base.md", []byte(content), 0o644))
	require.NoError(t, os.WriteFile("-ours.md", []byte(content), 0o644))
	require.NoError(t, os.WriteFile("-theirs.md", []byte(content), 0o644))

	_, code := mergeAndClean("-base.md", "-ours.md", "-theirs.md", 1<<20)
	assert.Equal(t, 0, code, "merge with dash-prefixed filenames must succeed")
}

func TestMergeAndClean_PreservesOursFileMode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits not meaningful on Windows")
	}
	dir := t.TempDir()

	base := filepath.Join(dir, "base.md")
	ours := filepath.Join(dir, "ours.md")
	theirs := filepath.Join(dir, "theirs.md")

	content := "# Hello\n"
	require.NoError(t, os.WriteFile(base, []byte(content), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte(content), 0o600))
	require.NoError(t, os.WriteFile(theirs, []byte(content), 0o644))

	_, code := mergeAndClean(base, ours, theirs, 1<<20)
	require.Equal(t, 0, code)

	info, err := os.Stat(ours)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"mergeAndClean must preserve the original permissions of ours")
}

func TestMergeFileMode_ExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o755))

	got := mergeFileMode(path, 0o644)
	assert.Equal(t, os.FileMode(0o755), got)
}

func TestMergeFileMode_MissingFile_UsesDefault(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nonexistent.md")
	got := mergeFileMode(missing, 0o644)
	assert.Equal(t, os.FileMode(0o644), got)
}

func TestGuardRegularFile_LstatNonENOENTError_ReturnsError(t *testing.T) {
	orig := lstatFn
	t.Cleanup(func() { lstatFn = orig })
	lstatFn = func(string) (os.FileInfo, error) {
		return nil, fmt.Errorf("mock lstat failure")
	}
	err := guardRegularFile("anypath")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lstat")
}

func TestMergeAndClean_PreReadGuardFails_ExitsTwo(t *testing.T) {
	// guardFn call sequence in mergeAndClean:
	//   1 = ours, 2 = base, 3 = theirs (loop), 4 = ours pre-read, 5 = ours pre-write
	// This test exercises call 4 (the pre-read re-check).
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	content := "# Hello\n"
	base := filepath.Join(dir, "base.md")
	ours := filepath.Join(dir, "ours.md")
	theirs := filepath.Join(dir, "theirs.md")
	require.NoError(t, os.WriteFile(base, []byte(content), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte(content), 0o644))
	require.NoError(t, os.WriteFile(theirs, []byte(content), 0o644))

	var calls int
	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = func(path string) error {
		calls++
		if calls == 4 {
			return fmt.Errorf("injected pre-read guard")
		}
		return orig(path)
	}

	got := captureStderr(func() {
		_, code := mergeAndClean(base, ours, theirs, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected pre-read guard")
}

func TestMergeAndClean_ReGuardFails_ExitsTwo(t *testing.T) {
	// guardFn call sequence in mergeAndClean:
	//   1 = ours, 2 = base, 3 = theirs (loop), 4 = ours pre-read, 5 = ours pre-write
	// This test exercises call 5 (the pre-write re-check).
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	content := "# Hello\n"
	base := filepath.Join(dir, "base.md")
	ours := filepath.Join(dir, "ours.md")
	theirs := filepath.Join(dir, "theirs.md")
	require.NoError(t, os.WriteFile(base, []byte(content), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte(content), 0o644))
	require.NoError(t, os.WriteFile(theirs, []byte(content), 0o644))

	var calls int
	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = func(path string) error {
		calls++
		if calls == 5 { // 5th call = re-check of ours before write
			return fmt.Errorf("injected: %s not regular", path)
		}
		return orig(path)
	}

	got := captureStderr(func() {
		_, code := mergeAndClean(base, ours, theirs, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected")
}

func TestMergeAndClean_WriteFileFails_ExitsTwo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()

	content := "# Hello\n"
	base := filepath.Join(dir, "base.md")
	ours := filepath.Join(dir, "ours.md")
	theirs := filepath.Join(dir, "theirs.md")
	require.NoError(t, os.WriteFile(base, []byte(content), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte(content), 0o644))
	require.NoError(t, os.WriteFile(theirs, []byte(content), 0o644))

	orig := osWriteFile
	t.Cleanup(func() { osWriteFile = orig })
	osWriteFile = func(string, []byte, os.FileMode) error {
		return fmt.Errorf("mock write failure")
	}

	got := captureStderr(func() {
		_, code := mergeAndClean(base, ours, theirs, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "writing cleaned merge")
}

func TestFixAtRealPath_WriteToPathnameFails_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	content := []byte("# Hello\n")
	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, content, 0o644))
	require.NoError(t, os.WriteFile(ours, content, 0o644))

	orig := osWriteFile
	t.Cleanup(func() { osWriteFile = orig })
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		if name == pathname {
			return fmt.Errorf("mock: write to pathname failed")
		}
		return os.WriteFile(name, data, perm)
	}

	got := captureStderr(func() {
		_, code := fixAtRealPath(content, ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "writing to")
}

func TestFixAtRealPath_WriteToOursFails_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	content := []byte("# Hello\n")
	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, content, 0o644))
	require.NoError(t, os.WriteFile(ours, content, 0o644))

	orig := osWriteFile
	t.Cleanup(func() { osWriteFile = orig })
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		if name == ours {
			return fmt.Errorf("mock: write to ours failed")
		}
		return os.WriteFile(name, data, perm)
	}

	got := captureStderr(func() {
		_, code := fixAtRealPath(content, ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "writing merge output")
}

// guardCallN returns a guardFn replacement that delegates to the real
// guardRegularFile for the first n-1 calls, then returns an error on call n,
// then delegates again for all subsequent calls.
func guardCallN(n int, msg string) func(string) error {
	var calls int
	return func(path string) error {
		calls++
		if calls == n {
			return fmt.Errorf("%s: %s", path, msg)
		}
		return guardRegularFile(path)
	}
}

func TestFixAtRealPath_ThirdGuardFails_ExitsTwo(t *testing.T) {
	// 3rd guardFn call = re-check of pathname immediately before write.
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = guardCallN(3, "injected pre-write guard")

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected pre-write guard")
}

func TestFixAtRealPath_FourthGuardFails_ExitsTwo(t *testing.T) {
	// guardFn call sequence in fixAtRealPath + readAndRestore (backup exists):
	//   1 = pathname, 2 = ours (start), 3 = pathname pre-write,
	//   4 = pathname pre-read (readAndRestore), 5 = pathname pre-restore,
	//   6 = ours pre-final-write
	// This test exercises call 4 (pre-read guard in readAndRestore).
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = guardCallN(4, "injected pre-read guard")

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected pre-read guard")
}

func TestFixAtRealPath_FifthGuardFails_ExitsTwo(t *testing.T) {
	// guardFn call 5 = re-check of pathname before restore write.
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = guardCallN(5, "injected pre-restore guard")

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected pre-restore guard")
}

func TestFixAtRealPath_SixthGuardFails_ExitsTwo(t *testing.T) {
	// guardFn call 6 = re-check of ours before final write.
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = guardCallN(6, "injected pre-ours-write guard")

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected pre-ours-write guard")
}

func TestFixAtRealPath_FixFails_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := fixFileInPlaceFn
	t.Cleanup(func() { fixFileInPlaceFn = orig })
	fixFileInPlaceFn = func(string, int64) error {
		return fmt.Errorf("mock fix failure")
	}

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "fix failed")
}

func TestFixAtRealPath_PathnameNotExist_RemovesAfterFix(t *testing.T) {
	// When pathname doesn't exist before the merge, fixAtRealPath should
	// remove the temp file it created rather than restoring non-existent content.
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "newfile.md")
	ours := filepath.Join(dir, "ours.md")
	// pathname does NOT exist — backup will be ENOENT.
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	fixed, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
	assert.Equal(t, 0, code)
	assert.NotEmpty(t, fixed)
	// pathname should have been removed after the fix cycle.
	_, statErr := os.Stat(pathname)
	assert.True(t, os.IsNotExist(statErr), "pathname should be removed after fix")
}

func TestFixAtRealPath_GuardBeforeRemoveFails_ExitsTwo(t *testing.T) {
	// 6th guardFn call = re-check of pathname before os.Remove in the
	// ENOENT-backup branch of readAndRestore.
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "newfile.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	// guardFn call sequence for the ENOENT-backup path:
	//   1 = pathname at top of fixAtRealPath
	//   2 = ours at top of fixAtRealPath
	//   3 = pathname re-check before write
	//   4 = pathname pre-read in readAndRestore
	//   5 = pathname re-check before os.Remove in readAndRestore
	// (the restore guard is skipped because backupErr is ENOENT)
	guardFn = guardCallN(5, "injected pre-remove guard")

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected pre-remove guard")
}

func TestFixAtRealPath_ReadFixedFileFails_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := readFileLimited
	t.Cleanup(func() { readFileLimited = orig })
	readFileLimited = func(string, int64) ([]byte, error) {
		return nil, fmt.Errorf("mock read fixed failure")
	}

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "reading fixed file")
}

func TestFixAtRealPath_RestoreWriteFails_ExitsTwo(t *testing.T) {
	// Restore write (2nd osWriteFile call) fails.
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# Hello\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	var writeCount int
	orig := osWriteFile
	t.Cleanup(func() { osWriteFile = orig })
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		writeCount++
		if writeCount == 2 { // second write = restore
			return fmt.Errorf("mock restore failure")
		}
		return orig(name, data, perm)
	}

	got := captureStderr(func() {
		_, code := fixAtRealPath([]byte("# Hello\n"), ours, pathname, 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "restoring")
}

func TestFixAtRealPath_PreservesOursFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits not meaningful on Windows")
	}

	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	content := []byte("# Hello\n\nWorld.\n")
	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, content, 0o644))
	require.NoError(t, os.WriteFile(ours, content, 0o600))

	fixed, code := fixAtRealPath(content, ours, pathname, 1<<20)
	require.Equal(t, 0, code)
	assert.NotEmpty(t, fixed)

	info, err := os.Stat(ours)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"fixAtRealPath must preserve the original permissions of ours")
}
