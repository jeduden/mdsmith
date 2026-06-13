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
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeDriverRules_ExcludesGitIndexMutators(t *testing.T) {
	// Precondition: MDS048 (git-hook-sync) is a registered git-index
	// mutator, so the exclusion is meaningful rather than a no-op.
	mds048 := rule.ByID("MDS048")
	require.NotNil(t, mds048, "precondition: MDS048 must be registered")
	m, ok := mds048.(rule.GitIndexMutator)
	require.True(t, ok && m.MutatesGitIndex(),
		"precondition: MDS048 must declare rule.GitIndexMutator")

	rules := mergeDriverRules()
	for _, r := range rules {
		gm, ok := r.(rule.GitIndexMutator)
		assert.False(t, ok && gm.MutatesGitIndex(),
			"the merge driver runs inside `git merge` (which holds "+
				".git/index.lock); it must exclude every git-index-mutating "+
				"rule, but %s remained", r.ID())
	}
	assert.NotEmpty(t, rules,
		"mergeDriverRules must still run the content-regenerating rules")
}

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

func TestFixMergedContent_FixFails_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	orig := fixSourceFn
	t.Cleanup(func() { fixSourceFn = orig })
	fixSourceFn = func(string, []byte, int64) ([]byte, error) {
		return nil, fmt.Errorf("mock fix failure")
	}

	got := captureStderr(func() {
		_, code := fixMergedContent([]byte("# Hello\n"), ours, "PLAN.md", 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "fix failed")
}

func TestFixMergedContent_GuardOursFails_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	origFix := fixSourceFn
	t.Cleanup(func() { fixSourceFn = origFix })
	fixSourceFn = func(_ string, src []byte, _ int64) ([]byte, error) {
		return src, nil
	}

	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = func(path string) error {
		return fmt.Errorf("%s: injected pre-ours-write guard", path)
	}

	got := captureStderr(func() {
		_, code := fixMergedContent([]byte("# Hello\n"), ours, "PLAN.md", 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "injected pre-ours-write guard")
}

func TestFixMergedContent_WriteToOursFails_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	origFix := fixSourceFn
	t.Cleanup(func() { fixSourceFn = origFix })
	fixSourceFn = func(_ string, src []byte, _ int64) ([]byte, error) {
		return src, nil
	}

	orig := osWriteFile
	t.Cleanup(func() { osWriteFile = orig })
	osWriteFile = func(string, []byte, os.FileMode) error {
		return fmt.Errorf("mock: write to ours failed")
	}

	got := captureStderr(func() {
		_, code := fixMergedContent([]byte("# Hello\n"), ours, "PLAN.md", 1<<20)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "writing merge output")
}

func TestFixMergedContent_DoesNotTouchWorktreePathname(t *testing.T) {
	// The worktree path exists; the driver must neither read nor
	// write it — a byte-identical rewrite would still change stat
	// data and abort the parent merge (see runMergeDriverRun).
	dir := t.TempDir()
	pathname := filepath.Join(dir, "PLAN.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(pathname, []byte("worktree\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))
	before, err := os.Lstat(pathname)
	require.NoError(t, err)

	origFix := fixSourceFn
	t.Cleanup(func() { fixSourceFn = origFix })
	fixSourceFn = func(_ string, _ []byte, _ int64) ([]byte, error) {
		return []byte("# Fixed\n"), nil
	}

	fixed, code := fixMergedContent([]byte("# Hello\n"), ours, pathname, 1<<20)
	require.Equal(t, 0, code)
	assert.Equal(t, "# Fixed\n", string(fixed))

	result, err := os.ReadFile(ours)
	require.NoError(t, err)
	assert.Equal(t, "# Fixed\n", string(result), "fixed content must land in ours")

	after, err := os.Lstat(pathname)
	require.NoError(t, err)
	assert.True(t, before.ModTime().Equal(after.ModTime()),
		"worktree pathname mtime must be untouched")
	worktree, err := os.ReadFile(pathname)
	require.NoError(t, err)
	assert.Equal(t, "worktree\n", string(worktree),
		"worktree pathname content must be untouched")
}

func TestFixMergedContent_PathnameNotExist_Succeeds(t *testing.T) {
	// %P may not exist in the worktree (add/add merges). The driver
	// never touches it, so a missing pathname is not an error and
	// nothing is created at that path.
	dir := t.TempDir()
	pathname := filepath.Join(dir, "newfile.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o644))

	origFix := fixSourceFn
	t.Cleanup(func() { fixSourceFn = origFix })
	fixSourceFn = func(_ string, src []byte, _ int64) ([]byte, error) {
		return src, nil
	}

	fixed, code := fixMergedContent([]byte("# Hello\n"), ours, pathname, 1<<20)
	assert.Equal(t, 0, code)
	assert.NotEmpty(t, fixed)
	_, statErr := os.Stat(pathname)
	assert.True(t, os.IsNotExist(statErr),
		"pathname must not be created by the merge driver")
}

func TestFixMergedContent_PreservesOursFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits not meaningful on Windows")
	}

	dir := t.TempDir()
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# Hello\n"), 0o600))

	origFix := fixSourceFn
	t.Cleanup(func() { fixSourceFn = origFix })
	fixSourceFn = func(_ string, src []byte, _ int64) ([]byte, error) {
		return src, nil
	}

	fixed, code := fixMergedContent([]byte("# Hello\n"), ours, "PLAN.md", 1<<20)
	require.Equal(t, 0, code)
	assert.NotEmpty(t, fixed)

	info, err := os.Stat(ours)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"fixMergedContent must preserve the original permissions of ours")
}

func TestFixMergedSource_RegeneratesCatalog_NoDiskWrites(t *testing.T) {
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("rules:\n  catalog: true\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plans", "alpha.md"),
		[]byte("---\ntitle: Alpha\n---\n\n# Alpha\n\nContent.\n"), 0o644))

	// Stale catalog body: regeneration must fill in the alpha row.
	source := "# Doc\n\n<?catalog\nglob: \"plans/*.md\"\nsort: title\n" +
		"row: \"- [{title}]({filename})\"\n?>\n<?/catalog?>\n"

	fixed, err := fixMergedSource("CATALOG.md", []byte(source), 1<<20)
	require.NoError(t, err)
	assert.Contains(t, string(fixed), "- [Alpha](plans/alpha.md)",
		"catalog must regenerate from neighbour files on disk")

	_, statErr := os.Stat(filepath.Join(dir, "CATALOG.md"))
	assert.True(t, os.IsNotExist(statErr),
		"fixMergedSource must not create the file on disk")
}

func TestFixMergedSource_ConfigLoadFails(t *testing.T) {
	dir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte(":\tnot yaml"), 0o644))

	_, err = fixMergedSource("PLAN.md", []byte("# Hello\n"), 1<<20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

// --- merge-driver ci-install (npm-ci-style: verify, never write) ---

func TestRunMergeDriver_CIInstall_HelpFlag_ExitsZero(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		got := captureStderr(func() {
			assert.Equal(t, 0, runMergeDriver([]string{"ci-install", flag}))
		})
		assert.Contains(t, got, "Usage: mdsmith merge-driver",
			"help must print the usage banner for %q, not just exit 0", flag)
	}
}

func TestRunMergeDriver_CIInstall_NotInRepo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"),
		[]byte("not a real gitdir"), 0o644))
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install"}))
	})
	assert.Contains(t, got, "not in a git repository")
}

// ci-install verifies the committed .gitattributes against .mdsmith.yml
// rather than scoping a fresh install, so it rejects the positional glob
// args that `install` accepts.
func TestRunMergeDriver_CIInstall_RejectsGlobArgs(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install", "docs/**/*.md"}))
	})
	assert.Contains(t, got, "no glob arguments")
}

func TestRunMergeDriver_CIInstall_MissingGitattributes_Fails(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install"}))
	})
	assert.Contains(t, got, "merge-driver install",
		"a missing .gitattributes must name the fix command")
}

func TestRunMergeDriver_CIInstall_NoManagedBlock_Fails(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("*.txt text\n"), 0o644))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install"}))
	})
	assert.Contains(t, got, "managed block")
}

func TestRunMergeDriver_CIInstall_InSync_DoesNotModifyGitattributes(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// install writes a canonical, in-sync .gitattributes.
	captureStderr(func() {
		require.Equal(t, 0, runMergeDriver([]string{"install"}))
	})
	attrPath := filepath.Join(dir, ".gitattributes")
	before, err := os.ReadFile(attrPath)
	require.NoError(t, err)

	got := captureStderr(func() {
		assert.Equal(t, 0, runMergeDriver([]string{"ci-install"}))
	})
	assert.Contains(t, got, "verified")

	after, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"ci-install must not modify .gitattributes")
}

// Drift is the exact condition that bounced the merge queue: an ignore
// pattern added to .mdsmith.yml without regenerating .gitattributes.
// ci-install must catch it and fail loudly without rewriting the file
// (which is what would dirty the worktree and abort the queue's merge).
func TestRunMergeDriver_CIInstall_Drift_Fails(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	captureStderr(func() {
		require.Equal(t, 0, runMergeDriver([]string{"install"}))
	})
	attrPath := filepath.Join(dir, ".gitattributes")
	before, err := os.ReadFile(attrPath)
	require.NoError(t, err)

	// Add an ignore pattern so the expected glob set diverges from the
	// committed .gitattributes.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("ignore:\n  - \"vendor/**\"\n"), 0o644))

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install"}))
	})
	assert.Contains(t, got, "out of sync")

	after, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"ci-install must not modify .gitattributes even on drift")
}

func TestRunMergeDriver_CIInstall_LoadConfigError(t *testing.T) {
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
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install"}))
	})
	assert.Contains(t, got, "loading config")
}

// pathWithOnlyGit points PATH at a temp dir holding just a `git` symlink
// for the rest of the test. resolveInstalledBinary's PATH and $GOPATH
// lookups then fail (no `mdsmith`, and no `go` to query GOPATH) while the
// merge-driver's own `git rev-parse` still resolves. Skips on Windows,
// where os.Symlink needs privilege; the coverage gate runs on Linux.
func pathWithOnlyGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink-based PATH isolation is POSIX-only")
	}
	gitPath, err := exec.LookPath("git")
	require.NoError(t, err, "git must be on PATH for this test")
	binDir := t.TempDir()
	require.NoError(t, os.Symlink(gitPath, filepath.Join(binDir, "git")))
	t.Setenv("PATH", binDir)
}

// When verification passes but the binary cannot be located, ci-install
// must surface registerMergeDriver's error and exit 2 — before writing
// the hook.
func TestRunMergeDriver_CIInstall_RegisterDriverError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// In-sync .gitattributes so verification passes and control reaches
	// registerMergeDriver.
	captureStderr(func() {
		require.Equal(t, 0, runMergeDriver([]string{"install"}))
	})

	// Make the binary unlocatable (transient exe + a PATH without
	// mdsmith or go), keeping git available for `rev-parse`.
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}
	pathWithOnlyGit(t)

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install"}))
	})
	// Pin the failure to the registration step: a hook-step failure
	// would carry ci-install's "installing pre-merge-commit hook:"
	// prefix, which registerMergeDriver's bare error does not — so its
	// absence proves we failed before the hook was touched.
	assert.Contains(t, got, "cannot locate mdsmith binary")
	assert.NotContains(t, got, "installing pre-merge-commit hook",
		"must fail at registerMergeDriver, before the hook step")
}

// When verification and driver registration pass but a user-authored
// (unmanaged) pre-merge-commit hook is present, ci-install must surface
// ensurePreMergeCommitHook's refusal and exit 2.
func TestRunMergeDriver_CIInstall_HookError_UnmanagedHook(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// install writes an in-sync .gitattributes plus a managed hook.
	captureStderr(func() {
		require.Equal(t, 0, runMergeDriver([]string{"install"}))
	})

	// Replace the managed hook with an unmanaged one so
	// ensurePreMergeCommitHook refuses to overwrite it.
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-merge-commit")
	require.NoError(t, os.WriteFile(hookPath,
		[]byte("#!/bin/sh\necho user\n"), 0o755))

	got := captureStderr(func() {
		assert.Equal(t, 2, runMergeDriver([]string{"ci-install"}))
	})
	// Assert the specific refusal text, not just "pre-merge-commit" —
	// that substring also appears in the success message and the usage
	// banner, so it would not distinguish the unmanaged-hook branch from
	// a different hook failure.
	assert.Contains(t, got, "not managed by mdsmith")
}

// --- verifyGitattributes ---

// A non-regular .gitattributes (here a directory) must be rejected by
// the lstat guard before any read, so ci-install cannot be tricked into
// reading a path outside the repository.
func TestVerifyGitattributes_NonRegularFile_ReturnsTwo(t *testing.T) {
	dir := t.TempDir()
	attr := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.Mkdir(attr, 0o755))

	got := captureStderr(func() {
		assert.Equal(t, 2, verifyGitattributes(attr, githooks.Globs{}))
	})
	assert.Contains(t, got, "not a regular file")
}

func TestVerifyGitattributes_ReadError_ReturnsTwo(t *testing.T) {
	dir := t.TempDir()
	attr := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(attr, []byte("x"), 0o644))

	orig := osReadFile
	t.Cleanup(func() { osReadFile = orig })
	osReadFile = func(string) ([]byte, error) { return nil, fmt.Errorf("boom") }

	got := captureStderr(func() {
		assert.Equal(t, 2, verifyGitattributes(attr, githooks.Globs{}))
	})
	// Assert the injected error is surfaced, not just the generic
	// "reading" prefix, so a swallowed underlying error fails the test.
	assert.Contains(t, got, "reading")
	assert.Contains(t, got, "boom")
}

// --- writeHookFile error paths ---

func TestWriteHookFile_GuardFails(t *testing.T) {
	orig := guardFn
	t.Cleanup(func() { guardFn = orig })
	guardFn = func(string) error { return fmt.Errorf("injected guard error") }

	dir := t.TempDir()
	err := writeHookFile(filepath.Join(dir, "pre-merge-commit"), []byte("content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}

func TestWriteHookFile_CreateTempFails(t *testing.T) {
	orig := hookCreateTempFn
	t.Cleanup(func() { hookCreateTempFn = orig })
	hookCreateTempFn = func(string, string) (*os.File, error) {
		return nil, fmt.Errorf("injected createtemp error")
	}

	dir := t.TempDir()
	err := writeHookFile(filepath.Join(dir, "pre-merge-commit"), []byte("content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}

func TestWriteHookFile_WriteFails(t *testing.T) {
	orig := hookCreateTempFn
	t.Cleanup(func() { hookCreateTempFn = orig })
	hookCreateTempFn = func(dir, pattern string) (*os.File, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		_ = f.Close()
		return f, nil
	}

	dir := t.TempDir()
	err := writeHookFile(filepath.Join(dir, "pre-merge-commit"), []byte("content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}

func TestWriteHookFile_SyncFails(t *testing.T) {
	orig := syncFileFn
	t.Cleanup(func() { syncFileFn = orig })
	syncFileFn = func(*os.File) error { return fmt.Errorf("injected sync error") }

	dir := t.TempDir()
	err := writeHookFile(filepath.Join(dir, "pre-merge-commit"), []byte("content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}

func TestWriteHookFile_CloseFails(t *testing.T) {
	orig := closeFileFn
	t.Cleanup(func() { closeFileFn = orig })
	closeFileFn = func(f *os.File) error {
		_ = f.Close()
		return fmt.Errorf("injected close error")
	}

	dir := t.TempDir()
	err := writeHookFile(filepath.Join(dir, "pre-merge-commit"), []byte("content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}

func TestWriteHookFile_RenameFails(t *testing.T) {
	orig := osRenameFn
	t.Cleanup(func() { osRenameFn = orig })
	osRenameFn = func(string, string) error { return fmt.Errorf("injected rename error") }

	dir := t.TempDir()
	err := writeHookFile(filepath.Join(dir, "pre-merge-commit"), []byte("content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}
