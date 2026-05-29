package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/release"
)

func TestRunSyncParityRules_CheckClean(t *testing.T) {
	// After the previous sync the committed fragment matches the
	// generator output, so --check must exit 0.
	chdirTo(t, repoRoot(t))
	assert.Equal(t, 0, run([]string{"sync-parity-rules", "--check"}))
}

func TestRunSyncParityRules_ApplyNoChanges(t *testing.T) {
	// Apply on an already-synced tree reports idempotent success.
	chdirTo(t, repoRoot(t))
	assert.Equal(t, 0, run([]string{"sync-parity-rules"}))
}

func TestRunSyncParityRulesApply_WritesWhenMissing(t *testing.T) {
	// Empty root: the generator creates the missing fragment under
	// docs/research/benchmarks/ and exits 0. Covers the "rewrote"
	// branch the clean-tree tests do not reach.
	root := t.TempDir()
	exit := runSyncParityRulesApply(root)
	assert.Equal(t, 0, exit)
	_, err := os.Stat(filepath.Join(root, release.ParityRulesFragmentFile))
	require.NoError(t, err)
}

func TestRunSyncParityRulesCheck_PropagatesReadError(t *testing.T) {
	// Empty root: the fragment is missing, so CheckParityRulesFragment
	// surfaces a read error and runSyncParityRulesCheck returns
	// non-zero via reportError.
	root := t.TempDir()
	exit := runSyncParityRulesCheck(root)
	assert.NotEqual(t, 0, exit)
}

func TestRunSyncParityRulesCheck_DriftAfterTamper(t *testing.T) {
	// Apply then mutate the fragment so CheckParityRulesFragment
	// returns a non-empty drift message; runSyncParityRulesCheck must
	// exit 1 (drift branch, distinct from the missing-file branch).
	root := t.TempDir()
	require.Equal(t, 0, runSyncParityRulesApply(root))
	path := filepath.Join(root, release.ParityRulesFragmentFile)
	require.NoError(t, os.WriteFile(path, []byte("hand-edited\n"), 0o644))
	exit := runSyncParityRulesCheck(root)
	assert.Equal(t, 1, exit)
}

func TestRunSyncParityRulesApply_PropagatesError(t *testing.T) {
	// A directory at the target file path makes os.ReadFile fail with
	// a non-NotExist error; Apply surfaces it and the runner exits
	// non-zero.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, release.ParityRulesFragmentFile), 0o755))
	exit := runSyncParityRulesApply(root)
	assert.NotEqual(t, 0, exit)
}

func TestRunSyncParityRules_ExtraArgRejected(t *testing.T) {
	// The subcommand takes no positional arguments; passing one hits
	// the NArg() != 0 branch and returns 2.
	assert.Equal(t, 2, run([]string{"sync-parity-rules", "extra"}))
}
