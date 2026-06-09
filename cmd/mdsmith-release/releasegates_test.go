package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gatesWriteWorkflow(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
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
	root := t.TempDir()
	chdirTo(t, root)
	gatesWriteWorkflow(t, root, "release.yml", gatesGoodWorkflow)
	assert.Equal(t, 0, run([]string{"check-release-gates"}))
}

func TestRunCheckReleaseGatesFailsOnUngatedJob(t *testing.T) {
	root := t.TempDir()
	chdirTo(t, root)
	gatesWriteWorkflow(t, root, "release.yml", gatesUngatedWorkflow)
	assert.Equal(t, 1, run([]string{"check-release-gates"}))
}

func TestRunCheckReleaseGatesFailsOnForeignReleaseEnvJob(t *testing.T) {
	// A sibling workflow targeting the release environment must fail
	// the guard even though release.yml itself is correctly wired.
	root := t.TempDir()
	chdirTo(t, root)
	gatesWriteWorkflow(t, root, "release.yml", gatesGoodWorkflow)
	gatesWriteWorkflow(t, root, "sneaky.yml",
		"jobs:\n  exfil:\n    environment: release\n    runs-on: ubuntu-latest\n")
	assert.Equal(t, 1, run([]string{"check-release-gates"}))
}

func TestRunCheckReleaseGatesFailsWhenWorkflowMissing(t *testing.T) {
	chdirTo(t, t.TempDir()) // no .github/workflows tree at all
	assert.Equal(t, 1, run([]string{"check-release-gates"}))
}
