package release

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// ReleaseWorkflowPath is the workflow that owns the gated release
	// pipeline, relative to the repo root. CheckReleaseGatesRoot
	// applies the full invariant to it and confines the two release
	// environments to it.
	ReleaseWorkflowPath = ".github/workflows/release.yml"

	// workflowsDirName is the directory CheckReleaseGatesRoot scans;
	// every *.yml / *.yaml file in it is checked.
	workflowsDirName = ".github/workflows"

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

// bypassIfPattern matches the `if:` expressions that make a job run
// even when a needed job failed or was cancelled. Any status-check
// function other than success() replaces the implicit success()
// condition: always() and !cancelled() run regardless, and failure()
// or cancelled() run exactly when a needed job failed or was
// cancelled. With the reviewer on the gate job rather than the
// `release` environment, any of these on a credential job would run
// it — and hand it the secrets — after a failed or rejected gate.
var bypassIfPattern = regexp.MustCompile(`(always|failure|cancelled)\s*\(`)

// GateViolation is one job that breaks the secret-gating invariant,
// with a human-readable reason. Workflow is the file the job lives in;
// empty when the caller checked a single document directly.
type GateViolation struct {
	Workflow string
	Job      string
	Reason   string
}

func (v GateViolation) String() string {
	if v.Workflow == "" {
		return fmt.Sprintf("%s: %s", v.Job, v.Reason)
	}
	return fmt.Sprintf("%s: %s: %s", v.Workflow, v.Job, v.Reason)
}

type gateRawJob struct {
	Environment yaml.Node `yaml:"environment"`
	Needs       yaml.Node `yaml:"needs"`
	If          yaml.Node `yaml:"if"`
}

type gateRawWorkflow struct {
	Jobs map[string]gateRawJob `yaml:"jobs"`
}

// CheckReleaseGates enforces the secret-gating invariant on the
// release workflow YAML. Every job that declares
// `environment: release` — and so can read that environment's
// publisher secrets or mint an OIDC token scoped to it — must list the
// `gate` job in `needs:`, must not carry an `if:` that survives a
// failed gate (any status-check function other than success()), and
// `gate` itself must be the only job on the reviewer-gated
// `release-approval` environment.
// Environment names compare case-insensitively because GitHub treats
// them that way, and expression-valued environments are rejected
// outright: the guard can only verify literal names.
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
	} else if env := environmentName(gate.Environment); !strings.EqualFold(env, approvalEnvName) {
		violations = append(violations, GateViolation{
			Job: gateJobName,
			Reason: fmt.Sprintf(
				"must declare environment: %s (the reviewer-gated env), got %q",
				approvalEnvName, env),
		})
	}

	for _, name := range slices.Sorted(maps.Keys(wf.Jobs)) {
		if name == gateJobName {
			continue
		}
		violations = append(violations, checkReleaseJob(name, wf.Jobs[name])...)
	}
	return violations, nil
}

// checkReleaseJob applies the per-job rules of CheckReleaseGates to
// one non-gate job in release.yml.
func checkReleaseJob(name string, job gateRawJob) []GateViolation {
	env := environmentName(job.Environment)
	if strings.Contains(env, "${{") {
		return []GateViolation{{
			Job: name,
			Reason: fmt.Sprintf(
				"environment is an expression (%q); the guard can only "+
					"verify literal environment names", env),
		}}
	}
	if strings.EqualFold(env, approvalEnvName) {
		return []GateViolation{{
			Job: name,
			Reason: fmt.Sprintf(
				"declares environment: %s, but only the %q job may use the "+
					"approval environment — a second user adds a second "+
					"approval prompt", approvalEnvName, gateJobName),
		}}
	}
	if !strings.EqualFold(env, secretEnvName) {
		return nil
	}
	var violations []GateViolation
	if !slices.Contains(needsList(job.Needs), gateJobName) {
		violations = append(violations, GateViolation{
			Job: name,
			Reason: fmt.Sprintf(
				"declares environment: %s but does not list %q in needs: — "+
					"it could read that environment's secrets without approval",
				secretEnvName, gateJobName),
		})
	}
	if bypassIfPattern.MatchString(ifCondition(job.If)) {
		violations = append(violations, GateViolation{
			Job: name,
			Reason: "its if: uses a status-check function other than success() " +
				"(always/failure/cancelled), which would run the job — with the " +
				"environment's secrets — even after a failed or rejected gate",
		})
	}
	return violations
}

// checkWorkflowEnvIsolation checks a workflow OTHER than release.yml:
// no job in it may target the `release` or `release-approval`
// environments. The gate chokepoint only exists inside release.yml, so
// a release-environment job anywhere else would reach the publisher
// secrets with no approval path at all.
func checkWorkflowEnvIsolation(workflowYAML []byte) ([]GateViolation, error) {
	var wf gateRawWorkflow
	if err := yaml.Unmarshal(workflowYAML, &wf); err != nil {
		return nil, err
	}
	var violations []GateViolation
	for _, name := range slices.Sorted(maps.Keys(wf.Jobs)) {
		env := environmentName(wf.Jobs[name].Environment)
		switch {
		case strings.Contains(env, "${{"):
			// GitHub resolves the expression at run time, so it could
			// evaluate to "release"; the guard cannot verify it
			// statically and rejects it like CheckReleaseGates does.
			violations = append(violations, GateViolation{
				Job: name,
				Reason: fmt.Sprintf(
					"environment is an expression (%q); the guard can only "+
						"verify literal environment names", env),
			})
		case strings.EqualFold(env, secretEnvName):
			violations = append(violations, GateViolation{
				Job: name,
				Reason: fmt.Sprintf(
					"declares environment: %s outside %s — the environment's "+
						"secrets must only be reachable through the gated "+
						"release pipeline", secretEnvName, ReleaseWorkflowPath),
			})
		case strings.EqualFold(env, approvalEnvName):
			violations = append(violations, GateViolation{
				Job: name,
				Reason: fmt.Sprintf(
					"declares environment: %s outside %s — the approval "+
						"environment is reserved for the release pipeline's "+
						"%q job", approvalEnvName, ReleaseWorkflowPath, gateJobName),
			})
		}
	}
	return violations, nil
}

// CheckReleaseGatesRoot scans every workflow under .github/workflows
// beneath root: release.yml gets the full CheckReleaseGates invariant,
// and every other workflow must stay clear of the two release
// environments (checkWorkflowEnvIsolation). Checking the whole
// directory — not just release.yml — is what stops a new or edited
// sibling workflow from reaching the `release` secrets ungated.
func CheckReleaseGatesRoot(root string) ([]GateViolation, error) {
	dir := filepath.Join(root, workflowsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", workflowsDirName, err)
	}
	var violations []GateViolation
	sawRelease := false
	releaseBase := filepath.Base(ReleaseWorkflowPath)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() ||
			(!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reading %s/%s: %w", workflowsDirName, name, err)
		}
		var found []GateViolation
		if name == releaseBase {
			sawRelease = true
			found, err = CheckReleaseGates(data)
		} else {
			found, err = checkWorkflowEnvIsolation(data)
		}
		if err != nil {
			return nil, fmt.Errorf("parsing %s/%s: %w", workflowsDirName, name, err)
		}
		for i := range found {
			found[i].Workflow = name
		}
		violations = append(violations, found...)
	}
	if !sawRelease {
		return nil, fmt.Errorf("missing %s", ReleaseWorkflowPath)
	}
	return violations, nil
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
// (`needs: build`) or a sequence (`needs: [build, gate]`), possibly
// shared between jobs through a YAML anchor — follow the alias so a
// shared needs list is not misread as empty.
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
	case yaml.AliasNode:
		if n.Alias != nil {
			return needsList(*n.Alias)
		}
	}
	return nil
}

// ifCondition returns a job's `if:` expression. release.yml shares one
// repository-guard condition through a YAML anchor, and yaml.v3 hands
// the alias node through unresolved when decoding into a yaml.Node —
// follow it so an aliased always() cannot hide from bypassIfPattern.
func ifCondition(n yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value
	case yaml.AliasNode:
		if n.Alias != nil {
			return n.Alias.Value
		}
	}
	return ""
}
