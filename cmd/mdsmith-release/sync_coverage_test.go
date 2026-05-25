package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/release"
)

func TestRunSyncCoverageMatrix_CheckClean(t *testing.T) {
	// After the previous sync the on-disk page matches the
	// generator output, so --check must exit 0.
	chdirTo(t, repoRoot(t))
	assert.Equal(t, 0, run([]string{"sync-coverage-matrix", "--check"}))
}

func TestRunSyncCoverageMatrix_ApplyNoChanges(t *testing.T) {
	// Apply on an already-synced tree must report idempotent
	// success and exit 0.
	chdirTo(t, repoRoot(t))
	assert.Equal(t, 0, run([]string{"sync-coverage-matrix"}))
}

func TestRunSyncCoverageMatrixApply_WritesWhenMissing(t *testing.T) {
	// Empty root: the generator creates the missing file under
	// docs/research/markdownlint-coverage/ and exits 0. Covers
	// the "rewrote" branch of runSyncCoverageMatrixApply that
	// the clean-tree tests do not reach.
	root := t.TempDir()
	exit := runSyncCoverageMatrixApply(root)
	assert.Equal(t, 0, exit)
	_, err := os.Stat(filepath.Join(root, release.CoverageMatrixFile))
	require.NoError(t, err)
}

func TestRunSyncCoverageMatrixCheck_ReportsDriftExits1(t *testing.T) {
	// Empty root: the target file is missing, so
	// release.CheckCoverageMatrix surfaces a read error and
	// runSyncCoverageMatrixCheck returns a non-zero exit code
	// via reportError. This drives the err-handling branch the
	// clean-tree test does not reach.
	root := t.TempDir()
	exit := runSyncCoverageMatrixCheck(root)
	assert.NotEqual(t, 0, exit)
}

func TestRunSyncCoverageMatrixCheck_DriftAfterTamper(t *testing.T) {
	// Apply then mutate the generated file so CheckCoverageMatrix
	// returns a non-empty drift message; runSyncCoverageMatrixCheck
	// must exit 1 (drift branch, distinct from the missing-file
	// error branch above).
	root := t.TempDir()
	require.Equal(t, 0, runSyncCoverageMatrixApply(root))
	path := filepath.Join(root, release.CoverageMatrixFile)
	require.NoError(t, os.WriteFile(path, []byte("hand-edited\n"), 0o644))
	exit := runSyncCoverageMatrixCheck(root)
	assert.Equal(t, 1, exit)
}

func TestRunSyncCoverageMatrix_ExtraArgRejected(t *testing.T) {
	// The subcommand takes no positional arguments; passing one
	// hits the NArg() != 0 branch and returns 2.
	assert.Equal(t, 2, run([]string{"sync-coverage-matrix", "extra"}))
}
