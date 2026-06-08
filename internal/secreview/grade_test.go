package secreview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildConstraintsOK(t *testing.T) {
	c, err := BuildConstraints([]string{"Critical", "high"}, "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"critical", "high"}, c.ForbidSeverities)
	assert.Empty(t, c.RequireMinSeverity)

	c, err = BuildConstraints(nil, "High", "internal/rules/build/rule.go")
	require.NoError(t, err)
	assert.Equal(t, "high", c.RequireMinSeverity)
	assert.Equal(t, "internal/rules/build/rule.go", c.RequireLocationFile)
}

func TestBuildConstraintsUnknownSeverity(t *testing.T) {
	_, err := BuildConstraints([]string{"sev"}, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown severity")

	_, err = BuildConstraints(nil, "huge", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown severity")
}

func TestBuildConstraintsLocationNeedsMin(t *testing.T) {
	_, err := BuildConstraints(nil, "", "internal/rules/build/rule.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a min_severity")
}

func TestBuildConstraintsVacuous(t *testing.T) {
	_, err := BuildConstraints(nil, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vacuous")
}

func TestValidateFindingsEmptySeverity(t *testing.T) {
	err := ValidateFindings([]Finding{{ID: "S001"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no severity")
}

func TestValidateFindingsUnknownSeverity(t *testing.T) {
	err := ValidateFindings([]Finding{{ID: "S001", Severity: "spicy"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown severity")
}

func TestValidateFindingsEmptyIDLabel(t *testing.T) {
	err := ValidateFindings([]Finding{{Severity: ""}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "#0")
}

func TestValidateFindingsOK(t *testing.T) {
	require.NoError(t, ValidateFindings([]Finding{
		{ID: "S001", Severity: "Critical"}, {ID: "S002", Severity: "info"},
	}))
}

func TestGradeForbidCatchesCritical(t *testing.T) {
	c, err := BuildConstraints([]string{"critical", "high"}, "", "")
	require.NoError(t, err)
	fs := []Finding{
		{ID: "S001", Severity: "critical", Title: "rce"},
		{ID: "S002", Severity: "info"},
	}
	failures := Grade(fs, c)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "forbidden severity")
	assert.Contains(t, failures[0], "S001")
}

func TestGradeForbidPassesWhenClean(t *testing.T) {
	c, err := BuildConstraints([]string{"critical", "high"}, "", "")
	require.NoError(t, err)
	assert.Empty(t, Grade([]Finding{{ID: "S001", Severity: "info"}}, c))
}

func TestGradeRequirePassesPrimaryMatch(t *testing.T) {
	c, err := BuildConstraints(nil, "high", "internal/rules/build/rule.go")
	require.NoError(t, err)
	fs := []Finding{{
		ID: "S001", Severity: "critical",
		Location: &Location{File: "internal/rules/build/rule.go", StartLine: 270},
	}}
	assert.Empty(t, Grade(fs, c))
}

func TestGradeRequireFailsWhenOnlyRelatedMatches(t *testing.T) {
	// The required file appears ONLY in related_locations; the primary
	// location points elsewhere. require_finding must not be satisfied.
	c, err := BuildConstraints(nil, "high", "internal/rules/build/rule.go")
	require.NoError(t, err)
	fs := []Finding{{
		ID: "S001", Severity: "critical",
		Location:         &Location{File: "some/other/file.go", StartLine: 10},
		RelatedLocations: []Location{{File: "internal/rules/build/rule.go", StartLine: 270}},
	}}
	failures := Grade(fs, c)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "internal/rules/build/rule.go")
}

func TestGradeRequireFailsWhenOnlyInfo(t *testing.T) {
	c, err := BuildConstraints(nil, "high", "")
	require.NoError(t, err)
	fs := []Finding{{ID: "S001", Severity: "info", Location: &Location{File: "x.go", StartLine: 1}}}
	failures := Grade(fs, c)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "no finding of severity >= high")
}

func TestGradeRequirePassesNoFileFloorOnly(t *testing.T) {
	c, err := BuildConstraints(nil, "medium", "")
	require.NoError(t, err)
	assert.Empty(t, Grade([]Finding{{ID: "S001", Severity: "high"}}, c))
	// A finding below the floor fails.
	assert.NotEmpty(t, Grade([]Finding{{ID: "S001", Severity: "low"}}, c))
}

func TestGradeRequireNoPrimaryLocation(t *testing.T) {
	// require_location_file set but the finding has no primary location.
	c, err := BuildConstraints(nil, "high", "f.go")
	require.NoError(t, err)
	assert.NotEmpty(t, Grade([]Finding{{ID: "S001", Severity: "critical"}}, c))
}

func TestLoadReportOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"target": {"mode": "pr", "repo": "r"},
		"findings": [{"id": "S001", "severity": "high", "location": {"file": "a.go", "startLine": 1}}]
	}`), 0o644))
	r, err := LoadReport(path)
	require.NoError(t, err)
	require.Len(t, r.Findings, 1)
	assert.Equal(t, "S001", r.Findings[0].ID)
	assert.Equal(t, "pr", r.Target.Mode)
}

func TestLoadReportBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o644))
	_, err := LoadReport(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestLoadReportMissingFile(t *testing.T) {
	_, err := LoadReport(filepath.Join(t.TempDir(), "nope.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

func TestLoadReportBadSeverity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"findings":[{"id":"S001","severity":"oops"}]}`), 0o644))
	_, err := LoadReport(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown severity")
}

func TestValidateFindingsRequiresID(t *testing.T) {
	// An empty id is rejected: buildSARIF keys rules by id, so empty-id
	// findings would otherwise collapse into one mislabeled rule.
	err := ValidateFindings([]Finding{{Severity: "high"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no id")
}

func TestValidateFindingsNormalizesSeverity(t *testing.T) {
	// Severity is lowercased in place so render and grade see a canonical
	// value (Python's load() did the same).
	fs := []Finding{{ID: "S001", Severity: "Critical"}, {ID: "S002", Severity: "HIGH"}}
	require.NoError(t, ValidateFindings(fs))
	assert.Equal(t, "critical", fs[0].Severity)
	assert.Equal(t, "high", fs[1].Severity)
}
