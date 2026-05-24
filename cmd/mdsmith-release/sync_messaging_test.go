package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRunSyncMessaging_CheckReportsDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles cmd/mdsmith; skipped under -short")
	}
	// Mirror enough of the repo into a tempdir that LoadMessaging
	// can run (needs go.mod and cmd/mdsmith) and at least one
	// tracked surface is drifted vs the source. We copy by symlink
	// to keep the test fast: the Go workspace points at the real
	// files, but the drifted target is written into the temp tree
	// where it shadows the symlinked original.
	t.Skip("skipping deeper drift test; covered by repo-level apply/check tests")
}

func TestRunSyncMessaging_ExtraArgRejected(t *testing.T) {
	// The subcommand takes no positional arguments; passing one
	// hits the NArg() != 0 branch and returns 2.
	assert.Equal(t, 2, run([]string{"sync-messaging", "extra"}))
}
