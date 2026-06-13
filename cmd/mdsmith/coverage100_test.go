package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
)

// writeMergeInputs writes base/ours/theirs files in dir and returns their
// paths. content selects a clean merge (identical) or a two-conflict merge.
func writeMergeInputs(t *testing.T, dir string, conflicting bool) (base, ours, theirs string) {
	t.Helper()
	base = filepath.Join(dir, "base")
	ours = filepath.Join(dir, "ours")
	theirs = filepath.Join(dir, "theirs")
	if conflicting {
		// Two separated conflict regions make git merge-file exit 2, which
		// this driver treats as a fatal (non-1) merge error.
		require.NoError(t, os.WriteFile(base, []byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n"), 0o644))
		require.NoError(t, os.WriteFile(ours, []byte("1\nO2\n3\n4\n5\n6\n7\nO8\n9\n"), 0o644))
		require.NoError(t, os.WriteFile(theirs, []byte("1\nT2\n3\n4\n5\n6\n7\nT8\n9\n"), 0o644))
		return base, ours, theirs
	}
	body := []byte("hello\nworld\n")
	require.NoError(t, os.WriteFile(base, body, 0o644))
	require.NoError(t, os.WriteFile(ours, body, 0o644))
	require.NoError(t, os.WriteFile(theirs, body, 0o644))
	return base, ours, theirs
}

func TestMergeAndClean_FatalMergeError(t *testing.T) {
	dir := t.TempDir()
	base, ours, theirs := writeMergeInputs(t, dir, true)
	captureStderr(func() {
		_, rc := mergeAndClean(base, ours, theirs, 1<<20)
		assert.Equal(t, 2, rc, "two-conflict merge is a fatal merge error")
	})
}

func TestMergeAndClean_ReadResultError(t *testing.T) {
	dir := t.TempDir()
	base, ours, theirs := writeMergeInputs(t, dir, false)
	// A maxBytes smaller than the merged file makes ReadFileLimited fail
	// after a clean merge — the "reading merge result" branch.
	captureStderr(func() {
		_, rc := mergeAndClean(base, ours, theirs, 1)
		assert.Equal(t, 2, rc)
	})
}

func TestRunMergeDriverRun_LoadConfigError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte("not: [valid\n"), 0o644))
	base, ours, theirs := writeMergeInputs(t, dir, false)
	t.Chdir(dir)
	captureStderr(func() {
		assert.Equal(t, 2, runMergeDriverRun([]string{base, ours, theirs, "p.md"}))
	})
}

func TestRunMergeDriverRun_MaxInputSizeError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("max-input-size: not-a-size\n"), 0o644))
	base, ours, theirs := writeMergeInputs(t, dir, false)
	t.Chdir(dir)
	captureStderr(func() {
		assert.Equal(t, 2, runMergeDriverRun([]string{base, ours, theirs, "p.md"}))
	})
}

func TestRunMergeDriverRun_MergeFails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))
	base, ours, theirs := writeMergeInputs(t, dir, true)
	t.Chdir(dir)
	captureStderr(func() {
		// mergeAndClean returns a non-zero code, which runMergeDriverRun
		// propagates.
		assert.Equal(t, 2, runMergeDriverRun([]string{base, ours, theirs, "p.md"}))
	})
}

func TestRunMergeDriverInstall_RegisterError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	orig := executableFunc
	// Return a temp-dir path so resolveInstalledBinary treats it as transient.
	executableFunc = func() (string, error) {
		return filepath.Join(os.TempDir(), "go-run-fake", "mdsmith"), nil
	}
	t.Cleanup(func() { executableFunc = orig })
	// Restrict PATH to only git so LookPath("mdsmith") fails,
	// while git rev-parse still resolves.
	pathWithOnlyGit(t)
	t.Chdir(dir)
	captureStderr(func() {
		// In a git repo, but registerMergeDriver fails because the binary
		// cannot be located via any fallback.
		assert.Equal(t, 2, runMergeDriverInstall(nil))
	})
}

func TestRegisterMergeDriver_GitConfigError(t *testing.T) {
	orig := executableFunc
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }
	t.Cleanup(func() { executableFunc = orig })
	// Not a git repository, so `git config` fails.
	t.Chdir(t.TempDir())
	err := registerMergeDriver()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git config")
}

func TestEnsurePreMergeCommitHook_ChmodError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	origExe := executableFunc
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }
	t.Cleanup(func() { executableFunc = origExe })
	origChmod := chmodFunc
	chmodFunc = func(string, os.FileMode) error { return errors.New("chmod failed") }
	t.Cleanup(func() { chmodFunc = origChmod })

	err := ensurePreMergeCommitHook(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "setting permissions")
}

func TestRunBacklinks_DiscoverError(t *testing.T) {
	dir := t.TempDir()
	badYml := filepath.Join(dir, "bad.yml")
	require.NoError(t, os.WriteFile(badYml, []byte("not: [valid\n"), 0o644))
	// A workspace-relative target passes validateBacklinksArgs, so we reach
	// discoverFiles; the bad config makes it return exit code 2, which
	// runBacklinks surfaces rather than treating as an empty result.
	captureStderr(func() {
		code := runBacklinks([]string{"-c", badYml, "target.md"})
		assert.Equal(t, 2, code)
	})
}

// chdirToRemoved changes into a fresh temp dir and then deletes it, so the
// process has no valid working directory and os.Getwd fails. t.Chdir
// restores the original directory at cleanup. This drives the Getwd-error
// fallbacks that a normal filesystem never reaches.
func chdirToRemoved(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mdsmith-nowd-*")
	require.NoError(t, err)
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))
}

func TestReportFlagParseErr_NilReturnsContinue(t *testing.T) {
	// A nil parse error means "no error": the caller should continue,
	// signalled by -1.
	assert.Equal(t, -1, reportFlagParseErr(nil, os.Stderr, "mdsmith: x"))
}

func TestDiscoverConfigPath_GetwdError(t *testing.T) {
	chdirToRemoved(t)
	// With no cwd, discoverConfigPath falls back to the empty-rooted default.
	assert.Equal(t, config.DefaultConfigPath(""), discoverConfigPath(""))
}

func TestRootDirFromConfig_GetwdError(t *testing.T) {
	chdirToRemoved(t)
	// An empty cfgPath with no cwd yields an empty root.
	assert.Equal(t, "", rootDirFromConfig(""))
}

func TestLoadConfigRaw_GetwdErrorFallsBackToDefaults(t *testing.T) {
	chdirToRemoved(t)
	cfg, path, err := loadConfigRaw("")
	require.NoError(t, err)
	assert.Equal(t, "", path, "no config discovered without a cwd")
	require.NotNil(t, cfg)
}

func TestRunQuery_LoadConfigError(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yml")
	require.NoError(t, os.WriteFile(bad, []byte("this: : : not yaml\n"), 0o644))
	// ResolveFilesWithOpts(["."]) succeeds, then loadConfig parses the bad
	// config and fails — the runQuery config-error branch.
	code := runQuery([]string{"-c", bad, "status: \"x\"", "."})
	assert.Equal(t, 2, code)
}

func TestRunQuery_ResolveFilesError(t *testing.T) {
	// An explicit, nonexistent file argument makes file resolution fail
	// before any config load.
	code := runQuery([]string{"status: \"x\"", filepath.Join(t.TempDir(), "nope.md")})
	assert.Equal(t, 2, code)
}

func TestExecuteMetricsRank_ResolveFilesError(t *testing.T) {
	// Metric selection and config load succeed, but an explicit nonexistent
	// file makes resolveRankFiles fail — the rank file-resolution branch.
	code := runMetricsRank([]string{filepath.Join(t.TempDir(), "missing.md")})
	assert.Equal(t, 2, code)
}
