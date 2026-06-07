package release

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateWorkflow is a minimal but structurally faithful release
// workflow: a gate job on the reviewer-gated environment plus one
// credential job that lists gate in needs. Tests mutate a copy of it.
const gateWorkflow = `
jobs:
  build:
    runs-on: ubuntu-latest
  gate:
    needs: [build]
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    needs: [build, gate]
    environment: release
    runs-on: ubuntu-latest
`

func TestCheckReleaseGatesAcceptsWiredWorkflow(t *testing.T) {
	got, err := CheckReleaseGates([]byte(gateWorkflow))
	require.NoError(t, err)
	assert.Empty(t, got, "a correctly wired workflow has no violations")
}

func TestCheckReleaseGatesFlagsUngatedSecretJob(t *testing.T) {
	// The npm job reaches the `release` environment's secrets but does
	// not depend on gate — the exact hole the guard exists to catch.
	const wf = `
jobs:
  build:
    runs-on: ubuntu-latest
  gate:
    needs: [build]
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    needs: [build]
    environment: release
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "npm", got[0].Job)
	assert.Contains(t, got[0].Reason, "without approval")
}

func TestCheckReleaseGatesFlagsScalarNeedsWithoutGate(t *testing.T) {
	// `needs:` in the scalar form must be handled too.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  release:
    needs: build
    environment: release
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "release", got[0].Job)
}

func TestCheckReleaseGatesAcceptsMappingEnvironmentForm(t *testing.T) {
	// `environment: { name: release }` is equivalent to the scalar
	// form and must be recognized as reaching the secrets.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  pypi:
    needs: [gate]
    environment:
      name: release
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCheckReleaseGatesRequiresGateJob(t *testing.T) {
	const wf = `
jobs:
  npm:
    needs: [build]
    environment: release
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	// Both the missing gate job and the now-unanchored npm job are
	// reported.
	var sawGate, sawNpm bool
	for _, v := range got {
		switch v.Job {
		case "gate":
			sawGate = true
			assert.Contains(t, v.Reason, "missing")
		case "npm":
			sawNpm = true
		}
	}
	assert.True(t, sawGate, "missing gate job must be reported: %v", got)
	assert.True(t, sawNpm, "ungated npm job must still be reported: %v", got)
}

func TestCheckReleaseGatesRejectsGateOnWrongEnvironment(t *testing.T) {
	// If gate sits on the secret environment instead of the
	// reviewer-gated one, the single-approval model is broken.
	const wf = `
jobs:
  gate:
    needs: [build]
    environment: release
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "gate", got[0].Job)
	assert.Contains(t, got[0].Reason, "release-approval")
}

func TestCheckReleaseGatesRejectsEmptyWorkflow(t *testing.T) {
	_, err := CheckReleaseGates([]byte("name: Release\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no jobs")
}

func TestCheckReleaseGatesRejectsInvalidYAML(t *testing.T) {
	_, err := CheckReleaseGates([]byte("jobs: : :\n"))
	require.Error(t, err)
}

// TestRealReleaseWorkflowSatisfiesGates runs the guard over the actual
// committed .github/workflows/release.yml so `go test ./...` fails the
// moment an edit introduces an ungated `environment: release` job —
// the same property the CI guard job enforces, also caught locally.
func TestRealReleaseWorkflowSatisfiesGates(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	got, err := CheckReleaseGatesFile(root)
	require.NoError(t, err)
	assert.Empty(t, got,
		"release.yml has an environment: release job missing needs:[gate]: %v", got)
}

func TestGateViolationString(t *testing.T) {
	v := GateViolation{Job: "npm", Reason: "missing gate"}
	assert.Equal(t, "npm: missing gate", v.String())
}

func TestCheckReleaseGatesFileReportsMissingWorkflow(t *testing.T) {
	_, err := CheckReleaseGatesFile(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ReleaseWorkflowPath)
}

func TestCheckReleaseGatesFlagsJobWithNoNeeds(t *testing.T) {
	// A release-env job with no `needs:` at all — exercises the
	// empty-needs path and is still flagged for not depending on gate.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    environment: release
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "npm", got[0].Job)
}

func TestCheckReleaseGatesIgnoresEnvironmentMappingWithoutName(t *testing.T) {
	// A mapping environment with no `name` resolves to "" and is not
	// treated as the secret env, so it raises no violation.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  weird:
    needs: [build]
    environment:
      url: https://example.com
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	assert.Empty(t, got)
}
