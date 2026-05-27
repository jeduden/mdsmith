package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/release"
)

// repoRoot resolves the project root from this test file's
// location (two parents up from cmd/mdsmith-release/). Used by
// the integration tests below that need to invoke the live
// `mdsmith extract messaging` against the real source.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// chdirTo switches the test's cwd for the duration of the test
// and restores it on cleanup. `run` resolves the subcommand
// root from os.Getwd, so the cwd is the seam.
func chdirTo(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))
}

// fixtureMessaging returns a non-zero Messaging value for the
// direct-entry-point tests. Distinct per-field strings catch a
// cross-wiring regression in MessagingTargets.
func fixtureMessaging() *release.Messaging {
	return &release.Messaging{
		Title:                       "T",
		Summary:                     "S",
		Eyebrow:                     "EYE",
		HeadlinePre:                 "PRE",
		HeadlineEm:                  "EM",
		HeadlinePost:                "POST",
		Lead:                        "LEAD",
		Tagline:                     "TAG",
		VSCodeDescription:           "VSC",
		VSCodeOverview:              "VSC-OVERVIEW",
		ClaudeCodeLSPDescription:    "LSP",
		ClaudeCodeSkillsDescription: "SKILLS",
		ClaudeCodeAuditDescription:  "AUDIT",
	}
}

func TestRunSyncMessaging_CheckClean(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles cmd/mdsmith; skipped under -short")
	}
	chdirTo(t, repoRoot(t))
	// After the previous sync the tree is clean by the CI gate's
	// definition; --check must exit 0.
	assert.Equal(t, 0, run([]string{"sync-messaging", "--check"}))
}

func TestRunSyncMessaging_ApplyNoChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles cmd/mdsmith; skipped under -short")
	}
	chdirTo(t, repoRoot(t))
	// The repo is already in sync; an apply run must produce no
	// edits (every target reports "unchanged") and exit 0.
	assert.Equal(t, 0, run([]string{"sync-messaging"}))
}

func TestRunSyncMessagingCheck_ReportsDriftExits1(t *testing.T) {
	// Empty root: every target file is missing, so CheckMessaging
	// returns a list of <missing> drifts and runSyncMessagingCheck
	// must exit 1 after formatting the report. This is the
	// drift-reporting branch the clean-tree tests above don't
	// reach.
	root := t.TempDir()
	exit := runSyncMessagingCheck(root, fixtureMessaging())
	assert.Equal(t, 1, exit)
}

func TestRunSyncMessagingApply_FailsWhenRequiredTargetMissing(t *testing.T) {
	// Empty root with no non-fragment target files; ApplyMessaging
	// returns an error ("required file missing"), and
	// runSyncMessagingApply must surface it via reportError (exit
	// code 1).
	root := t.TempDir()
	exit := runSyncMessagingApply(root, fixtureMessaging())
	assert.Equal(t, 1, exit)
}

func TestRunSyncMessaging_LoadFailsExitsNonZero(t *testing.T) {
	// runSyncMessaging on a root without docs/brand/messaging.md
	// makes the embedded `mdsmith extract` shell-out fail; the
	// reportError branch should return non-zero.
	if testing.Short() {
		t.Skip("compiles cmd/mdsmith; skipped under -short")
	}
	root := t.TempDir()
	exit := runSyncMessaging(root, []string{})
	assert.NotEqual(t, 0, exit)
}

func TestRunSyncMessaging_ExtraArgRejected(t *testing.T) {
	// The subcommand takes no positional arguments; passing one
	// hits the NArg() != 0 branch and returns 2.
	assert.Equal(t, 2, run([]string{"sync-messaging", "extra"}))
}

// applyFixtureFiles mirrors internal/release/applyTestFixtures
// just enough to drive runSyncMessagingApply through a happy
// path that actually writes files (Changed=true for every
// target). Used to cover the changed-marker branch in
// runSyncMessagingApply that the failure-path tests above don't
// reach.
func applyFixtureFiles(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"website/hugo.toml": `baseURL = "x"

[params]
  description = "stale"
  version = "0.0.0-dev"
`,
		"website/content/_index.md": `---
title: "x"
summary: "stale"
hero:
  eyebrow: "old"
  headline_pre: "a"
  headline_em: "b"
  headline_post: "c"
  lead: "old"
---
Body.
`,
		"npm/mdsmith/package.json": `{
  "name": "@mdsmith/cli",
  "description": "stale"
}
`,
		"python/pyproject.toml": `[project]
name = "mdsmith"
description = "stale"
`,
		"editors/vscode/package.json": `{
  "name": "mdsmith",
  "description": "stale"
}
`,
		"editors/claude-code/.claude-plugin/plugin.json": `{
  "name": "a",
  "description": "stale"
}
`,
		"editors/claude-code-skills/.claude-plugin/plugin.json": `{
  "name": "b",
  "description": "stale"
}
`,
		"editors/claude-code-audit/.claude-plugin/plugin.json": `{
  "name": "c",
  "description": "stale"
}
`,
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
}

func TestRunSyncMessagingCheck_PropagatesReadError(t *testing.T) {
	// Replace one target with a directory of the same name so
	// release.CheckMessaging returns a non-nil error. This drives
	// runSyncMessagingCheck's err-handling branch (exit 1 via
	// reportError) — the branch the clean and drift tests don't
	// reach.
	root := t.TempDir()
	applyFixtureFiles(t, root)
	pkg := filepath.Join(root, "npm/mdsmith/package.json")
	require.NoError(t, os.Remove(pkg))
	require.NoError(t, os.Mkdir(pkg, 0o755))
	exit := runSyncMessagingCheck(root, fixtureMessaging())
	assert.NotEqual(t, 0, exit)
}

func TestRunSyncMessagingApply_WritesChangedMarker(t *testing.T) {
	// All 15 surfaces present and drifted from the fixture
	// Messaging; runSyncMessagingApply iterates results and
	// flips r.Changed=true for each — the changed-marker branch
	// the missing-target test does not reach.
	root := t.TempDir()
	applyFixtureFiles(t, root)
	assert.Equal(t, 0, runSyncMessagingApply(root, fixtureMessaging()))
}
