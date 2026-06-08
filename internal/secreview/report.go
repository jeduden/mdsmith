package secreview

import (
	"fmt"
	"strings"
	"time"
)

// coveragePlaceholder is emitted in the report's Coverage section when the
// report carries no coverage note, matching render_findings.py.
const coveragePlaceholder = "_Document what was and was not reviewed here._"

// orQuestion returns s, or "?" when s is empty — the fallback
// render_findings.py uses for missing target/finding fields.
func orQuestion(s string) string {
	return orDefault(s, "?")
}

// buildReport renders the Markdown security report, mirroring
// render_findings.py's build_report. now supplies the report date so the
// output is deterministic in tests.
func buildReport(r *Report, now time.Time) string {
	findings := sortedFindings(r.Findings)
	var b strings.Builder
	writeHeader(&b, &r.Target, now)
	writeSummary(&b, findings)
	writeFindingSections(&b, findings)
	writeCoverage(&b, r.Coverage)
	return b.String()
}

// writeHeader writes the title and target/mode/scope/date block.
func writeHeader(b *strings.Builder, t *Target, now time.Time) {
	b.WriteString("# mdsmith Security Review\n\n")
	fmt.Fprintf(b, "- **Target:** %s @ `%s`\n", orQuestion(t.Repo), orQuestion(t.Ref))
	fmt.Fprintf(b, "- **Mode:** %s  \n", orQuestion(t.Mode))
	fmt.Fprintf(b, "- **Scope:** %s\n", orQuestion(t.Scope))
	fmt.Fprintf(b, "- **Date:** %s\n\n", now.Format("2006-01-02"))
}

// writeSummary writes the per-severity counts line and the one-row-per
// finding summary table.
func writeSummary(b *strings.Builder, findings []Finding) {
	b.WriteString("## Summary\n\n")
	b.WriteString(severityCounts(findings) + "\n\n")
	b.WriteString("| ID | Sev | Conf | Title | Surface | Location |\n")
	b.WriteString("|----|-----|------|-------|---------|----------|\n")
	for i := range findings {
		f := &findings[i]
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | `%s` |\n",
			f.ID, f.Severity, orQuestion(f.Confidence), f.Title, f.Surface, locStr(f.Location))
	}
	b.WriteString("\n")
}

// severityCounts renders "Critical: n | High: n | ..." across all five
// severities in descending order, matching render_findings.py.
func severityCounts(findings []Finding) string {
	order := []string{"critical", "high", "medium", "low", "info"}
	counts := make(map[string]int, len(order))
	for i := range findings {
		counts[findings[i].Severity]++
	}
	parts := make([]string, 0, len(order))
	for _, s := range order {
		parts = append(parts, fmt.Sprintf("%s: %d", capitalize(s), counts[s]))
	}
	return strings.Join(parts, " | ")
}

// writeFindingSections writes the real findings under "## Findings" and the
// info-severity ones under "## Hardening / Informational".
func writeFindingSections(b *strings.Builder, findings []Finding) {
	var real, info []Finding
	for i := range findings {
		if findings[i].Severity == "info" {
			info = append(info, findings[i])
		} else {
			real = append(real, findings[i])
		}
	}
	if len(real) > 0 {
		b.WriteString("## Findings\n\n")
		for i := range real {
			b.WriteString(renderFinding(&real[i]))
		}
	}
	if len(info) > 0 {
		b.WriteString("## Hardening / Informational\n\n")
		for i := range info {
			b.WriteString(renderFinding(&info[i]))
		}
	}
}

// renderFinding renders one finding's section: the heading, the
// severity/confidence/surface/CWE line, the location(s), and the optional
// what/impact/repro/fix paragraphs.
func renderFinding(f *Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s · %s\n\n", f.ID, f.Title)
	cwe := ""
	if f.CWE != "" {
		cwe = " · **" + f.CWE + "**"
	}
	fmt.Fprintf(&b, "**Severity:** %s · **Confidence:** %s · **Surface:** %s%s\n\n",
		f.Severity, orQuestion(f.Confidence), orQuestion(f.Surface), cwe)
	fmt.Fprintf(&b, "**Location:** `%s`\n", locStr(f.Location))
	for i := range f.RelatedLocations {
		fmt.Fprintf(&b, "- related: `%s`\n", locStr(&f.RelatedLocations[i]))
	}
	b.WriteString("\n")
	writeFindingProse(&b, f)
	return b.String()
}

// writeFindingProse writes the optional what/impact/repro/fix paragraphs,
// each only when its source field is non-empty.
func writeFindingProse(b *strings.Builder, f *Finding) {
	if f.Description != "" {
		fmt.Fprintf(b, "**What.** %s\n\n", f.Description)
	}
	if f.Impact != "" {
		fmt.Fprintf(b, "**Impact.** %s\n\n", f.Impact)
	}
	if f.Repro != "" {
		fmt.Fprintf(b, "**Repro (sketch).** %s\n\n", f.Repro)
	}
	if f.Remediation != "" {
		fmt.Fprintf(b, "**Fix.** %s\n\n", f.Remediation)
	}
}

// writeCoverage writes the Coverage section, falling back to the
// placeholder when the report carries no note.
func writeCoverage(b *strings.Builder, coverage string) {
	b.WriteString("## Coverage\n\n")
	b.WriteString(orDefault(coverage, coveragePlaceholder))
	b.WriteString("\n")
}

// capitalize uppercases the first byte of s (ASCII severities only), the
// Go equivalent of Python's str.capitalize for these values.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Annotation is one inline PR review comment in inline-annotations.json.
type Annotation struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`
	Severity string `json:"severity"`
	ID       string `json:"id"`
	Body     string `json:"body"`
}

// buildAnnotations builds the flat inline-annotation list, one per finding
// whose primary location has both a file and a startLine, ordered the same
// way as the report. It mirrors render_findings.py's build_annotations.
func buildAnnotations(r *Report) []Annotation {
	findings := sortedFindings(r.Findings)
	anns := make([]Annotation, 0, len(findings))
	for i := range findings {
		f := &findings[i]
		loc := f.Location
		if loc == nil || loc.File == "" || loc.StartLine == 0 {
			continue
		}
		anns = append(anns, Annotation{
			Path:     loc.File,
			Line:     loc.StartLine,
			Side:     "RIGHT",
			Severity: f.Severity,
			ID:       f.ID,
			Body:     annotationBody(f),
		})
	}
	return anns
}

// annotationBody formats one annotation's Markdown body: the headline, the
// description, and the fix (or "n/a").
func annotationBody(f *Finding) string {
	return fmt.Sprintf("**[%s · %s] %s**\n\n%s\n\n**Fix:** %s",
		f.ID, f.Severity, f.Title, f.Description, orDefault(f.Remediation, "n/a"))
}
