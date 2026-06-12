package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const smokeCoveredReleaseWorkflow = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
          - channel: pip
          - channel: mise
          - channel: asdf
          - channel: go
`

const smokeMissingGoWorkflow = `
jobs:
  smoke-test:
    strategy:
      matrix:
        include:
          - channel: npm
          - channel: pip
          - channel: mise
`

func TestRunCheckReleaseSmokeAcceptsCoveredMatrix(t *testing.T) {
	root := t.TempDir()
	chdirTo(t, root)
	gatesWriteWorkflow(t, root, "release.yml", smokeCoveredReleaseWorkflow)
	assert.Equal(t, 0, run([]string{"check-release-smoke"}))
}

func TestRunCheckReleaseSmokeFailsOnMissingChannel(t *testing.T) {
	root := t.TempDir()
	chdirTo(t, root)
	gatesWriteWorkflow(t, root, "release.yml", smokeMissingGoWorkflow)
	assert.Equal(t, 1, run([]string{"check-release-smoke"}))
}

func TestRunCheckReleaseSmokeFailsOnRequiredChannelSoftSkip(t *testing.T) {
	// All required channels are present, but one of them carries the
	// best-effort skipped=true output — the second violation class.
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
              echo "skipped=true" >> "$GITHUB_OUTPUT"
          - channel: asdf
          - channel: go
`
	root := t.TempDir()
	chdirTo(t, root)
	gatesWriteWorkflow(t, root, "release.yml", wf)
	assert.Equal(t, 1, run([]string{"check-release-smoke"}))
}

func TestRunCheckReleaseSmokeFailsWithoutWorkflow(t *testing.T) {
	root := t.TempDir()
	chdirTo(t, root)
	assert.Equal(t, 1, run([]string{"check-release-smoke"}))
}
