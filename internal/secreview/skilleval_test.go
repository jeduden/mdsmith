package secreview

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot walks up from this test file until it finds the go.mod that
// declares the module, so the eval test can locate the skill directory
// regardless of the working directory the test runs from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqualf(t, parent, dir, "go.mod not found walking up from %s", file)
		dir = parent
	}
}

// skillDir returns the absolute path to the mdsmith-security-review skill
// directory.
func skillDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), ".claude", "skills", "mdsmith-security-review")
}

// loadSkillSpec loads and returns the skill's real cases.yaml spec.
func loadSkillSpec(t *testing.T) *Spec {
	t.Helper()
	spec, err := LoadSpec(filepath.Join(skillDir(t), "evals", "cases.yaml"))
	require.NoError(t, err)
	return spec
}

// fixturePath returns the golden findings.json path for a case id.
func fixturePath(t *testing.T, id string) string {
	t.Helper()
	return filepath.Join(skillDir(t), "evals", "fixtures", id+".findings.json")
}

func TestSkillCasesValid(t *testing.T) {
	spec := loadSkillSpec(t)
	require.NoError(t, spec.Validate())
	require.NotEmpty(t, spec.Cases)
}

func TestSkillFixturesRenderAndMap(t *testing.T) {
	spec := loadSkillSpec(t)
	for _, c := range spec.Cases {
		t.Run(c.ID, func(t *testing.T) {
			r, err := LoadReport(fixturePath(t, c.ID))
			require.NoError(t, err)

			out := t.TempDir()
			require.NoError(t, Render(r, out))
			for _, name := range RenderFileNames() {
				_, statErr := os.Stat(filepath.Join(out, name))
				require.NoErrorf(t, statErr, "expected %s", name)
			}
			assertSARIFMapping(t, filepath.Join(out, "findings.sarif"), r)
		})
	}
}

// assertSARIFMapping parses a rendered findings.sarif and asserts the
// result count matches the report and that every result's level and its
// rule's security-severity equal the package maps for that finding's
// severity. This makes the severity maps load-bearing.
func assertSARIFMapping(t *testing.T, sarifPath string, r *Report) {
	t.Helper()
	raw, err := os.ReadFile(sarifPath)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))

	run := doc["runs"].([]any)[0].(map[string]any)
	results := run["results"].([]any)
	require.Len(t, results, len(r.Findings))

	ruleSecSev := map[string]string{}
	for _, ru := range run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any) {
		rule := ru.(map[string]any)
		ruleSecSev[rule["id"].(string)] = rule["properties"].(map[string]any)["security-severity"].(string)
	}
	for i, res := range results {
		result := res.(map[string]any)
		sev := r.Findings[i].Severity
		assert.Equalf(t, sarifLevel[sev], result["level"], "level for %s", result["ruleId"])
		assert.Equalf(t, securitySeverity[sev], ruleSecSev[result["ruleId"].(string)],
			"security-severity for %s", result["ruleId"])
	}
}

func TestSkillGradeGoldensAndNegative(t *testing.T) {
	spec := loadSkillSpec(t)
	for _, c := range spec.Cases {
		t.Run(c.ID, func(t *testing.T) {
			con, err := ConstraintsForCase(c)
			require.NoError(t, err)
			r, err := LoadReport(fixturePath(t, c.ID))
			require.NoError(t, err)
			assert.Empty(t, Grade(r.Findings, con), "golden fixture should pass its rubric")
		})
	}

	// The complacent negative fixture (sees the exec sink but rates it
	// info) must FAIL the regression case's rubric — proof the grader has
	// teeth.
	regCase := caseByID(t, spec, "pr-regression-introduces-exec")
	con, err := ConstraintsForCase(regCase)
	require.NoError(t, err)
	bad := filepath.Join(skillDir(t), "evals", "fixtures", "bad-missed-regression.findings.json")
	r, err := LoadReport(bad)
	require.NoError(t, err)
	assert.NotEmpty(t, Grade(r.Findings, con), "negative fixture must fail the regression rubric")
}

// caseByID returns the case with the given id, failing if absent.
func caseByID(t *testing.T, spec *Spec, id string) Case {
	t.Helper()
	for _, c := range spec.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %q not in cases.yaml", id)
	return Case{}
}

// TestSkillRegressionPatchAtBaseline proves the committed regression patch
// still applies and compiles against the pinned baseline commit. Under
// SECREVIEW_STRICT=1 a missing git, an absent baseline, or a build failure
// is fatal; otherwise the test skips with the reason (CI sets the env).
func TestSkillRegressionPatchAtBaseline(t *testing.T) {
	root := repoRoot(t)
	spec := loadSkillSpec(t)
	require.NoError(t, spec.Validate())
	patch := filepath.Join(skillDir(t), "evals", "regressions", "build-exec.patch")
	assertPatchTargetsBuildRule(t, patch)

	strict := os.Getenv("SECREVIEW_STRICT") == "1"
	if _, err := exec.LookPath("git"); err != nil {
		skipOrFatal(t, strict, "git not on PATH")
		return
	}
	if !baselinePresent(root, spec.BaselineRef) {
		skipOrFatal(t, strict, "baseline commit "+spec.BaselineRef+" not present")
		return
	}
	checkPatchAppliesAndBuilds(t, root, spec.BaselineRef, patch)
}

// assertPatchTargetsBuildRule asserts the patch text introduces an
// exec.Command sink in the build rule file.
func assertPatchTargetsBuildRule(t *testing.T, patch string) {
	t.Helper()
	text, err := os.ReadFile(patch)
	require.NoError(t, err)
	assert.Contains(t, string(text), "exec.Command", "patch must introduce an exec sink")
	assert.Contains(t, string(text), "internal/rules/build/rule.go", "patch must target the build rule")
}

// skipOrFatal fails the test under strict mode and skips it otherwise.
func skipOrFatal(t *testing.T, strict bool, reason string) {
	t.Helper()
	if strict {
		t.Fatal(reason)
	}
	t.Skip(reason)
}

// baselinePresent reports whether ref resolves to a commit in root's repo.
func baselinePresent(root, ref string) bool {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--verify", ref+"^{commit}")
	return cmd.Run() == nil
}

// checkPatchAppliesAndBuilds checks out baseline into a throwaway worktree,
// applies the patch, and builds the build rule package — proving the
// regression compiles at the pinned commit.
func checkPatchAppliesAndBuilds(t *testing.T, root, baseline, patch string) {
	t.Helper()
	wt := filepath.Join(t.TempDir(), "baseline-worktree")
	// Clear any worktree admin entry leaked by a prior run whose --force
	// remove failed: the temp dir is already gone, so prune drops the stale
	// registration and avoids an "already registered" failure on add.
	runGit(t, root, "worktree", "prune")
	runGit(t, root, "worktree", "add", "--detach", wt, baseline)
	defer func() { _ = exec.Command("git", "-C", root, "worktree", "remove", "--force", wt).Run() }()

	runGit(t, wt, "apply", "--check", patch)
	runGit(t, wt, "apply", patch)

	build := exec.Command("go", "build", "./internal/rules/build/")
	build.Dir = wt
	out, err := build.CombinedOutput()
	require.NoErrorf(t, err, "go build of the patched build rule failed: %s", out)

	assertGoldenCitesSink(t, wt)
}

// assertGoldenCitesSink cross-checks that the pr-regression golden fixture's
// primary location cites the real exec.Command line in the patched build
// rule, so the exemplar cannot drift to a stale line after a recalibration.
func assertGoldenCitesSink(t *testing.T, worktree string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(worktree, "internal", "rules", "build", "rule.go"))
	require.NoError(t, err)
	sink := 0
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "exec.Command(") {
			sink = i + 1
			break
		}
	}
	require.NotZero(t, sink, "exec.Command not found in the patched build rule")
	golden, err := LoadReport(filepath.Join(skillDir(t), "evals", "fixtures",
		"pr-regression-introduces-exec.findings.json"))
	require.NoError(t, err)
	require.NotNil(t, golden.Findings[0].Location)
	assert.Equalf(t, sink, golden.Findings[0].Location.StartLine,
		"golden fixture should cite the exec.Command line (%d) as its primary location", sink)
}

// runGit runs a git command in dir and fails the test on a non-zero exit,
// surfacing combined output.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// TestSkillEvalWorkflowEnforcesStrict guards the CI config itself.
// TestSkillRegressionPatchAtBaseline only runs (rather than skips) under
// SECREVIEW_STRICT=1 with the baseline commit present, so the skill-eval
// workflow must set that env var and check out full history. This test —
// which runs in the ordinary `go test` job, no strict mode needed — fails
// if either guarantee is removed, so the calibration gate cannot silently
// degrade to a no-op skip.
func TestSkillEvalWorkflowEnforcesStrict(t *testing.T) {
	wf := filepath.Join(repoRoot(t), ".github", "workflows", "skill-eval.yml")
	data, err := os.ReadFile(wf)
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "SECREVIEW_STRICT",
		"skill-eval.yml must run the eval in strict mode")
	assert.Contains(t, body, "fetch-depth: 0",
		"skill-eval.yml must fetch full history so baseline_ref is present")
}
