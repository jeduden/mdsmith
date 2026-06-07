package release

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	// ReleaseWorkflowPath is the workflow whose secret-gating
	// invariant CheckReleaseGates enforces, relative to the repo root.
	ReleaseWorkflowPath = ".github/workflows/release.yml"

	// gateJobName is the single approval chokepoint every job that can
	// reach a release-environment secret must depend on.
	gateJobName = "gate"
	// secretEnvName is the environment that holds the publisher
	// secrets and the OIDC publishing scope.
	secretEnvName = "release"
	// approvalEnvName is the single-use environment that carries the
	// lone required reviewer; only the gate job uses it.
	approvalEnvName = "release-approval"
)

// GateViolation is one job that breaks the secret-gating invariant,
// with a human-readable reason.
type GateViolation struct {
	Job    string
	Reason string
}

func (v GateViolation) String() string {
	return fmt.Sprintf("%s: %s", v.Job, v.Reason)
}

type gateRawJob struct {
	Environment yaml.Node `yaml:"environment"`
	Needs       yaml.Node `yaml:"needs"`
}

type gateRawWorkflow struct {
	Jobs map[string]gateRawJob `yaml:"jobs"`
}

// CheckReleaseGates enforces the secret-gating invariant on the
// release workflow YAML. Every job that declares
// `environment: release` — and so can read that environment's
// publisher secrets or mint an OIDC token scoped to it — must list the
// `gate` job in `needs:`, and `gate` itself must be the single
// reviewer chokepoint on the `release-approval` environment.
//
// The `release` environment carries no required reviewer (so one
// approval covers the whole run); the `needs: gate` edge is therefore
// the only thing that keeps a credential job from starting — and
// reading a secret — before the approval lands. Enforcing it here, in
// CI, is what makes that gating a checked guarantee rather than a
// convention a future edit could silently drop.
func CheckReleaseGates(workflowYAML []byte) ([]GateViolation, error) {
	var wf gateRawWorkflow
	if err := yaml.Unmarshal(workflowYAML, &wf); err != nil {
		return nil, fmt.Errorf("parsing release workflow YAML: %w", err)
	}
	if len(wf.Jobs) == 0 {
		return nil, fmt.Errorf("release workflow declares no jobs")
	}

	var violations []GateViolation

	// The gate job must exist and sit on the reviewer-gated
	// environment. Without that, the needs:[gate] edges below point at
	// a job that gates nothing and the single-approval model is gone.
	gate, ok := wf.Jobs[gateJobName]
	if !ok {
		violations = append(violations, GateViolation{
			Job:    gateJobName,
			Reason: "missing: the single approval chokepoint must exist",
		})
	} else if env := environmentName(gate.Environment); env != approvalEnvName {
		violations = append(violations, GateViolation{
			Job: gateJobName,
			Reason: fmt.Sprintf(
				"must declare environment: %s (the reviewer-gated env), got %q",
				approvalEnvName, env),
		})
	}

	for _, name := range sortedJobNames(wf.Jobs) {
		if name == gateJobName {
			continue
		}
		if environmentName(wf.Jobs[name].Environment) != secretEnvName {
			continue
		}
		if !slices.Contains(needsList(wf.Jobs[name].Needs), gateJobName) {
			violations = append(violations, GateViolation{
				Job: name,
				Reason: fmt.Sprintf(
					"declares environment: %s but does not list %q in needs: — "+
						"it could read that environment's secrets without approval",
					secretEnvName, gateJobName),
			})
		}
	}
	return violations, nil
}

// CheckReleaseGatesFile reads the release workflow under root and runs
// CheckReleaseGates over it.
func CheckReleaseGatesFile(root string) ([]GateViolation, error) {
	path := filepath.Join(root, ReleaseWorkflowPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ReleaseWorkflowPath, err)
	}
	return CheckReleaseGates(data)
}

// environmentName returns the environment a job targets. GitHub allows
// both the scalar form (`environment: release`) and the mapping form
// (`environment: { name: release, url: ... }`); both resolve to the
// same name. A job with no environment returns "".
func environmentName(n yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "name" {
				return n.Content[i+1].Value
			}
		}
	}
	return ""
}

// needsList returns a job's dependencies. `needs:` is either a scalar
// (`needs: build`) or a sequence (`needs: [build, gate]`).
func needsList(n yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, c.Value)
		}
		return out
	}
	return nil
}

func sortedJobNames(jobs map[string]gateRawJob) []string {
	names := make([]string, 0, len(jobs))
	for name := range jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
