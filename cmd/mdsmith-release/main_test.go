package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/release"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunStampThenCheck exercises the CLI dispatcher end-to-end:
// stamp a temp tree with a real version, then run check against
// the same tree (which should now fail because the dev sentinel
// is gone). Confirms the subcommand wiring and the cwd-as-root
// contract.
func TestRunStampThenCheck(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 0, run([]string{"stamp", "1.2.3"}))
	// After stamping, check should fail because the manifests no
	// longer carry the dev sentinel.
	assert.Equal(t, 1, run([]string{"check"}))
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	assert.Equal(t, 2, run([]string{"frobnicate"}))
}

func TestRunHelpExitsZero(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		assert.Equal(t, 0, run([]string{arg}), "%s", arg)
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	assert.Equal(t, 2, run(nil))
}

func TestRunRejectsBadArity(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"stamp without version", []string{"stamp"}},
		{"stamp with extra args", []string{"stamp", "1.2.3", "extra"}},
		{"check with extra arg", []string{"check", "extra"}},
		{"build-npm without args", []string{"build-npm"}},
		{"build-npm with one arg", []string{"build-npm", "art"}},
		{"build-wheels without args", []string{"build-wheels"}},
		{"build-wheels with one arg", []string{"build-wheels", "art"}},
		{"build-flatpak without args", []string{"build-flatpak"}},
		{"build-flatpak with one arg", []string{"build-flatpak", "art"}},
		{"package-obsidian without args", []string{"package-obsidian"}},
		{"package-obsidian with one arg", []string{"package-obsidian", "dist"}},
		{"build-website with three positionals", []string{"build-website", "a", "b", "c"}},
	}
	for _, c := range cases {
		assert.Equal(t, 2, run(c.args), c.name)
	}
}

func TestRunStampReturnsErrorOnInvalidVersion(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 1, run([]string{"stamp", "v1.2.3"}))
}

// TestReportErrorMapsExitCodes pins the wrapper that translates a
// (possibly nil) error into the integer exit code main returns.
func TestReportErrorMapsExitCodes(t *testing.T) {
	assert.Equal(t, 0, reportError(nil))
	assert.Equal(t, 1, reportError(errors.New("sentinel error")))
}

// TestReportFlagParseErrNilReturnsContinue exercises the nil
// branch of reportFlagParseErr that real subcommand callers
// only hit when fs.Parse() succeeds. A direct unit test ensures
// the contract holds.
func TestReportFlagParseErrNilReturnsContinue(t *testing.T) {
	assert.Equal(t, -1, reportFlagParseErr(nil, os.Stderr, "test"))
}

// TestSubcommandHelpExitsZero exercises the pflag --help branch
// of reportFlagParseErr per subcommand. pflag prints the Usage
// itself, so the dispatcher just needs to surface exit code 0.
func TestSubcommandHelpExitsZero(t *testing.T) {
	for _, sub := range []string{
		"stamp", "check", "build-npm", "build-wheels",
		"build-flatpak", "package-obsidian",
		"sync-docs", "build-website", "verify-website-links",
		"verify-install-picker",
		"sync-messaging",
		"sync-parity-rules",
		"sync-channels",
		"render-scoop-manifest",
		"render-winget-manifest",
	} {
		assert.Equal(t, 0, run([]string{sub, "--help"}), "%s --help", sub)
	}
}

// TestSubcommandRejectsUnknownFlag exercises the non-help, non-nil
// branch of reportFlagParseErr.
func TestSubcommandRejectsUnknownFlag(t *testing.T) {
	for _, sub := range []string{
		"stamp", "check", "build-npm", "build-wheels",
		"build-flatpak", "package-obsidian",
		"sync-docs", "build-website", "verify-website-links",
		"verify-install-picker",
		"sync-messaging",
		"sync-parity-rules",
		"sync-channels",
		"render-scoop-manifest",
		"render-winget-manifest",
	} {
		assert.Equal(t, 2, run([]string{sub, "--bogus"}), "%s --bogus", sub)
	}
}

// TestRunVerifyInstallPicker drives the handler's three outcomes:
// missing --dir (usage, 2), a homepage that matches the loaded
// channels.yaml (0), and a root with no channels.yaml so the load
// fails (1).
func TestRunVerifyInstallPicker(t *testing.T) {
	// No --dir → usage, exit 2.
	assert.Equal(t, 2, runVerifyInstallPicker(t.TempDir(), nil))

	// A root whose channels.yaml matches the rendered homepage → 0.
	root := t.TempDir()
	cy := filepath.Join(root, release.ChannelsDataFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(cy), 0o755))
	require.NoError(t, os.WriteFile(cy,
		[]byte("- title: Go\n  command: go install x\n  weight: 1\n"), 0o644))
	htmlDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(htmlDir, "index.html"),
		[]byte(`<div class="install-row" data-cmd-default="go install x"></div>`), 0o644))
	assert.Equal(t, 0, runVerifyInstallPicker(root, []string{"--dir", htmlDir}))

	// A root with no channels.yaml → load error, exit 1.
	assert.Equal(t, 1, runVerifyInstallPicker(t.TempDir(), []string{"--dir", htmlDir}))
}

// TestRunCheckOnDevSentinel exercises runCheck's success branch
// (the println "all manifests pinned at ..." line) which the
// stamp-then-check test above does not reach because check
// always fails after a successful stamp.
func TestRunCheckOnDevSentinel(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 0, run([]string{"check"}))
}

// TestRunBuildNpmEndToEnd dispatches through `run build-npm` so
// the subcommand wiring (FlagSet parse, NArg() validation,
// reportError translation) gets exercised end-to-end with
// realistic positional args, not just the bad-arity branches.
func TestRunBuildNpmEndToEnd(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	// Stamp first so npm/mdsmith/package.json carries the version
	// build-npm will read. Use any valid SemVer.
	require.Equal(t, 0, run([]string{"stamp", "1.2.3"}))

	// Stage fake artifacts (the same set internal/release tests
	// use). build-npm only cares that the asset filenames exist.
	artifacts := filepath.Join(root, "artifacts")
	require.NoError(t, os.MkdirAll(artifacts, 0o755))
	for _, asset := range []string{
		"mdsmith-linux-amd64",
		"mdsmith-linux-arm64",
		"mdsmith-darwin-amd64",
		"mdsmith-darwin-arm64",
		"mdsmith-windows-amd64.exe",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(artifacts, asset),
			[]byte("#!/bin/sh\necho fake\n"), 0o755))
	}
	out := filepath.Join(root, "dist")

	assert.Equal(t, 0, run([]string{"build-npm", "artifacts", "dist"}))
	for _, plat := range []string{"linux-x64", "darwin-arm64", "win32-x64"} {
		_, err := os.Stat(filepath.Join(out, plat, "package.json"))
		assert.NoError(t, err, "%s package.json", plat)
	}
}

// TestRunBuildNpmReportsError dispatches through run build-npm
// with a missing artifacts dir so reportError's non-nil branch
// fires for build-npm.
func TestRunBuildNpmReportsError(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	require.Equal(t, 0, run([]string{"stamp", "1.2.3"}))
	assert.Equal(t, 1, run([]string{"build-npm", "missing-artifacts", "dist"}))
}

// TestRunBuildFlatpakEndToEnd dispatches through `run build-flatpak`
// so the FlagSet parse, arity check, and reportError wiring run with
// realistic args, and asserts the manifest plus the staged binary it
// references land in out-dir.
func TestRunBuildFlatpakEndToEnd(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	artifacts := filepath.Join(root, "artifacts")
	require.NoError(t, os.MkdirAll(artifacts, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(artifacts, "mdsmith-linux-amd64"),
		[]byte("#!/bin/sh\necho fake\n"), 0o755))

	assert.Equal(t, 0, run([]string{"build-flatpak", "artifacts", "out"}))
	for _, f := range []string{
		"io.github.jeduden.mdsmith.yml",
		"mdsmith-linux-amd64",
	} {
		_, err := os.Stat(filepath.Join(root, "out", f))
		assert.NoError(t, err, f)
	}
}

func TestRunBuildFlatpakReportsError(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))
	// Missing artifacts dir → BuildFlatpak errors → exit 1.
	assert.Equal(t, 1, run([]string{"build-flatpak", "missing-artifacts", "out"}))
}

// TestRunPackageObsidianEndToEnd dispatches through `run
// package-obsidian` so the FlagSet parse, arity check, and reportError
// wiring run with realistic args, and asserts the zip lands in out-dir.
func TestRunPackageObsidianEndToEnd(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	dist := filepath.Join(root, "dist")
	require.NoError(t, os.MkdirAll(dist, 0o755))
	for _, name := range []string{
		"main.js", "styles.css", "mdsmith.wasm", "wasm_exec.js",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dist, name), []byte("x"), 0o644))
	}
	// Pretty-printed manifest with "version" on its own line, matching
	// the committed editors/obsidian/manifest.json and the format the
	// stamp step preserves.
	require.NoError(t, os.WriteFile(filepath.Join(dist, "manifest.json"),
		[]byte("{\n  \"id\": \"mdsmith\",\n  \"version\": \"9.9.9\"\n}\n"), 0o644))

	assert.Equal(t, 0, run([]string{"package-obsidian", "dist", "out"}))
	_, err = os.Stat(filepath.Join(root, "out", "mdsmith-obsidian-9.9.9.zip"))
	assert.NoError(t, err)
}

func TestRunPackageObsidianReportsError(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))
	// Missing dist dir → PackageObsidian errors → exit 1.
	assert.Equal(t, 1, run([]string{"package-obsidian", "missing-dist", "out"}))
}

// TestRunBuildWebsiteEndToEnd dispatches through `run
// build-website --no-fix` so the subcommand wiring (flag
// parse, positional defaulting, reportError) is exercised
// without shelling out to `go run ./cmd/mdsmith` (the fix
// pass is skipped). It also confirms the explicit src/dst
// positionals override the defaults.
func TestRunBuildWebsiteEndToEnd(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "top.md"), []byte("top body\n"), 0o644))
	dst := filepath.Join(root, "out")

	assert.Equal(t, 0, run([]string{"build-website", "--no-fix", src, dst}))
	got, err := os.ReadFile(filepath.Join(dst, "top.md"))
	require.NoError(t, err)
	assert.Equal(t, "top body\n", string(got))
}

// TestRunVerifyWebsiteLinksHappyPath dispatches through
// `run verify-website-links --dir <dir>` against a
// minimal Hugo-output fixture so the subcommand wiring
// (required-flag check, baseURL default, reportError) is
// exercised end-to-end.
func TestRunVerifyWebsiteLinksHappyPath(t *testing.T) {
	root := t.TempDir()
	ref := filepath.Join(root, "reference", "index.html")
	st := filepath.Join(root, "reference", "schema-types", "index.html")
	rule := filepath.Join(root, "rules", "mds001", "index.html")
	for _, dir := range []string{filepath.Dir(ref), filepath.Dir(st), filepath.Dir(rule)} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	require.NoError(t, os.WriteFile(ref,
		[]byte(`<a href="/reference/cli/">x</a>`), 0o644))
	require.NoError(t, os.WriteFile(st,
		[]byte(`<a href="/rules/mds020-required-structure/">x</a>`), 0o644))
	require.NoError(t, os.WriteFile(rule,
		[]byte(`<a href="/rules/mds021/">x</a>`), 0o644))

	assert.Equal(t, 0, run([]string{"verify-website-links", "--dir", root}))
}

// TestRunVerifyWebsiteLinksMissingDir drives the
// required-flag branch: omitting --dir prints usage and
// returns exit code 2 without touching the filesystem.
func TestRunVerifyWebsiteLinksMissingDir(t *testing.T) {
	assert.Equal(t, 2, run([]string{"verify-website-links"}))
}

// TestRunVerifyWebsiteLinksReportsError drives the
// reportError non-nil branch: pointing at an empty
// directory makes the first probe fail with
// "rendered HTML not found", so runVerifyWebsiteLinks
// must surface exit code 1.
func TestRunVerifyWebsiteLinksReportsError(t *testing.T) {
	assert.Equal(t, 1, run([]string{"verify-website-links", "--dir", t.TempDir()}))
}

// TestRunBuildWebsiteReportsError drives the reportError
// non-nil branch: src==dst trips the SyncDocs overlap guard,
// so runBuildWebsite must surface exit code 1.
func TestRunBuildWebsiteReportsError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.md"), []byte("x\n"), 0o644))
	assert.Equal(t, 1, run([]string{"build-website", "--no-fix", dir, dir}))
}

// TestRunBuildWheelsReportsError dispatches through run
// build-wheels for the fast-fail "python source missing" path so
// runBuildWheels gets full coverage without needing python.
func TestRunBuildWheelsReportsError(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	// writeFixture creates python/pyproject.toml so the
	// python-source check passes; missing artifacts trips the
	// per-build stat check instead.
	assert.Equal(t, 1, run([]string{"build-wheels", "missing-artifacts", "wheels"}))
}

// TestRunSyncDocsHappyPath dispatches through `run sync-docs` so
// the subcommand wiring (FlagSet parse, NArg() validation,
// reportError translation, default-toolkit handoff) gets
// exercised end-to-end against a small staged docs tree.
func TestRunSyncDocsHappyPath(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	require.NoError(t, os.WriteFile(filepath.Join(src, "intro.md"), []byte("intro body\n"), 0o644))

	assert.Equal(t, 0, run([]string{"sync-docs", src, dst}))

	body, err := os.ReadFile(filepath.Join(dst, "intro.md"))
	require.NoError(t, err)
	assert.Equal(t, "intro body\n", string(body))
}

// TestRunSyncDocsBadArity covers the `fs.NArg() != 2` branch for
// both under- and over-supply of positional args. Each case
// exits with the usage-error code (2) without touching the FS.
func TestRunSyncDocsBadArity(t *testing.T) {
	for _, argv := range [][]string{
		{"sync-docs"},
		{"sync-docs", "only-one"},
		{"sync-docs", "a", "b", "c"},
	} {
		assert.Equal(t, 2, run(argv), "%v", argv)
	}
}

// TestRunSyncDocsReportsErrorAsExitOne covers the reportError
// branch when the underlying release.SyncDocs call fails (here,
// missing source dir).
func TestRunSyncDocsReportsErrorAsExitOne(t *testing.T) {
	src := filepath.Join(t.TempDir(), "does-not-exist")
	dst := t.TempDir()
	assert.Equal(t, 1, run([]string{"sync-docs", src, dst}))
}

// writeFixture mirrors internal/release/version_test.go's
// fixtureManifests but without taking a dependency back on the
// internal package's test helpers.
func writeFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"editors/vscode/package.json": `{
  "name": "mdsmith",
  "version": "0.0.0-dev",
  "optionalDependencies": {
    "@mdsmith/cli": "0.0.0-dev"
  }
}
`,
		"editors/obsidian/manifest.json": `{
  "id": "mdsmith",
  "name": "mdsmith",
  "version": "0.0.0-dev"
}
`,
		"editors/obsidian/package.json": `{
  "name": "mdsmith-obsidian",
  "version": "0.0.0-dev"
}
`,
		"npm/mdsmith/package.json": `{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev",
  "optionalDependencies": {
    "@mdsmith/linux-x64": "0.0.0-dev",
    "@mdsmith/linux-arm64": "0.0.0-dev",
    "@mdsmith/darwin-x64": "0.0.0-dev",
    "@mdsmith/darwin-arm64": "0.0.0-dev",
    "@mdsmith/win32-x64": "0.0.0-dev"
  }
}
`,
		"python/pyproject.toml": `[project]
name = "mdsmith"
version = "0.0.0-dev"
`,
		"website/hugo.toml": `baseURL = "https://mdsmith.dev/"
title = "mdsmith"
[params]
  version = "0.0.0-dev"
`,
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
}

// TestRunRecordRotationHappyPath dispatches through `run` with
// a real per-secret file in a temp tree so the runRecordRotation
// success-with-change branch is covered end-to-end.
func TestRunRecordRotationHappyPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "development", "secret-rotations")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := "---\n" +
		"title: VSCE_PAT\n" +
		"lastRotated: \"2026-04-01\"\n" +
		"periodDays: 335\n" +
		"---\nbody\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vsce-pat.md"), []byte(body), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 0, run([]string{"record-rotation", "VSCE_PAT", "2026-05-12"}))
	// Re-run with the same date: returns 0 but logs the no-op
	// path through runRecordRotation.
	assert.Equal(t, 0, run([]string{"record-rotation", "VSCE_PAT", "2026-05-12"}))
}

// TestRunRecordRotationBadDate covers the err branch of
// runRecordRotation: a calendar-invalid date trips IsISODate
// inside release.RecordRotation and propagates back as exit 1.
func TestRunRecordRotationBadDate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "development", "secret-rotations")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 1, run([]string{"record-rotation", "ANY", "2026-02-31"}))
}

// TestRunRecordRotationRejectsBadArity covers the fs.NArg()
// guard in runRecordRotation. The CLI prints usage and returns
// 2 when the caller forgets the date arg.
func TestRunRecordRotationRejectsBadArity(t *testing.T) {
	assert.Equal(t, 2, run([]string{"record-rotation", "VSCE_PAT"}))
}

// TestRunCheckSecretRotationsRejectsBadArity covers the
// fs.NArg() guard in runCheckSecretRotations.
func TestRunCheckSecretRotationsRejectsBadArity(t *testing.T) {
	assert.Equal(t, 2, run([]string{"check-secret-rotations", "extra-arg"}))
}

// TestRunCheckSecretRotationsErrorsOnMissingDir covers the err
// branch of runCheckSecretRotations: with no secret-rotations
// directory in cwd, LoadRotations returns an error and the
// subcommand exits 1.
func TestRunCheckSecretRotationsErrorsOnMissingDir(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 1, run([]string{"check-secret-rotations"}))
}

// TestRunCheckSecretRotationsHappyPath dispatches through `run`
// with a per-secret file whose lastRotated is the workflow's
// `today` value (so no entries are due). The subcommand prints
// the "no secrets due for rotation" line and returns 0,
// covering runCheckSecretRotations' default success branches
// without needing a real `gh` binary on the runner.
func TestRunCheckSecretRotationsHappyPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "development", "secret-rotations")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// Make periodDays large enough that the entry is not due no
	// matter what real-clock day the test runs. 4000 days ~ 11
	// years from any lastRotated within the last decade.
	body := "---\n" +
		"title: VSCE_PAT\n" +
		"lastRotated: \"2026-05-12\"\n" +
		"periodDays: 4000\n" +
		"provider: Azure\n" +
		"issuerUrl: https://x\n" +
		"usedBy: r\n" +
		"scope: s\n" +
		"---\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "v.md"), []byte(body), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 0, run([]string{"check-secret-rotations"}))
}

// TestRunCheckSecretRotationsRejectsUnknownFlag covers the
// pflag parse-error path. ContinueOnError + reportFlagParseErr
// returns 2 with the message on stderr.
func TestRunCheckSecretRotationsRejectsUnknownFlag(t *testing.T) {
	assert.Equal(t, 2, run([]string{"check-secret-rotations", "--bogus"}))
}

// TestRunRecordRotationRejectsUnknownFlag covers the
// reportFlagParseErr branch of runRecordRotation.
func TestRunRecordRotationRejectsUnknownFlag(t *testing.T) {
	assert.Equal(t, 2, run([]string{"record-rotation", "--bogus", "T", "2026-05-12"}))
}

// TestRunRenderScoopManifestEndToEnd dispatches through
// `run render-scoop-manifest` against a temp checksums.txt so the
// subcommand wiring (arity check, open, parse, render, stdout) is
// exercised end-to-end.
func TestRunRenderScoopManifestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums,
		[]byte("deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead  mdsmith-windows-amd64.exe\n"),
		0o644))

	// Redirect stdout to capture manifest output.
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	code := run([]string{"render-scoop-manifest", "1.2.3", checksums})

	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	assert.Equal(t, 0, code)
	assert.Contains(t, out, `"version": "1.2.3"`)
	assert.Contains(t, out, "v1.2.3")
	assert.Contains(t, out, "deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead")
}

// TestRunRenderScoopManifestBadArity covers the arity check in
// runRenderScoopManifest.
func TestRunRenderScoopManifestBadArity(t *testing.T) {
	assert.Equal(t, 2, run([]string{"render-scoop-manifest"}))
	assert.Equal(t, 2, run([]string{"render-scoop-manifest", "1.2.3"}))
}

// TestRunRenderScoopManifestMissingChecksums covers the os.Open
// error branch.
func TestRunRenderScoopManifestMissingChecksums(t *testing.T) {
	assert.Equal(t, 1, run([]string{"render-scoop-manifest", "1.2.3",
		filepath.Join(t.TempDir(), "no-such-file.txt")}))
}

// TestRunRenderScoopManifestMissingHash covers the ParseChecksumFor
// error branch (file exists but target is absent).
func TestRunRenderScoopManifestMissingHash(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums,
		[]byte("abc123  mdsmith-linux-amd64\n"), 0o644))
	assert.Equal(t, 1, run([]string{"render-scoop-manifest", "1.2.3", checksums}))
}

// TestRunRenderWingetManifestEndToEnd dispatches through
// `run render-winget-manifest` and asserts the three YAML files
// land in the output directory.
func TestRunRenderWingetManifestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums,
		[]byte("cafe0000cafe0000cafe0000cafe0000cafe0000cafe0000cafe0000cafe0000  mdsmith-windows-amd64.exe\n"),
		0o644))
	outDir := filepath.Join(dir, "manifests")

	code := run([]string{"render-winget-manifest", "--out", outDir, "0.13.0", checksums})
	assert.Equal(t, 0, code)

	for _, name := range []string{
		"jeduden.mdsmith.yaml",
		"jeduden.mdsmith.installer.yaml",
		"jeduden.mdsmith.locale.en-US.yaml",
	} {
		data, err := os.ReadFile(filepath.Join(outDir, name))
		require.NoError(t, err, name)
		assert.Contains(t, string(data), "jeduden.mdsmith", name)
	}
}

// TestRunRenderWingetManifestBadArity covers the arity and missing
// --out checks.
func TestRunRenderWingetManifestBadArity(t *testing.T) {
	assert.Equal(t, 2, run([]string{"render-winget-manifest"}))
	assert.Equal(t, 2, run([]string{"render-winget-manifest", "1.2.3", "x.txt"})) // missing --out
}

// TestRunRenderWingetManifestMissingChecksums covers the os.Open
// error branch.
func TestRunRenderWingetManifestMissingChecksums(t *testing.T) {
	assert.Equal(t, 1, run([]string{"render-winget-manifest", "--out",
		t.TempDir(), "1.2.3",
		filepath.Join(t.TempDir(), "no-such-file.txt")}))
}

// TestRunRenderWingetManifestMissingHash covers the ParseChecksumFor
// error branch: the checksums file exists but lacks the Windows exe
// entry, so the subcommand exits 1.
func TestRunRenderWingetManifestMissingHash(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums,
		[]byte("abc123  mdsmith-linux-amd64\n"), 0o644))
	assert.Equal(t, 1, run([]string{"render-winget-manifest", "--out",
		filepath.Join(dir, "manifests"), "1.2.3", checksums}))
}

// TestRunRenderWingetManifestMkdirError covers the os.MkdirAll error
// branch: --out points under a regular file, so the directory cannot
// be created and the subcommand exits 1.
func TestRunRenderWingetManifestMkdirError(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums,
		[]byte("deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead  mdsmith-windows-amd64.exe\n"),
		0o644))
	// A regular file cannot have children, so MkdirAll(blocker/sub) fails.
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	assert.Equal(t, 1, run([]string{"render-winget-manifest", "--out",
		filepath.Join(blocker, "sub"), "1.2.3", checksums}))
}

// TestRunRenderWingetManifestWriteError covers the os.WriteFile error
// branch: the first manifest's target path is pre-created as a
// directory, so writing the file fails and the subcommand exits 1.
func TestRunRenderWingetManifestWriteError(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksums,
		[]byte("deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead  mdsmith-windows-amd64.exe\n"),
		0o644))
	outDir := filepath.Join(dir, "manifests")
	// Pre-create the first output name as a directory so os.WriteFile
	// to that path fails with "is a directory".
	require.NoError(t, os.MkdirAll(filepath.Join(outDir, "jeduden.mdsmith.yaml"), 0o755))
	assert.Equal(t, 1, run([]string{"render-winget-manifest", "--out",
		outDir, "0.13.0", checksums}))
}

// TestPrintCheckResult verifies the three formatting branches
// of the human-readable summary.
func TestPrintCheckResult(t *testing.T) {
	cases := []struct {
		name string
		res  release.CheckSecretRotationsResult
		want []string
	}{
		{
			name: "opened only",
			res:  release.CheckSecretRotationsResult{Opened: []string{"A", "B"}},
			want: []string{"opened secret-rotation reminders for: [A B]"},
		},
		{
			name: "skipped only",
			res:  release.CheckSecretRotationsResult{Skipped: []string{"C"}},
			want: []string{"existing open reminders (skipped): [C]"},
		},
		{
			name: "opened and skipped together",
			res: release.CheckSecretRotationsResult{
				Opened:  []string{"A"},
				Skipped: []string{"B"},
			},
			want: []string{
				"opened secret-rotation reminders for: [A]",
				"existing open reminders (skipped): [B]",
			},
		},
		{
			name: "neither",
			res:  release.CheckSecretRotationsResult{},
			want: []string{"no secrets due for rotation"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			printCheckResult(&buf, tc.res)
			for _, line := range tc.want {
				assert.Contains(t, buf.String(), line)
			}
		})
	}
}
