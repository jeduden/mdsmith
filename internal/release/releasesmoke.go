package release

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// smokeJobName is the release.yml job that installs the just-published
// version from each public channel and asserts `mdsmith version`
// reports the tag.
const smokeJobName = "smoke-test"

// RequiredSmokeChannels are the install channels that must each have a
// `channel:` entry in release.yml's smoke-test matrix. Every channel
// here is consumable the moment the release workflow finishes — no
// third-party registry follow-up — so a missing entry means a channel
// users can hit on day one ships unverified. `go` earned its place the
// hard way: v0.40.0's go.mod carried a replace directive, which is
// fatal only on the `go install m@version` path, and no pre-release
// job exercises that path (CI's `go install ./cmd/mdsmith` is a
// directory install, which honors replace directives).
var RequiredSmokeChannels = []string{"go", "mise", "npm", "pip"}

type smokeRawJob struct {
	Strategy struct {
		Matrix struct {
			Include []struct {
				Channel string `yaml:"channel"`
			} `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
}

type smokeRawWorkflow struct {
	Jobs map[string]smokeRawJob `yaml:"jobs"`
}

// CheckReleaseSmoke enforces post-publication install coverage on the
// release workflow YAML: the smoke-test job must exist and its matrix
// must include one entry per RequiredSmokeChannels channel. The gate
// keeps the matrix in step with the channels users reach directly, so
// an install path cannot silently drop out of release verification.
func CheckReleaseSmoke(workflowYAML []byte) ([]GateViolation, error) {
	var wf smokeRawWorkflow
	if err := yaml.Unmarshal(workflowYAML, &wf); err != nil {
		return nil, fmt.Errorf("parsing release workflow YAML: %w", err)
	}
	job, ok := wf.Jobs[smokeJobName]
	if !ok {
		return []GateViolation{{
			Job: smokeJobName,
			Reason: "missing: the post-publication install smoke job must " +
				"exist so a broken channel fails the release, not a user",
		}}, nil
	}
	have := make(map[string]bool, len(job.Strategy.Matrix.Include))
	for _, entry := range job.Strategy.Matrix.Include {
		have[entry.Channel] = true
	}
	var violations []GateViolation
	for _, channel := range RequiredSmokeChannels {
		if have[channel] {
			continue
		}
		violations = append(violations, GateViolation{
			Job: smokeJobName,
			Reason: fmt.Sprintf(
				"matrix has no entry for channel %q; every directly consumable "+
					"channel must be installed and version-checked after publication",
				channel),
		})
	}
	return violations, nil
}

// CheckReleaseSmokeRoot applies CheckReleaseSmoke to release.yml under
// root, tagging violations with the workflow file name.
func CheckReleaseSmokeRoot(root string) ([]GateViolation, error) {
	path := filepath.Join(root, ReleaseWorkflowPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ReleaseWorkflowPath, err)
	}
	violations, err := CheckReleaseSmoke(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ReleaseWorkflowPath, err)
	}
	for i := range violations {
		violations[i].Workflow = filepath.Base(ReleaseWorkflowPath)
	}
	return violations, nil
}
