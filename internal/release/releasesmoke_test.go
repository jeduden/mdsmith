package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const smokeCoveredWorkflow = `
jobs:
  smoke-test:
    needs: [npm, pypi, release]
    strategy:
      matrix:
        include:
          - channel: npm
            container: node:lts
          - channel: pip
            container: python:3.12-slim
          - channel: mise
            container: jdxcode/mise:latest
          - channel: asdf
            container: ubuntu:latest
          - channel: go
            container: golang:1.25
`

func TestCheckReleaseSmokeAcceptsCoveredMatrix(t *testing.T) {
	got, err := CheckReleaseSmoke([]byte(smokeCoveredWorkflow))
	require.NoError(t, err)
	assert.Empty(t, got, "a matrix covering every required channel has no violations")
}

func TestCheckReleaseSmokeFlagsMissingGoChannel(t *testing.T) {
	// The exact v0.40.0 hole: npm, pip, and mise are smoked after
	// publication but `go install m@version` is not, so a go.mod
	// replace directive shipped fatal on the one path nothing tested.
	const wf = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
          - channel: pip
          - channel: mise
          - channel: asdf
`
	got, err := CheckReleaseSmoke([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, smokeJobName, got[0].Job)
	assert.Contains(t, got[0].Reason, `"go"`)
}

func TestCheckReleaseSmokeFlagsMissingAsdfChannel(t *testing.T) {
	// asdf is consumable on day one via the explicit plugin URL
	// (asdf plugin add mdsmith https://github.com/jeduden/asdf-mdsmith.git),
	// so a release that breaks the asdf install path must fail its own
	// pipeline rather than ship unverified.
	const wf = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
          - channel: pip
          - channel: mise
          - channel: go
`
	got, err := CheckReleaseSmoke([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, smokeJobName, got[0].Job)
	assert.Contains(t, got[0].Reason, `"asdf"`)
}

func TestCheckReleaseSmokeFlagsEveryMissingChannel(t *testing.T) {
	const wf = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
`
	got, err := CheckReleaseSmoke([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, len(RequiredSmokeChannels)-1)
}

func TestCheckReleaseSmokeFlagsRequiredChannelThatSoftSkips(t *testing.T) {
	// The skipped=true output is the best-effort contract: an install
	// script writes it and exits 0, and the shared Verify step skips.
	// A REQUIRED channel carrying that marker would pass the matrix
	// coverage check while never actually verifying the binary, so the
	// gate must reject the combination loudly.
	const wf = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
          - channel: pip
          - channel: mise
            install: |
              if ! mise use -g "mdsmith@1.0.0"; then
                echo "skipped=true" >> "$GITHUB_OUTPUT"
                exit 0
              fi
          - channel: asdf
          - channel: go
`
	got, err := CheckReleaseSmoke([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, smokeJobName, got[0].Job)
	assert.Contains(t, got[0].Reason, `"mise"`)
	assert.Contains(t, got[0].Reason, "skipped=true")
}

func TestCheckReleaseSmokeAcceptsBestEffortSoftSkip(t *testing.T) {
	// A channel outside RequiredSmokeChannels (mise-registry while the
	// jdx/mise registry entry is pending) may soft-skip: that is the
	// designed best-effort path, not a coverage hole.
	const wf = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
          - channel: pip
          - channel: mise
          - channel: mise-registry
            install: |
              if ! mise use -g "mdsmith@1.0.0"; then
                echo "skipped=true" >> "$GITHUB_OUTPUT"
                exit 0
              fi
          - channel: asdf
          - channel: go
`
	got, err := CheckReleaseSmoke([]byte(wf))
	require.NoError(t, err)
	assert.Empty(t, got, "a best-effort channel outside RequiredSmokeChannels may soft-skip")
}

func TestCheckReleaseSmokeFlagsMissingJob(t *testing.T) {
	const wf = `
jobs:
  build:
    runs-on: ubuntu-latest
`
	got, err := CheckReleaseSmoke([]byte(wf))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, smokeJobName, got[0].Job)
	assert.Contains(t, got[0].Reason, "missing")
}

func TestCheckReleaseSmokeRejectsMalformedYAML(t *testing.T) {
	_, err := CheckReleaseSmoke([]byte("jobs: ["))
	assert.Error(t, err)
}

func TestCheckReleaseSmokeRootReadsReleaseWorkflow(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "release.yml"), []byte(smokeCoveredWorkflow), 0o644))

	got, err := CheckReleaseSmokeRoot(root)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCheckReleaseSmokeRootErrorsWithoutWorkflow(t *testing.T) {
	_, err := CheckReleaseSmokeRoot(t.TempDir())
	assert.Error(t, err)
}

func TestCheckReleaseSmokeRootErrorsOnMalformedWorkflow(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "release.yml"), []byte("jobs: ["), 0o644))

	_, err := CheckReleaseSmokeRoot(root)
	assert.ErrorContains(t, err, "parsing")
}

func TestCheckReleaseSmokeRootTagsViolationsWithWorkflow(t *testing.T) {
	const wf = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
          - channel: pip
          - channel: mise
          - channel: asdf
`
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "release.yml"), []byte(wf), 0o644))

	got, err := CheckReleaseSmokeRoot(root)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "release.yml", got[0].Workflow)
}

// TestRepoReleaseWorkflowCoversRequiredSmokeChannels pins the real
// release.yml: every channel in RequiredSmokeChannels has a
// post-publication smoke entry, so a release that breaks one of them
// fails its own pipeline instead of shipping and waiting for a user
// report (as the v0.40.0 `go install` breakage did).
func TestRepoReleaseWorkflowCoversRequiredSmokeChannels(t *testing.T) {
	got, err := CheckReleaseSmokeRoot("../..")
	require.NoError(t, err)
	assert.Empty(t, got)
}
