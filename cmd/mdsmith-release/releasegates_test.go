package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatesChdir switches into a fresh temp dir for the cwd-as-root
// contract and restores the original working directory afterward.
func gatesChdir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))
	return root
}

func gatesWriteWorkflow(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release.yml"), []byte(body), 0o644))
}

const gatesGoodWorkflow = `
jobs:
  gate:
    needs: [build]
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    needs: [build, gate]
    environment: release
    runs-on: ubuntu-latest
`

const gatesUngatedWorkflow = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    needs: [build]
    environment: release
    runs-on: ubuntu-latest
`

func TestRunCheckReleaseGatesAcceptsWiredWorkflow(t *testing.T) {
	root := gatesChdir(t)
	gatesWriteWorkflow(t, root, gatesGoodWorkflow)
	assert.Equal(t, 0, run([]string{"check-release-gates"}))
}

func TestRunCheckReleaseGatesFailsOnUngatedJob(t *testing.T) {
	root := gatesChdir(t)
	gatesWriteWorkflow(t, root, gatesUngatedWorkflow)
	assert.Equal(t, 1, run([]string{"check-release-gates"}))
}

func TestRunCheckReleaseGatesFailsWhenWorkflowMissing(t *testing.T) {
	gatesChdir(t) // no .github/workflows/release.yml written
	assert.Equal(t, 1, run([]string{"check-release-gates"}))
}

func TestRunCheckReleaseGatesRejectsExtraArg(t *testing.T) {
	assert.Equal(t, 2, run([]string{"check-release-gates", "extra"}))
}

func TestRunCheckReleaseGatesRejectsBadFlag(t *testing.T) {
	assert.Equal(t, 2, run([]string{"check-release-gates", "--bogus"}))
}

func TestRunCheckReleaseGatesHelpExitsZero(t *testing.T) {
	assert.Equal(t, 0, run([]string{"check-release-gates", "--help"}))
}
