package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeJSON writes body to a temp findings.json and returns its path.
func writeJSON(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "findings.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

const criticalFinding = `{
  "target": {"mode": "pr", "repo": "jeduden/mdsmith", "ref": "abc"},
  "findings": [{
    "id": "S001", "title": "rce", "severity": "critical", "confidence": "confirmed",
    "location": {"file": "internal/rules/build/rule.go", "startLine": 270},
    "remediation": "revert"
  }]
}`

const infoFinding = `{
  "target": {"mode": "pr", "repo": "r"},
  "findings": [{"id": "S001", "title": "note", "severity": "info",
    "location": {"file": "internal/rules/build/rule.go", "startLine": 270}}]
}`

// runCLI invokes run with args and returns the exit code plus captured
// stdout/stderr.
func runCLI(args ...string) (int, string, string) {
	var out, errBuf bytes.Buffer
	code := run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestRunNoArgs(t *testing.T) {
	code, _, errOut := runCLI()
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "Usage")
}

func TestRunHelp(t *testing.T) {
	code, out, _ := runCLI("--help")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "Commands:")
}

func TestRunUnknownCommand(t *testing.T) {
	code, _, errOut := runCLI("frobnicate")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "unknown command")
}

func TestRenderHappy(t *testing.T) {
	path := writeJSON(t, criticalFinding)
	outDir := t.TempDir()
	code, out, errOut := runCLI("render", path, "--out-dir", outDir)
	require.Equalf(t, 0, code, "stderr: %s", errOut)
	assert.Contains(t, out, "rendered 1 finding(s) ->")
	assert.Contains(t, out, "findings.sarif")
	for _, name := range []string{"findings.sarif", "security-review.md", "inline-annotations.json"} {
		_, err := os.Stat(filepath.Join(outDir, name))
		require.NoErrorf(t, err, "expected %s", name)
	}
}

func TestRenderMissingArg(t *testing.T) {
	code, _, errOut := runCLI("render")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "Usage")
}

func TestRenderBadFindings(t *testing.T) {
	path := writeJSON(t, "{not json")
	code, _, errOut := runCLI("render", path)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "not valid JSON")
}

func TestRenderUnknownFlag(t *testing.T) {
	code, _, errOut := runCLI("render", "--nope")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "render")
}

func TestRenderWriteError(t *testing.T) {
	// out-dir under a regular file: Render's MkdirAll fails, exercising
	// runRender's render-error branch (exit 1).
	path := writeJSON(t, criticalFinding)
	file := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	code, _, errOut := runCLI("render", path, "--out-dir", filepath.Join(file, "sub"))
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "mdsmith-secreview:")
}

func TestRenderHelp(t *testing.T) {
	code, _, _ := runCLI("render", "--help")
	assert.Equal(t, 0, code)
}

func TestGradeHelp(t *testing.T) {
	code, _, _ := runCLI("grade", "--help")
	assert.Equal(t, 0, code)
}

func TestGradeUnknownFlag(t *testing.T) {
	code, _, errOut := runCLI("grade", "--bogus")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "grade")
}

func TestGradeFlagsPass(t *testing.T) {
	path := writeJSON(t, criticalFinding)
	code, out, errOut := runCLI("grade", "--findings", path,
		"--require-min-severity", "high", "--require-location-file", "internal/rules/build/rule.go")
	require.Equalf(t, 0, code, "stderr: %s", errOut)
	assert.Contains(t, out, "PASS")
}

func TestGradeFlagsForbidFail(t *testing.T) {
	path := writeJSON(t, criticalFinding)
	code, _, errOut := runCLI("grade", "--findings", path, "--forbid-severity", "critical")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "FAIL")
	assert.Contains(t, errOut, "forbidden severity")
}

func TestGradeFlagsRequireFail(t *testing.T) {
	path := writeJSON(t, infoFinding)
	code, _, errOut := runCLI("grade", "--findings", path,
		"--require-min-severity", "high", "--require-location-file", "internal/rules/build/rule.go")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "FAIL")
}

func TestGradeMissingFindings(t *testing.T) {
	code, _, errOut := runCLI("grade", "--forbid-severity", "critical")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "requires --findings")
}

func TestGradeVacuousRubric(t *testing.T) {
	path := writeJSON(t, criticalFinding)
	code, _, errOut := runCLI("grade", "--findings", path)
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "vacuous")
}

func TestGradeBadFindingsIsInputError(t *testing.T) {
	path := writeJSON(t, "{bad")
	code, _, errOut := runCLI("grade", "--findings", path, "--forbid-severity", "critical")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "not valid JSON")
}

func TestGradeCaseRequiresCases(t *testing.T) {
	path := writeJSON(t, criticalFinding)
	code, _, errOut := runCLI("grade", "--findings", path, "--case", "some-case")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "--case requires --cases")
}

func TestGradeViaCasesFile(t *testing.T) {
	casesPath := filepath.Join(t.TempDir(), "cases.yaml")
	require.NoError(t, os.WriteFile(casesPath, []byte(`baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: reg
    mode: pr
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade:
      require_finding:
        min_severity: high
        location_file: internal/rules/build/rule.go
`), 0o644))
	path := writeJSON(t, criticalFinding)
	code, out, errOut := runCLI("grade", "--findings", path, "--cases", casesPath, "--case", "reg")
	require.Equalf(t, 0, code, "stderr: %s", errOut)
	assert.Contains(t, out, "PASS reg")
}

func TestGradeViaCasesMissingCase(t *testing.T) {
	casesPath := filepath.Join(t.TempDir(), "cases.yaml")
	require.NoError(t, os.WriteFile(casesPath, []byte(`baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: reg
    mode: pr
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade: {forbid_severities: [high]}
`), 0o644))
	path := writeJSON(t, criticalFinding)
	code, _, errOut := runCLI("grade", "--findings", path, "--cases", casesPath, "--case", "absent")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "not found")
}

func TestGradeViaCasesBadFile(t *testing.T) {
	path := writeJSON(t, criticalFinding)
	code, _, errOut := runCLI("grade", "--findings", path,
		"--cases", filepath.Join(t.TempDir(), "nope.yaml"), "--case", "reg")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "cannot read")
}

func TestGradeExtraPositional(t *testing.T) {
	// grade takes no positionals; a stray arg (e.g. a fat-fingered path)
	// is an input error, not silently ignored — symmetric with render.
	path := writeJSON(t, criticalFinding)
	code, _, errOut := runCLI("grade", "--findings", path, "--forbid-severity", "critical", "stray")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "positional")
}

func TestGradeViaCasesInvalidSpec(t *testing.T) {
	// A structurally invalid cases.yaml (duplicate ids) is rejected even
	// when the named case parses — the CLI validates the whole spec, so a
	// duplicate id can't silently resolve to the wrong rubric.
	casesPath := filepath.Join(t.TempDir(), "cases.yaml")
	require.NoError(t, os.WriteFile(casesPath, []byte(`baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: dup
    mode: pr
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade: {forbid_severities: [high]}
  - id: dup
    mode: pr
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade: {forbid_severities: [high]}
`), 0o644))
	path := writeJSON(t, criticalFinding)
	code, _, errOut := runCLI("grade", "--findings", path, "--cases", casesPath, "--case", "dup")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "duplicate case id")
}
