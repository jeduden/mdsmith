package secreview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSpec writes body to a temp cases.yaml and returns its path.
func writeSpec(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cases.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

const validSpecBody = `baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: forbid-case
    mode: audit
    prompt: "do an audit"
    setup: "clean clone"
    expect:
      must: ["traces the path"]
      must_not: ["invents an exec path"]
    grade:
      forbid_severities: [critical, high]
  - id: require-case
    mode: pr
    prompt: "review the PR"
    setup: "apply the patch"
    expect:
      must: ["flags a critical"]
      must_not: ["passes clean"]
    grade:
      require_finding:
        min_severity: high
        location_file: internal/rules/build/rule.go
grading_note: "calibrated only if both hold"
`

func TestLoadSpecAndValidateOK(t *testing.T) {
	spec, err := LoadSpec(writeSpec(t, validSpecBody))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())
	require.Len(t, spec.Cases, 2)
	assert.Equal(t, "b72285df2f6b422dd2eb31058884757f13acc78c", spec.BaselineRef)
	assert.Equal(t, "forbid-case", spec.Cases[0].ID)
	assert.Equal(t, []string{"critical", "high"}, spec.Cases[0].Grade.ForbidSeverities)
	require.NotNil(t, spec.Cases[1].Grade.RequireFinding)
	assert.Equal(t, "high", spec.Cases[1].Grade.RequireFinding.MinSeverity)
}

func TestLoadSpecRejectsTypoedGradeKey(t *testing.T) {
	// forbid_severity (singular) is a typo for forbid_severities. With
	// KnownFields(true) it is a hard decode error, not a silently ignored
	// key that would leave the rubric vacuous.
	body := `baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: c1
    mode: audit
    prompt: p
    setup: s
    expect:
      must: [a]
      must_not: [b]
    grade:
      forbid_severity: [critical]
`
	_, err := LoadSpec(writeSpec(t, body))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbid_severity")
}

func TestLoadSpecRejectsUnknownTopLevelKey(t *testing.T) {
	body := validSpecBody + "extra_key: nope\n"
	_, err := LoadSpec(writeSpec(t, body))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extra_key")
}

func TestLoadSpecMissingFile(t *testing.T) {
	_, err := LoadSpec(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

func TestValidateBadBaselineRef(t *testing.T) {
	body := `baseline_ref: not-a-sha
cases:
  - id: c1
    mode: audit
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade: {forbid_severities: [high]}
`
	spec, err := LoadSpec(writeSpec(t, body))
	require.NoError(t, err)
	err = spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "baseline_ref")
}

func TestValidateBadMode(t *testing.T) {
	body := `baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: c1
    mode: drive-by
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade: {forbid_severities: [high]}
`
	spec, err := LoadSpec(writeSpec(t, body))
	require.NoError(t, err)
	err = spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad mode")
}

func TestValidateVacuousGrade(t *testing.T) {
	body := `baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: c1
    mode: audit
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade: {}
`
	spec, err := LoadSpec(writeSpec(t, body))
	require.NoError(t, err)
	err = spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vacuous")
}

func TestValidateNoCases(t *testing.T) {
	body := "baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c\ncases: []\n"
	spec, err := LoadSpec(writeSpec(t, body))
	require.NoError(t, err)
	err = spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no cases")
}

func TestValidateDuplicateID(t *testing.T) {
	body := `baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: dup
    mode: audit
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
`
	spec, err := LoadSpec(writeSpec(t, body))
	require.NoError(t, err)
	err = spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate case id")
}

func TestValidateEmptyPromptSetupExpect(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"empty prompt", caseBody("", "s", "[a]", "[b]"), "empty prompt"},
		{"empty setup", caseBody("p", "", "[a]", "[b]"), "empty setup"},
		{"empty must", caseBody("p", "s", "[]", "[b]"), "expect.must"},
		{"empty must_not", caseBody("p", "s", "[a]", "[]"), "expect.must_not"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := LoadSpec(writeSpec(t, tc.body))
			require.NoError(t, err)
			err = spec.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// caseBody builds a single-case spec body with the given prompt, setup, and
// must / must_not list literals.
func caseBody(prompt, setup, must, mustNot string) string {
	return "baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c\n" +
		"cases:\n  - id: c1\n    mode: audit\n" +
		"    prompt: \"" + prompt + "\"\n    setup: \"" + setup + "\"\n" +
		"    expect: {must: " + must + ", must_not: " + mustNot + "}\n" +
		"    grade: {forbid_severities: [high]}\n"
}

func TestConstraintsForCase(t *testing.T) {
	c := Case{Grade: GradeSpec{ForbidSeverities: []string{"critical"}}}
	con, err := ConstraintsForCase(c)
	require.NoError(t, err)
	assert.Equal(t, []string{"critical"}, con.ForbidSeverities)

	c = Case{Grade: GradeSpec{RequireFinding: &RequireFinding{MinSeverity: "high", LocationFile: "f.go"}}}
	con, err = ConstraintsForCase(c)
	require.NoError(t, err)
	assert.Equal(t, "high", con.RequireMinSeverity)
	assert.Equal(t, "f.go", con.RequireLocationFile)

	// Empty grade -> vacuous error.
	_, err = ConstraintsForCase(Case{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vacuous")
}

func TestValidateEmptyID(t *testing.T) {
	body := `baseline_ref: b72285df2f6b422dd2eb31058884757f13acc78c
cases:
  - id: ""
    mode: audit
    prompt: p
    setup: s
    expect: {must: [a], must_not: [b]}
    grade: {forbid_severities: [high]}
`
	spec, err := LoadSpec(writeSpec(t, body))
	require.NoError(t, err)
	err = spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no id")
}
