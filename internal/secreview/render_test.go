package secreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiSevReport returns a report with one critical, one info, and one
// medium finding so SARIF level / security-severity mapping is exercised
// across the scale.
func multiSevReport() *Report {
	return &Report{
		Target:   Target{Mode: "audit", Repo: "jeduden/mdsmith", Ref: "abc1234", Scope: "all"},
		Coverage: "Reviewed everything.",
		Findings: []Finding{
			{
				ID: "S001", Title: "critical sink", Severity: "critical",
				Confidence: "confirmed", Surface: "directive", CWE: "CWE-78",
				Location:    &Location{File: "internal/rules/build/rule.go", StartLine: 270, EndLine: 273},
				Description: "an exec sink", Remediation: "revert",
			},
			{
				ID: "S002", Title: "info note", Severity: "info",
				Confidence: "confirmed", Surface: "directive",
				Location: &Location{File: "internal/x.go", StartLine: 1}, Description: "fyi",
			},
			{
				ID: "S003", Title: "medium thing", Severity: "medium",
				Confidence: "likely", Location: &Location{File: "y.go", StartLine: 9},
			},
		},
	}
}

// decodeSARIF renders r to a temp dir, asserts the three files exist, and
// returns the parsed findings.sarif as a generic map.
func decodeSARIF(t *testing.T, r *Report) map[string]any {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, Render(r, dir))
	for _, name := range []string{"findings.sarif", "report.md", "inline-annotations.json"} {
		_, err := os.Stat(filepath.Join(dir, name))
		require.NoErrorf(t, err, "expected %s to exist", name)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "findings.sarif"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

// firstResult returns runs[0].results[0] of a parsed SARIF document.
func firstResult(doc map[string]any) map[string]any {
	run := doc["runs"].([]any)[0].(map[string]any)
	return run["results"].([]any)[0].(map[string]any)
}

func TestRenderSARIFTopLevel(t *testing.T) {
	doc := decodeSARIF(t, multiSevReport())
	assert.Equal(t, "https://json.schemastore.org/sarif-2.1.0.json", doc["$schema"])
	assert.Equal(t, "2.1.0", doc["version"])
	runs, ok := doc["runs"].([]any)
	require.True(t, ok)
	require.Len(t, runs, 1)
	driver := runs[0].(map[string]any)["tool"].(map[string]any)["driver"].(map[string]any)
	assert.Equal(t, "mdsmith-security-review", driver["name"])
	assert.Equal(t, "https://github.com/jeduden/mdsmith", driver["informationUri"])
}

func TestRenderSARIFSeverityMapping(t *testing.T) {
	doc := decodeSARIF(t, multiSevReport())
	run := doc["runs"].([]any)[0].(map[string]any)
	results := run["results"].([]any)
	require.Len(t, results, 3)
	rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
	require.Len(t, rules, 3)

	wantLevel := map[string]string{"S001": "error", "S002": "note", "S003": "warning"}
	wantSecSev := map[string]string{"S001": "9.5", "S002": "0.0", "S003": "5.5"}
	for _, r := range results {
		res := r.(map[string]any)
		id := res["ruleId"].(string)
		assert.Equalf(t, wantLevel[id], res["level"], "result level for %s", id)
	}
	for _, ru := range rules {
		rule := ru.(map[string]any)
		id := rule["id"].(string)
		props := rule["properties"].(map[string]any)
		assert.Equalf(t, wantSecSev[id], props["security-severity"], "security-severity for %s", id)
	}
}

func TestRenderSARIFRuleProperties(t *testing.T) {
	doc := decodeSARIF(t, multiSevReport())
	run := doc["runs"].([]any)[0].(map[string]any)
	rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
	byID := map[string]map[string]any{}
	for _, ru := range rules {
		rule := ru.(map[string]any)
		byID[rule["id"].(string)] = rule
	}
	// CWE-bearing finding: tags = ["security", cwe]; surface -> rule name.
	s1 := byID["S001"]
	assert.Equal(t, "directive", s1["name"])
	assert.Equal(t, "critical sink", s1["shortDescription"].(map[string]any)["text"])
	tags := s1["properties"].(map[string]any)["tags"].([]any)
	assert.Equal(t, []any{"security", "CWE-78"}, tags)
	// No-surface, no-CWE finding: name falls back to "security", tags just security.
	s3 := byID["S003"]
	assert.Equal(t, "security", s3["name"])
	assert.Equal(t, []any{"security"}, s3["properties"].(map[string]any)["tags"].([]any))
}

func TestRenderSARIFRegionAndFallback(t *testing.T) {
	// A finding with no usable location falls back to a single location
	// naming the target repo.
	r := &Report{
		Target:   Target{Repo: "jeduden/mdsmith"},
		Findings: []Finding{{ID: "S001", Title: "t", Severity: "high"}},
	}
	locs := firstResult(decodeSARIF(t, r))["locations"].([]any)
	require.Len(t, locs, 1)
	phys0 := locs[0].(map[string]any)["physicalLocation"].(map[string]any)
	art := phys0["artifactLocation"].(map[string]any)
	assert.Equal(t, "jeduden/mdsmith", art["uri"])

	// A finding with a region: endLine present and primary + related kept.
	region := multiSevReport().Findings[0]
	locs2 := firstResult(decodeSARIF(t, &Report{Findings: []Finding{region}}))["locations"].([]any)
	phys := locs2[0].(map[string]any)["physicalLocation"].(map[string]any)
	reg := phys["region"].(map[string]any)
	assert.Equal(t, float64(270), reg["startLine"])
	assert.Equal(t, float64(273), reg["endLine"])
}

func TestRenderResultPropertiesConfidence(t *testing.T) {
	r := &Report{Findings: []Finding{{ID: "S001", Severity: "low"}}}
	props := firstResult(decodeSARIF(t, r))["properties"].(map[string]any)
	assert.Equal(t, "unspecified", props["confidence"])
	assert.Equal(t, "low", props["severity"])
}

func TestRenderReportCoveragePlaceholder(t *testing.T) {
	r := multiSevReport()
	r.Coverage = ""
	md := buildReport(r, time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC))
	assert.Contains(t, md, coveragePlaceholder)

	r.Coverage = "Looked at the parser only."
	md = buildReport(r, time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC))
	assert.Contains(t, md, "Looked at the parser only.")
	assert.NotContains(t, md, coveragePlaceholder)
}

func TestRenderReportSections(t *testing.T) {
	md := buildReport(multiSevReport(), time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC))
	assert.Contains(t, md, "# mdsmith Security Review")
	assert.Contains(t, md, "- **Target:** jeduden/mdsmith @ `abc1234`")
	assert.Contains(t, md, "- **Date:** 2026-06-08")
	assert.Contains(t, md, "## Summary")
	assert.Contains(t, md, "Critical: 1 | High: 0 | Medium: 1 | Low: 0 | Info: 1")
	assert.Contains(t, md, "## Findings")
	assert.Contains(t, md, "## Hardening / Informational")
	// Critical sorts before medium in the findings section.
	assert.Less(t, strings.Index(md, "### S001"), strings.Index(md, "### S003"))
	// The info finding is under Hardening, after the real findings.
	assert.Less(t, strings.Index(md, "## Findings"), strings.Index(md, "### S002"))
}

func TestRenderReportNoRealFindings(t *testing.T) {
	// Only an info finding: no "## Findings" header, only Hardening.
	r := &Report{Findings: []Finding{{ID: "S001", Title: "note", Severity: "info"}}}
	md := buildReport(r, time.Now())
	assert.NotContains(t, md, "## Findings\n")
	assert.Contains(t, md, "## Hardening / Informational")
}

func TestRenderAnnotations(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Render(multiSevReport(), dir))
	raw, err := os.ReadFile(filepath.Join(dir, "inline-annotations.json"))
	require.NoError(t, err)
	var anns []Annotation
	require.NoError(t, json.Unmarshal(raw, &anns))
	require.Len(t, anns, 3)
	// Ordered by severity asc-rank: critical first.
	assert.Equal(t, "S001", anns[0].ID)
	assert.Equal(t, "internal/rules/build/rule.go", anns[0].Path)
	assert.Equal(t, 270, anns[0].Line)
	assert.Equal(t, "RIGHT", anns[0].Side)
	assert.Equal(t, "critical", anns[0].Severity)
	assert.Contains(t, anns[0].Body, "**[S001 · critical] critical sink**")
	assert.Contains(t, anns[0].Body, "**Fix:** revert")
}

func TestRenderAnnotationsSkipNoLocation(t *testing.T) {
	r := &Report{Findings: []Finding{
		{ID: "S001", Severity: "high", Title: "no loc"},                   // no location -> skipped
		{ID: "S002", Severity: "high", Location: &Location{File: "f.go"}}, // no startLine -> skipped
		{ID: "S003", Severity: "high", Location: &Location{StartLine: 5}}, // no file -> skipped
		{ID: "S004", Severity: "high", Location: &Location{File: "g.go", StartLine: 7}, Remediation: ""},
	}}
	anns := buildAnnotations(r)
	require.Len(t, anns, 1)
	assert.Equal(t, "S004", anns[0].ID)
	assert.Contains(t, anns[0].Body, "**Fix:** n/a") // empty remediation -> n/a
}

func TestRenderFileNames(t *testing.T) {
	got := RenderFileNames()
	assert.Equal(t, []string{"findings.sarif", "report.md", "inline-annotations.json"}, got)
	// Returned slice is a copy: mutating it must not affect the package var.
	got[0] = "mutated"
	assert.Equal(t, "findings.sarif", RenderFileNames()[0])
}

func TestRenderWritesFixedNamesIntoAuditDir(t *testing.T) {
	// Render writes the three fixed-name artifacts into the per-audit
	// directory the caller supplies; the directory namespaces the
	// review, so the basenames never vary.
	dir := filepath.Join(t.TempDir(), "2026-06-09-full-repo-audit")
	require.NoError(t, Render(multiSevReport(), dir))
	for _, name := range []string{"findings.sarif", "report.md", "inline-annotations.json"} {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.NoErrorf(t, err, "expected %s to exist", name)
	}
}

func TestRenderErrorOnUnwritableDir(t *testing.T) {
	// Point out-dir at a path whose parent is a regular file, so MkdirAll
	// fails and Render surfaces a wrapped error rather than panicking.
	file := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	err := Render(multiSevReport(), filepath.Join(file, "sub"))
	require.Error(t, err)
}

func TestBuildSARIFDuplicateRuleID(t *testing.T) {
	// Two findings share id S001: one rule, two results, both pointing at
	// rule index 0.
	r := &Report{Findings: []Finding{
		{ID: "S001", Title: "first", Severity: "high"},
		{ID: "S001", Title: "second", Severity: "low"},
	}}
	doc := buildSARIF(r)
	require.Len(t, doc.Runs[0].Tool.Driver.Rules, 1)
	require.Len(t, doc.Runs[0].Results, 2)
	assert.Equal(t, "first", doc.Runs[0].Tool.Driver.Rules[0].ShortDescription.Text)
	assert.Equal(t, 0, doc.Runs[0].Results[0].RuleIndex)
	assert.Equal(t, 0, doc.Runs[0].Results[1].RuleIndex)
}

func TestPhysicalLocationsKeepsRelated(t *testing.T) {
	// Primary + three related: one with a startLine, one file-only (kept,
	// no region — matching render_findings.py), and one with no file at all
	// (dropped). Only a missing file drops a location.
	f := &Finding{
		ID: "S001", Severity: "high",
		Location: &Location{File: "a.go", StartLine: 1},
		RelatedLocations: []Location{
			{File: "b.go", StartLine: 2},
			{File: "c.go"}, // file-only: kept, no region
			{StartLine: 9}, // no file: dropped
		},
	}
	doc := buildSARIF(&Report{Findings: []Finding{*f}})
	locs := doc.Runs[0].Results[0].Locations
	require.Len(t, locs, 3)
	assert.Equal(t, "a.go", locs[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, "b.go", locs[1].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, "c.go", locs[2].PhysicalLocation.ArtifactLocation.URI)
	assert.Nil(t, locs[2].PhysicalLocation.Region, "file-only location has no region")
}

func TestReportSummaryEscapesPipe(t *testing.T) {
	// A finding title containing a pipe (security findings routinely name
	// shell metacharacters) must be escaped in the Markdown summary table so
	// the row keeps its columns instead of splitting into extras.
	r := &Report{Findings: []Finding{{
		ID: "S001", Severity: "high", Confidence: "confirmed",
		Title: "shell || and | pipe", Surface: "cli",
		Location: &Location{File: "a.go", StartLine: 5},
	}}}
	md := buildReport(r, time.Now())
	assert.Contains(t, md, `| shell \|\| and \| pipe |`)
}

func TestReportSummaryEscapesBackslashThenPipe(t *testing.T) {
	// A literal `\|` in a title must escape the backslash too (`\\\|`), or
	// GFM would treat the pipe as a live delimiter and split the row.
	r := &Report{Findings: []Finding{{
		ID: "S001", Severity: "high", Confidence: "confirmed",
		Title: `a \| b`, Surface: "cli", Location: &Location{File: "x.go", StartLine: 1},
	}}}
	md := buildReport(r, time.Now())
	assert.Contains(t, md, `a \\\| b`)
}

func TestAnnotationBodyEmptyDescription(t *testing.T) {
	// A finding with no description must not leave a blank paragraph between
	// the headline and the fix line.
	r := &Report{Findings: []Finding{{
		ID: "S001", Severity: "high", Title: "no desc",
		Location: &Location{File: "a.go", StartLine: 1},
	}}}
	anns := buildAnnotations(r)
	require.Len(t, anns, 1)
	assert.Equal(t, "**[S001 · high] no desc**\n\n**Fix:** n/a", anns[0].Body)
	assert.NotContains(t, anns[0].Body, "\n\n\n")
}

func TestRenderSARIFFileOnlyLocation(t *testing.T) {
	// A finding whose primary location names a file but no startLine renders
	// as that file's artifactLocation (no region) — NOT the repo-name
	// fallback used only when a finding has no usable location at all.
	r := &Report{
		Target:   Target{Repo: "jeduden/mdsmith"},
		Findings: []Finding{{ID: "S001", Severity: "high", Location: &Location{File: "only.go"}}},
	}
	locs := firstResult(decodeSARIF(t, r))["locations"].([]any)
	require.Len(t, locs, 1)
	phys := locs[0].(map[string]any)["physicalLocation"].(map[string]any)
	assert.Equal(t, "only.go", phys["artifactLocation"].(map[string]any)["uri"])
	_, hasRegion := phys["region"]
	assert.False(t, hasRegion, "file-only location must not emit a region")
}

func TestValidateNormalizesSeverityForRender(t *testing.T) {
	// A mixed-case severity passes validation and, after normalization,
	// renders with the correct SARIF level/security-severity instead of a
	// blank one (the bug when render read the raw, un-lowercased value).
	fs := []Finding{{
		ID: "S001", Title: "t", Severity: "Critical",
		Location: &Location{File: "a.go", StartLine: 1},
	}}
	require.NoError(t, ValidateFindings(fs))
	assert.Equal(t, "critical", fs[0].Severity)
	doc := buildSARIF(&Report{Findings: fs})
	assert.Equal(t, "error", doc.Runs[0].Results[0].Level)
	assert.Equal(t, "9.5", doc.Runs[0].Tool.Driver.Rules[0].Properties.SecuritySeverity)
}

func TestSortConfidenceTieAndDefault(t *testing.T) {
	// Same severity, differing confidence: confirmed < likely < tentative
	// < unknown. Exercises confOrder's known and default branches.
	r := []Finding{
		{ID: "D", Severity: "high", Confidence: "weird"},
		{ID: "C", Severity: "high", Confidence: "tentative"},
		{ID: "B", Severity: "high", Confidence: "likely"},
		{ID: "A", Severity: "high", Confidence: "confirmed"},
	}
	got := sortedFindings(r)
	order := []string{got[0].ID, got[1].ID, got[2].ID, got[3].ID}
	assert.Equal(t, []string{"A", "B", "C", "D"}, order)
}

func TestCapitalizeEmpty(t *testing.T) {
	assert.Equal(t, "", capitalize(""))
	assert.Equal(t, "Info", capitalize("info"))
}

func TestLocStr(t *testing.T) {
	assert.Equal(t, "—", locStr(nil))
	assert.Equal(t, "?", locStr(&Location{}))
	assert.Equal(t, "a.go", locStr(&Location{File: "a.go"}))
	assert.Equal(t, "a.go:5", locStr(&Location{File: "a.go", StartLine: 5}))
	assert.Equal(t, "a.go:5-9", locStr(&Location{File: "a.go", StartLine: 5, EndLine: 9}))
	assert.Equal(t, "a.go:5", locStr(&Location{File: "a.go", StartLine: 5, EndLine: 5}))
}

func TestMarshalJSONError(t *testing.T) {
	// A channel cannot be JSON-encoded, so marshalJSON surfaces the encoder
	// error instead of panicking.
	_, err := marshalJSON(make(chan int))
	require.Error(t, err)
}

func TestRenderErrorWhenOutputFileIsADir(t *testing.T) {
	// MkdirAll succeeds, but findings.sarif already exists as a directory,
	// so the WriteFile of the first output fails and Render returns a
	// wrapped write error rather than panicking.
	out := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(out, "findings.sarif"), 0o755))
	err := Render(multiSevReport(), out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "findings.sarif")
}
