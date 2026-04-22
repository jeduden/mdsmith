package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plan 84: symlinks are skipped by default across discovery and
// explicit walks; users must opt in with --follow-symlinks (CLI) or
// follow-symlinks: true (config).

// TestE2E_Symlink_DefaultDeny_ExternalTargetSkipped is the core
// security test: a repo with a symlink pointing to a file outside
// the project must not be walked by default. Running `fix` would
// otherwise overwrite that external file.
func TestE2E_Symlink_DefaultDeny_ExternalTargetSkipped(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"rules:\n  no-trailing-spaces: true\n")
	writeFixture(t, project, "ok.md", "# Title\n\nClean body.\n")

	// Place a dirty markdown file OUTSIDE the project and symlink
	// it in. Without default-deny, `check` would find it.
	externalFile := filepath.Join(external, "evil.md")
	require.NoError(t, os.WriteFile(externalFile,
		[]byte("# Evil\n\ntrailing   \n"), 0o644))
	require.NoError(t, os.Symlink(externalFile,
		filepath.Join(project, "evil.md")))

	// Default: symlink is skipped, only ok.md is seen, exit 0.
	_, stderr, exitCode := runBinaryInDir(t, project, "",
		"check", "--no-color", "--no-gitignore", ".")
	assert.Equal(t, 0, exitCode,
		"expected exit 0 with symlink skipped by default, got %d; stderr: %s",
		exitCode, stderr)
}

// TestE2E_Symlink_DefaultDeny_ExplicitFileArg asserts that the
// secure default also covers explicit, non-glob path arguments.
// `mdsmith check ./evil.md` must not process a symlink target.
func TestE2E_Symlink_DefaultDeny_ExplicitFileArg(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"rules:\n  no-trailing-spaces: true\n")

	externalFile := filepath.Join(external, "dirty.md")
	require.NoError(t, os.WriteFile(externalFile,
		[]byte("# Evil\n\ntrailing   \n"), 0o644))
	require.NoError(t, os.Symlink(externalFile,
		filepath.Join(project, "evil.md")))

	// Default-deny: explicit symlinked file arg is silently skipped;
	// no diagnostics reported, exit 0.
	_, _, exitCode := runBinaryInDir(t, project, "", "check",
		"--no-color", "--no-gitignore", "evil.md")
	assert.Equal(t, 0, exitCode,
		"explicit symlinked file arg must be skipped by default")

	// Opt-in: the symlinked file is visited and flagged.
	_, _, exitOpt := runBinaryInDir(t, project, "", "check",
		"--no-color", "--no-gitignore", "--follow-symlinks", "evil.md")
	assert.Equal(t, 1, exitOpt,
		"with --follow-symlinks the symlink target is linted")
}

// TestE2E_Symlink_FollowSymlinksFlag_OptsIn asserts the new
// --follow-symlinks CLI flag walks symlinked entries.
func TestE2E_Symlink_FollowSymlinksFlag_OptsIn(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"rules:\n  no-trailing-spaces: true\n")

	externalFile := filepath.Join(external, "dirty.md")
	require.NoError(t, os.WriteFile(externalFile,
		[]byte("# Dirty\n\ntrailing   \n"), 0o644))
	require.NoError(t, os.Symlink(externalFile,
		filepath.Join(project, "linked.md")))

	// Opting in follows the symlink and flags the trailing-space issue.
	_, _, exitCode := runBinaryInDir(t, project, "",
		"check", "--no-color", "--no-gitignore", "--follow-symlinks", ".")
	assert.Equal(t, 1, exitCode,
		"expected exit 1 with --follow-symlinks exposing dirty linked file")
}

// TestE2E_Symlink_FollowSymlinksConfigKey_OptsIn asserts the new
// follow-symlinks: true config key works.
func TestE2E_Symlink_FollowSymlinksConfigKey_OptsIn(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"follow-symlinks: true\nrules:\n  no-trailing-spaces: true\n")

	externalFile := filepath.Join(external, "dirty.md")
	require.NoError(t, os.WriteFile(externalFile,
		[]byte("# Dirty\n\ntrailing   \n"), 0o644))
	require.NoError(t, os.Symlink(externalFile,
		filepath.Join(project, "linked.md")))

	_, _, exitCode := runBinaryInDir(t, project, "",
		"check", "--no-color", "--no-gitignore", ".")
	assert.Equal(t, 1, exitCode,
		"expected exit 1 with follow-symlinks: true exposing dirty linked file")
}

// TestE2E_Symlink_DirSymlinkAlwaysSkipped asserts that a symlink
// pointing at a directory is skipped even in opt-in mode. Rationale:
// filepath.Walk is Lstat-based and would silently return zero files
// for a symlink root; `--follow-symlinks` applies only to file
// symlinks, as documented on ResolveOpts.FollowSymlinks.
func TestE2E_Symlink_DirSymlinkAlwaysSkipped(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"rules:\n  no-trailing-spaces: true\n")

	// External dir with a dirty file. Project contains a symlinked
	// directory pointing at it.
	require.NoError(t, os.WriteFile(filepath.Join(external, "dirty.md"),
		[]byte("# Evil\n\ntrailing   \n"), 0o644))
	require.NoError(t, os.Symlink(external, filepath.Join(project, "linked")))

	// Explicit arg + --follow-symlinks: symlinked directory is
	// skipped silently; no findings.
	_, _, exitOpt := runBinaryInDir(t, project, "", "check",
		"--no-color", "--no-gitignore", "--follow-symlinks", "linked")
	assert.Equal(t, 0, exitOpt,
		"symlinked directory must be skipped even with --follow-symlinks")
}

// TestE2E_Symlink_DirSymlinkWithMdName asserts that a symlink named
// like a markdown file but pointing at a directory is NOT picked up
// as a markdown file, even in opt-in mode. This guards against a
// subtle failure mode: without the target os.Stat, Lstat-based info
// reports IsDir==false and isMarkdown(path) is true, so the entry
// would be queued and then fail later on "is a directory" read.
func TestE2E_Symlink_DirSymlinkWithMdName(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"files:\n  - \"**/*.md\"\nrules:\n  no-trailing-spaces: true\n")
	writeFixture(t, project, "ok.md", "# OK\n\nClean body.\n")

	// Symlink whose name ends in .md but whose target is a directory.
	require.NoError(t, os.Symlink(external, filepath.Join(project, "evil.md")))

	// Discovery (no explicit file args) with --follow-symlinks: the
	// dir-symlink-with-.md-name must be filtered out.
	_, stderr, exitCode := runBinaryInDir(t, project, "",
		"check", "--no-color", "--no-gitignore", "--follow-symlinks")
	assert.Equal(t, 0, exitCode,
		"dir-symlink-with-.md-name must not be read; stderr: %s", stderr)
	assert.NotContains(t, stderr, "is a directory",
		"symlink-to-dir must not leak as a markdown read error; stderr: %s",
		stderr)
}

// TestE2E_Symlink_LegacyFlag_OverridesFollowConfig verifies that the
// deprecated --no-follow-symlinks flag still has meaning: it forces
// deny even when `follow-symlinks: true` is set in config. This lets
// users run one-off secure invocations without editing the config.
func TestE2E_Symlink_LegacyFlag_OverridesFollowConfig(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"follow-symlinks: true\nrules:\n  no-trailing-spaces: true\n")

	externalFile := filepath.Join(external, "dirty.md")
	require.NoError(t, os.WriteFile(externalFile,
		[]byte("# Evil\n\ntrailing   \n"), 0o644))
	require.NoError(t, os.Symlink(externalFile,
		filepath.Join(project, "linked.md")))

	// Baseline: with follow-symlinks:true in config, the linked
	// file is walked and the dirty line surfaces.
	_, _, withConfig := runBinaryInDir(t, project, "", "check",
		"--no-color", "--no-gitignore", ".")
	require.Equal(t, 1, withConfig,
		"follow-symlinks: true config must expose the linked file")

	// Legacy --no-follow-symlinks overrides the config and forces
	// deny for this invocation; no findings.
	_, _, withLegacy := runBinaryInDir(t, project, "", "check",
		"--no-color", "--no-gitignore", "--no-follow-symlinks", ".")
	assert.Equal(t, 0, withLegacy,
		"legacy --no-follow-symlinks must force deny over config opt-in")
}

// TestE2E_Symlink_LegacyNoFollowConfig_Deprecation verifies that the
// old `no-follow-symlinks:` key still parses and emits a deprecation
// warning on stderr.
func TestE2E_Symlink_LegacyNoFollowConfig_Deprecation(t *testing.T) {
	project := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"no-follow-symlinks:\n  - \"**\"\nrules:\n  no-trailing-spaces: true\n")
	writeFixture(t, project, "ok.md", "# Title\n\nClean body.\n")

	_, stderr, exitCode := runBinaryInDir(t, project, "",
		"check", "--no-color", "--no-gitignore", ".")
	assert.Equal(t, 0, exitCode,
		"expected exit 0, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "no-follow-symlinks",
		"expected deprecation warning mentioning no-follow-symlinks, got: %s",
		stderr)
	assert.Contains(t, stderr, "deprecated",
		"expected deprecation warning, got: %s", stderr)
}

// TestE2E_Symlink_FixRespectsFollowSymlinks ensures `fix` honors
// --follow-symlinks: the dirty external file is never rewritten
// (atomic rename replaces the symlink itself, not its target — see
// plan 83 section C), and the in-project symlink is only visited
// when the flag is set.
func TestE2E_Symlink_FixRespectsFollowSymlinks(t *testing.T) {
	project := t.TempDir()
	external := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	writeFixture(t, project, ".mdsmith.yml",
		"rules:\n  no-trailing-spaces: true\n")

	externalFile := filepath.Join(external, "dirty.md")
	const dirtyContent = "# Dirty\n\ntrailing   \n"
	require.NoError(t, os.WriteFile(externalFile,
		[]byte(dirtyContent), 0o644))
	linked := filepath.Join(project, "linked.md")
	require.NoError(t, os.Symlink(externalFile, linked))

	// Default-deny: fix does not visit the symlink. The link remains
	// a symlink and the external file is untouched.
	_, _, _ = runBinaryInDir(t, project, "",
		"fix", "--no-color", "--no-gitignore", ".")
	lstat, err := os.Lstat(linked)
	require.NoError(t, err)
	assert.NotZero(t, lstat.Mode()&os.ModeSymlink,
		"default-deny must leave the symlink intact")
	got, err := os.ReadFile(externalFile)
	require.NoError(t, err)
	assert.Equal(t, dirtyContent, string(got),
		"fix must not rewrite symlinked external file by default")

	// Opt-in: fix visits the symlink. Atomic rename replaces the
	// symlink with a regular file containing the fixed content; the
	// external target stays untouched (plan 83 write-side protection).
	_, _, _ = runBinaryInDir(t, project, "",
		"fix", "--no-color", "--no-gitignore", "--follow-symlinks", ".")
	lstat2, err := os.Lstat(linked)
	require.NoError(t, err)
	assert.Zero(t, lstat2.Mode()&os.ModeSymlink,
		"fix --follow-symlinks must replace symlink with a regular file")
	projectContent, err := os.ReadFile(linked)
	require.NoError(t, err)
	assert.NotContains(t, string(projectContent), "   \n",
		"fix --follow-symlinks must rewrite the in-project file")
	extAfter, err := os.ReadFile(externalFile)
	require.NoError(t, err)
	assert.Equal(t, dirtyContent, string(extAfter),
		"fix must never rewrite the external symlink target directly")
}
