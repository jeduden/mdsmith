package release

import (
	"os"
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

func TestCheckReleaseGatesMatchesEnvironmentCaseInsensitively(t *testing.T) {
	// GitHub environment names are case-insensitive: `Release` reaches
	// the same secrets as `release`, so the guard must flag it too.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  sneaky:
    needs: [build]
    environment: Release
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "sneaky", got[0].Job)
}

func TestCheckReleaseGatesAcceptsGateEnvironmentCaseVariant(t *testing.T) {
	// The same case-insensitivity applies to the approval environment
	// on the gate job itself.
	const wf = `
jobs:
  gate:
    environment: Release-Approval
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCheckReleaseGatesRejectsExpressionEnvironment(t *testing.T) {
	// An expression-valued environment cannot be verified statically;
	// GitHub would evaluate it at run time, so the guard rejects it.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  dynamic:
    needs: [gate]
    environment: ${{ inputs.env }}
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "dynamic", got[0].Job)
	assert.Contains(t, got[0].Reason, "expression")
}

func TestCheckReleaseGatesFlagsSecondApprovalEnvironmentUser(t *testing.T) {
	// A second job on release-approval would add a second reviewer
	// prompt, defeating the single-approval design.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  extra-gate:
    needs: [gate]
    environment: release-approval
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "extra-gate", got[0].Job)
	assert.Contains(t, got[0].Reason, "second approval prompt")
}

func TestCheckReleaseGatesFlagsApprovalBypassingIf(t *testing.T) {
	// needs:[gate] only blocks a job while the gate result propagates;
	// `if: always()` (or !cancelled()) overrides a failed or rejected
	// gate and would hand the job the secrets anyway.
	cases := map[string]string{
		"always":          "always()",
		"not-cancelled":   "${{ !cancelled() }}",
		"failure":         "failure()",
		"plain-cancelled": "cancelled()",
	}
	for name, cond := range cases {
		t.Run(name, func(t *testing.T) {
			wf := `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    needs: [gate]
    environment: release
    if: ` + cond + `
    runs-on: ubuntu-latest
`
			got, err := CheckReleaseGates([]byte(wf))
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, "npm", got[0].Job)
			assert.Contains(t, got[0].Reason, "rejected gate")
		})
	}
}

func TestCheckReleaseGatesFollowsIfAlias(t *testing.T) {
	// release.yml shares its repository guard through a YAML anchor;
	// an aliased always() must not hide from the bypass check.
	const wf = `
jobs:
  gate:
    if: &cond >-
      always()
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    needs: [gate]
    environment: release
    if: *cond
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseGates([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "npm", got[0].Job)
}

func TestCheckReleaseGatesFollowsNeedsAlias(t *testing.T) {
	// A needs list shared between jobs through a YAML anchor must
	// resolve through the alias, or the aliased job would be misread
	// as gate-less and flagged spuriously.
	const wf = `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    needs: &shared [build, gate]
    environment: release
    runs-on: ubuntu-latest
  pypi:
    needs: *shared
    environment: release
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

func TestGateViolationString(t *testing.T) {
	v := GateViolation{Job: "npm", Reason: "missing gate"}
	assert.Equal(t, "npm: missing gate", v.String())
	v.Workflow = "release.yml"
	assert.Equal(t, "release.yml: npm: missing gate", v.String())
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

// --- directory scan (CheckReleaseGatesRoot) ---

// writeWorkflows lays a synthetic .github/workflows tree under a temp
// root and returns the root.
func writeWorkflows(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return root
}

func TestCheckReleaseGatesRootAcceptsCleanTree(t *testing.T) {
	root := writeWorkflows(t, map[string]string{
		"release.yml": gateWorkflow,
		"ci.yml":      "jobs:\n  test:\n    runs-on: ubuntu-latest\n",
		// Non-workflow files in the directory are skipped.
		"README.md": "not a workflow\n",
	})
	got, err := CheckReleaseGatesRoot(root)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCheckReleaseGatesRootFlagsForeignReleaseEnvJob(t *testing.T) {
	// The whole point of scanning the directory: a sibling workflow
	// must not reach the `release` secrets — there is no gate there.
	root := writeWorkflows(t, map[string]string{
		"release.yml": gateWorkflow,
		"sneaky.yml": `
jobs:
  exfil:
    environment: release
    runs-on: ubuntu-latest
`,
	})
	got, err := CheckReleaseGatesRoot(root)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "sneaky.yml", got[0].Workflow)
	assert.Equal(t, "exfil", got[0].Job)
	assert.Contains(t, got[0].Reason, "outside")
}

func TestCheckReleaseGatesRootFlagsForeignApprovalEnvJob(t *testing.T) {
	root := writeWorkflows(t, map[string]string{
		"release.yml": gateWorkflow,
		"other.yaml": `
jobs:
  second-gate:
    environment: Release-Approval
    runs-on: ubuntu-latest
`,
	})
	got, err := CheckReleaseGatesRoot(root)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "other.yaml", got[0].Workflow)
	assert.Contains(t, got[0].Reason, "reserved")
}

func TestCheckReleaseGatesRootFlagsForeignExpressionEnv(t *testing.T) {
	// An expression-valued environment in a sibling workflow could
	// evaluate to "release" at run time; the guard rejects what it
	// cannot verify statically.
	root := writeWorkflows(t, map[string]string{
		"release.yml": gateWorkflow,
		"dynamic.yml": `
jobs:
  deploy:
    environment: ${{ inputs.env }}
    runs-on: ubuntu-latest
`,
	})
	got, err := CheckReleaseGatesRoot(root)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "dynamic.yml", got[0].Workflow)
	assert.Contains(t, got[0].Reason, "expression")
}

func TestCheckReleaseGatesRootTagsReleaseWorkflowViolations(t *testing.T) {
	root := writeWorkflows(t, map[string]string{
		"release.yml": `
jobs:
  gate:
    environment: release-approval
    runs-on: ubuntu-latest
  npm:
    environment: release
    runs-on: ubuntu-latest
`,
	})
	got, err := CheckReleaseGatesRoot(root)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "release.yml", got[0].Workflow)
	assert.Equal(t, "npm", got[0].Job)
}

func TestCheckReleaseGatesRootRequiresReleaseWorkflow(t *testing.T) {
	root := writeWorkflows(t, map[string]string{
		"ci.yml": "jobs:\n  test:\n    runs-on: ubuntu-latest\n",
	})
	_, err := CheckReleaseGatesRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ReleaseWorkflowPath)
}

func TestCheckReleaseGatesRootReportsMissingWorkflowsDir(t *testing.T) {
	_, err := CheckReleaseGatesRoot(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), workflowsDirName)
}

func TestCheckReleaseGatesRootSurfacesUnreadableWorkflow(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; chmod 0o000 is bypassed")
	}
	root := writeWorkflows(t, map[string]string{
		"release.yml": gateWorkflow,
		"locked.yml":  "jobs: {}\n",
	})
	locked := filepath.Join(root, ".github", "workflows", "locked.yml")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	_, err := CheckReleaseGatesRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked.yml")
}

func TestCheckReleaseGatesRootSurfacesForeignParseError(t *testing.T) {
	root := writeWorkflows(t, map[string]string{
		"release.yml": gateWorkflow,
		"broken.yml":  "jobs: : :\n",
	})
	_, err := CheckReleaseGatesRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken.yml")
}

func TestCheckReleaseGatesRootIgnoresForeignWorkflowWithoutJobs(t *testing.T) {
	// A jobs-less foreign file (e.g. a stub) is fine — only release.yml
	// itself must declare jobs.
	root := writeWorkflows(t, map[string]string{
		"release.yml": gateWorkflow,
		"stub.yml":    "name: stub\n",
	})
	got, err := CheckReleaseGatesRoot(root)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestRealReleaseWorkflowSatisfiesGates runs the guard over the actual
// committed .github/workflows tree so `go test ./...` fails the moment
// an edit introduces an ungated `environment: release` job anywhere —
// the same property the CI guard job enforces, also caught locally.
func TestRealReleaseWorkflowSatisfiesGates(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	got, err := CheckReleaseGatesRoot(root)
	require.NoError(t, err)
	assert.Empty(t, got,
		"a workflow violates the release-gate invariant: %v", got)
}
