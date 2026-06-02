package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/release"
)

// fixtureChannels is a pre-loaded channel slice for the check/apply
// entry-point tests that should not drive the real `extract`.
func fixtureChannels() []release.Channel {
	return []release.Channel{{
		Title: "A", Summary: "s", Mechanism: "push", Artifact: "cli",
		Command: "cmd", Audience: "aud", Platforms: []string{"go"},
		URL: "https://example.test/a", Weight: 1,
	}}
}

func TestRunSyncChannels_CheckClean(t *testing.T) {
	if testing.Short() {
		t.Skip("builds cmd/mdsmith; skipped under -short")
	}
	chdirTo(t, repoRoot(t))
	// After the committed sync the data file matches; --check exits 0.
	assert.Equal(t, 0, run([]string{"sync-channels", "--check"}))
}

func TestRunSyncChannels_ApplyNoChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("builds cmd/mdsmith; skipped under -short")
	}
	chdirTo(t, repoRoot(t))
	// The repo is in sync; an apply run produces no edits and exits 0.
	assert.Equal(t, 0, run([]string{"sync-channels"}))
}

func TestRunSyncChannels_ExtraArgRejected(t *testing.T) {
	// The subcommand takes no positional arguments.
	assert.Equal(t, 2, run([]string{"sync-channels", "extra"}))
}

func TestRunSyncChannels_LoadFailsExitsNonZero(t *testing.T) {
	// Empty root: no channel dir, so LoadChannels errors before any
	// shell-out, and runSyncChannels surfaces it via reportError.
	exit := runSyncChannels(t.TempDir(), []string{"--check"})
	assert.NotEqual(t, 0, exit)
}

func TestRunSyncChannelsCheck_DriftExits1(t *testing.T) {
	// Pre-loaded fixture, missing data file → drift → exit 1.
	exit := runSyncChannelsCheck(t.TempDir(), fixtureChannels())
	assert.Equal(t, 1, exit)
}

func TestRunSyncChannelsCheck_Clean(t *testing.T) {
	root := t.TempDir()
	chs := fixtureChannels()
	_, err := release.WriteChannelsData(root, chs)
	require.NoError(t, err)
	assert.Equal(t, 0, runSyncChannelsCheck(root, chs))
}

func TestRunSyncChannelsCheck_ReadErrorExitsNonZero(t *testing.T) {
	// Data file is a directory → CheckChannelsData errors → reportError.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, "website", "data", "channels.yaml"), 0o755))
	exit := runSyncChannelsCheck(root, fixtureChannels())
	assert.NotEqual(t, 0, exit)
}

func TestRunSyncChannelsApply_WritesThenNoChange(t *testing.T) {
	root := t.TempDir()
	chs := fixtureChannels()
	assert.Equal(t, 0, runSyncChannelsApply(root, chs)) // changed → "wrote"
	assert.Equal(t, 0, runSyncChannelsApply(root, chs)) // unchanged
}

func TestRunSyncChannelsApply_ErrorExitsNonZero(t *testing.T) {
	// website/data is a file → MkdirAll fails → WriteChannelsData errors.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "website"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "website", "data"), []byte("x"), 0o644))
	exit := runSyncChannelsApply(root, fixtureChannels())
	assert.NotEqual(t, 0, exit)
}
