package secreview

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- orQuestion ---

func TestOrQuestion(t *testing.T) {
	assert.Equal(t, "hello", orQuestion("hello"))
	assert.Equal(t, "?", orQuestion(""))
}

// --- buildReport ---

func TestBuildReport(t *testing.T) {
	r := &Report{
		Target:   Target{Mode: "audit", Repo: "jeduden/mdsmith", Ref: "abc123", Scope: "cli"},
		Coverage: "All code paths reviewed.",
		Findings: []Finding{
			{ID: "S001", Title: "cmd injection", Severity: "high", Confidence: "confirmed", Surface: "cli"},
			{ID: "S002", Title: "info note", Severity: "info", Confidence: "likely", Surface: "lsp"},
		},
	}
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	got := buildReport(r, now)
	assert.Contains(t, got, "# mdsmith Security Review")
	assert.Contains(t, got, "jeduden/mdsmith")
	assert.Contains(t, got, "abc123")
	assert.Contains(t, got, "2026-06-14")
	assert.Contains(t, got, "cmd injection")
	assert.Contains(t, got, "info note")
	assert.Contains(t, got, "All code paths reviewed.")
	// high-severity finding must appear before info-severity (sortedFindings ordering).
	assert.Less(t, strings.Index(got, "cmd injection"), strings.Index(got, "info note"))
}

func TestBuildReport_Empty(t *testing.T) {
	r := &Report{}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := buildReport(r, now)
	assert.Contains(t, got, "# mdsmith Security Review")
	assert.Contains(t, got, coveragePlaceholder)
}

// --- writeHeader ---

func TestWriteHeader(t *testing.T) {
	var b strings.Builder
	target := &Target{Repo: "jeduden/mdsmith", Ref: "deadbeef", Mode: "pr", Scope: "cli + lsp"}
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	writeHeader(&b, target, now)
	got := b.String()
	assert.Contains(t, got, "# mdsmith Security Review")
	assert.Contains(t, got, "jeduden/mdsmith")
	assert.Contains(t, got, "`deadbeef`")
	assert.Contains(t, got, "pr")
	assert.Contains(t, got, "cli + lsp")
	assert.Contains(t, got, "2026-06-14")
}

func TestWriteHeader_EmptyTarget(t *testing.T) {
	var b strings.Builder
	writeHeader(&b, &Target{}, time.Time{})
	got := b.String()
	assert.Contains(t, got, "?")
}

// --- writeSummary ---

func TestWriteSummary(t *testing.T) {
	findings := []Finding{
		{ID: "S001", Title: "critical bug", Severity: "critical", Confidence: "confirmed", Surface: "cli",
			Location: &Location{File: "a.go", StartLine: 10}},
		{ID: "S002", Title: "medium thing", Severity: "medium", Confidence: "likely", Surface: "lsp"},
	}
	var b strings.Builder
	writeSummary(&b, findings)
	got := b.String()
	assert.Contains(t, got, "## Summary")
	assert.Contains(t, got, "Critical: 1 |")
	assert.Contains(t, got, "Medium: 1 |")
	assert.Contains(t, got, "S001")
	assert.Contains(t, got, "critical bug")
	assert.Contains(t, got, "medium thing")
}

func TestWriteSummary_Empty(t *testing.T) {
	var b strings.Builder
	writeSummary(&b, nil)
	got := b.String()
	assert.Contains(t, got, "## Summary")
	assert.Contains(t, got, "Critical: 0 |")
}

// --- tableCell ---

func TestTableCell(t *testing.T) {
	assert.Equal(t, "plain", tableCell("plain"))
}

func TestTableCell_Pipe(t *testing.T) {
	assert.Equal(t, `a\|b`, tableCell("a|b"))
}

func TestTableCell_Backslash(t *testing.T) {
	assert.Equal(t, `a\\b`, tableCell(`a\b`))
}

func TestTableCell_BackslashPipe(t *testing.T) {
	// A literal \| input must not produce a live pipe in the output.
	// The backslash is doubled first, then the pipe is escaped.
	assert.Equal(t, `a\\\|b`, tableCell(`a\|b`))
}

func TestTableCell_Newline(t *testing.T) {
	assert.Equal(t, "a b", tableCell("a\nb"))
	assert.Equal(t, "a b", tableCell("a\rb"))
}

// --- severityCounts ---

func TestSeverityCounts(t *testing.T) {
	findings := []Finding{
		{Severity: "critical"},
		{Severity: "critical"},
		{Severity: "high"},
		{Severity: "info"},
	}
	got := severityCounts(findings)
	assert.Contains(t, got, "Critical: 2 |")
	assert.Contains(t, got, "High: 1 |")
	assert.Contains(t, got, "Medium: 0 |")
	assert.Contains(t, got, "Low: 0 |")
	assert.Contains(t, got, "Info: 1")
}

func TestSeverityCounts_Empty(t *testing.T) {
	got := severityCounts(nil)
	assert.Contains(t, got, "Critical: 0 |")
	assert.Contains(t, got, "Info: 0")
}

// --- writeFindingSections ---

func TestWriteFindingSections(t *testing.T) {
	findings := []Finding{
		{ID: "S001", Title: "exec sink", Severity: "high", Confidence: "confirmed"},
		{ID: "S002", Title: "hardening note", Severity: "info", Confidence: "likely"},
	}
	var b strings.Builder
	writeFindingSections(&b, findings)
	got := b.String()
	assert.Contains(t, got, "## Findings")
	assert.Contains(t, got, "exec sink")
	assert.Contains(t, got, "## Hardening / Informational")
	assert.Contains(t, got, "hardening note")
}

func TestWriteFindingSections_NoReal(t *testing.T) {
	findings := []Finding{
		{ID: "S001", Title: "note", Severity: "info"},
	}
	var b strings.Builder
	writeFindingSections(&b, findings)
	got := b.String()
	assert.NotContains(t, got, "## Findings")
	assert.Contains(t, got, "## Hardening / Informational")
}

func TestWriteFindingSections_Empty(t *testing.T) {
	var b strings.Builder
	writeFindingSections(&b, nil)
	assert.Empty(t, b.String())
}

// --- renderFinding ---

func TestRenderFinding(t *testing.T) {
	f := &Finding{
		ID:          "S001",
		Title:       "exec injection",
		Severity:    "critical",
		Confidence:  "confirmed",
		Surface:     "cli",
		CWE:         "CWE-78",
		Location:    &Location{File: "cmd/main.go", StartLine: 10},
		Description: "shell args passed unsanitized",
		Impact:      "RCE",
		Repro:       "mdsmith fix $(malicious)",
		Remediation: "use argv not sh -c",
	}
	got := renderFinding(f)
	assert.Contains(t, got, "### S001 · exec injection")
	assert.Contains(t, got, "**Severity:** critical")
	assert.Contains(t, got, "**CWE-78**")
	assert.Contains(t, got, "cmd/main.go:10")
	assert.Contains(t, got, "**What.** shell args passed unsanitized")
	assert.Contains(t, got, "**Impact.** RCE")
	assert.Contains(t, got, "**Repro (sketch).** mdsmith fix $(malicious)")
	assert.Contains(t, got, "**Fix.** use argv not sh -c")
}

func TestRenderFinding_NoCWE(t *testing.T) {
	f := &Finding{ID: "S001", Title: "t", Severity: "low", Confidence: "likely"}
	got := renderFinding(f)
	assert.NotContains(t, got, "CWE")
}

func TestRenderFinding_RelatedLocations(t *testing.T) {
	f := &Finding{
		ID:               "S001",
		Title:            "t",
		Severity:         "medium",
		RelatedLocations: []Location{{File: "x.go", StartLine: 5}},
	}
	got := renderFinding(f)
	assert.Contains(t, got, "- related: `x.go:5`")
}

// --- writeFindingProse ---

func TestWriteFindingProse(t *testing.T) {
	var b strings.Builder
	f := &Finding{
		Description: "desc",
		Impact:      "impact",
		Repro:       "repro",
		Remediation: "fix",
	}
	writeFindingProse(&b, f)
	got := b.String()
	assert.Contains(t, got, "**What.** desc")
	assert.Contains(t, got, "**Impact.** impact")
	assert.Contains(t, got, "**Repro (sketch).** repro")
	assert.Contains(t, got, "**Fix.** fix")
}

func TestWriteFindingProse_AllEmpty(t *testing.T) {
	var b strings.Builder
	writeFindingProse(&b, &Finding{})
	assert.Empty(t, b.String())
}

func TestWriteFindingProse_PartialFields(t *testing.T) {
	var b strings.Builder
	writeFindingProse(&b, &Finding{Description: "only desc"})
	got := b.String()
	assert.Contains(t, got, "**What.**")
	assert.NotContains(t, got, "**Impact.**")
	assert.NotContains(t, got, "**Fix.**")
}

// --- writeCoverage ---

func TestWriteCoverage(t *testing.T) {
	var b strings.Builder
	writeCoverage(&b, "Reviewed all LSP paths.")
	got := b.String()
	assert.Contains(t, got, "## Coverage")
	assert.Contains(t, got, "Reviewed all LSP paths.")
}

func TestWriteCoverage_Empty(t *testing.T) {
	var b strings.Builder
	writeCoverage(&b, "")
	got := b.String()
	assert.Contains(t, got, "## Coverage")
	assert.Contains(t, got, coveragePlaceholder)
}

// --- capitalize ---

func TestCapitalize(t *testing.T) {
	assert.Equal(t, "Critical", capitalize("critical"))
	assert.Equal(t, "High", capitalize("high"))
	assert.Equal(t, "Info", capitalize("info"))
	assert.Equal(t, "A", capitalize("a"))
}

func TestCapitalize_Empty(t *testing.T) {
	assert.Equal(t, "", capitalize(""))
}

func TestCapitalize_AlreadyUpper(t *testing.T) {
	assert.Equal(t, "Critical", capitalize("Critical"))
}

// --- buildAnnotations ---

func TestBuildAnnotations(t *testing.T) {
	r := &Report{
		Findings: []Finding{
			{ID: "S001", Title: "exec sink", Severity: "critical", Confidence: "confirmed",
				Location: &Location{File: "cmd/main.go", StartLine: 10}},
			{ID: "S002", Title: "no location", Severity: "high"},
		},
	}
	anns := buildAnnotations(r)
	require.Len(t, anns, 1)
	assert.Equal(t, "cmd/main.go", anns[0].Path)
	assert.Equal(t, 10, anns[0].Line)
	assert.Equal(t, "RIGHT", anns[0].Side)
	assert.Equal(t, "critical", anns[0].Severity)
	assert.Equal(t, "S001", anns[0].ID)
	assert.Contains(t, anns[0].Body, "S001")
}

func TestBuildAnnotations_ZeroStartLine(t *testing.T) {
	r := &Report{
		Findings: []Finding{
			{ID: "S001", Title: "no line", Severity: "high",
				Location: &Location{File: "a.go", StartLine: 0}},
		},
	}
	anns := buildAnnotations(r)
	assert.Empty(t, anns)
}

func TestBuildAnnotations_NilLocation(t *testing.T) {
	r := &Report{
		Findings: []Finding{
			{ID: "S001", Title: "no loc", Severity: "medium"},
		},
	}
	anns := buildAnnotations(r)
	assert.Empty(t, anns)
}

func TestBuildAnnotations_Empty(t *testing.T) {
	assert.Empty(t, buildAnnotations(&Report{}))
}

// --- annotationBody ---

func TestAnnotationBody(t *testing.T) {
	f := &Finding{ID: "S001", Title: "exec sink", Severity: "critical",
		Description: "shell args not sanitized", Remediation: "use argv"}
	got := annotationBody(f)
	assert.Contains(t, got, "**[S001 · critical] exec sink**")
	assert.Contains(t, got, "shell args not sanitized")
	assert.Contains(t, got, "**Fix:** use argv")
}

func TestAnnotationBody_NoDescription(t *testing.T) {
	f := &Finding{ID: "S001", Title: "t", Severity: "low"}
	got := annotationBody(f)
	assert.Equal(t, "**[S001 · low] t**\n\n**Fix:** n/a", got)
}

func TestAnnotationBody_NoRemediation(t *testing.T) {
	f := &Finding{ID: "S001", Title: "t", Severity: "low", Description: "desc"}
	got := annotationBody(f)
	assert.Equal(t, "**[S001 · low] t**\n\ndesc\n\n**Fix:** n/a", got)
}
