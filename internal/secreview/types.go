// Package secreview is the engine behind the mdsmith-security-review
// skill. It is a faithful Go port of the skill's former Python helpers:
// it renders a single findings.json into the three review outputs
// (SARIF, a Markdown report, and inline PR annotations), grades a
// findings.json against the machine-checkable subset of a case rubric,
// and loads/validates the eval case specification.
//
// The package is intentionally free of any I/O beyond reading the
// findings/spec files it is handed, so the CLI in cmd/mdsmith-secreview
// and the eval integration test can both drive it.
package secreview

// Severity ranking, lowest to highest. A finding's severity must be one
// of these five values; everything downstream (SARIF level mapping,
// report ordering, grade floors) keys off this scale.
const (
	severityInfo     = "info"
	severityLow      = "low"
	severityMedium   = "medium"
	severityHigh     = "high"
	severityCritical = "critical"
)

// severityRank orders severities low to high so a "min severity" floor
// can be expressed as an integer comparison (info=0 … critical=4).
var severityRank = map[string]int{
	severityInfo:     0,
	severityLow:      1,
	severityMedium:   2,
	severityHigh:     3,
	severityCritical: 4,
}

// severitiesHighToLow is the single source of truth for severity
// display order, most severe first. severityOrder and the report's
// count line both derive from it so they cannot drift apart.
var severitiesHighToLow = []string{
	severityCritical, severityHigh, severityMedium, severityLow, severityInfo,
}

// severityOrder orders severities high to low for report/SARIF output
// (critical=0 … info=4), derived from severitiesHighToLow.
var severityOrder = indexOf(severitiesHighToLow)

// knownSeverityList is the five severities least-severe-first, used in
// the "want one of …" validity error.
var knownSeverityList = []string{
	severityInfo, severityLow, severityMedium, severityHigh, severityCritical,
}

// sarifLevel maps a finding severity to the SARIF result level, matching
// render_findings.py: critical/high are errors, medium is a warning, and
// low/info are notes.
var sarifLevel = map[string]string{
	severityCritical: "error",
	severityHigh:     "error",
	severityMedium:   "warning",
	severityLow:      "note",
	severityInfo:     "note",
}

// securitySeverity maps a finding severity to the GitHub code-scanning
// security-severity score string, matching render_findings.py.
var securitySeverity = map[string]string{
	severityCritical: "9.5",
	severityHigh:     "8.0",
	severityMedium:   "5.5",
	severityLow:      "3.0",
	severityInfo:     "0.0",
}

// confidenceOrder orders confidence labels for report/annotation sorting:
// confirmed before likely before tentative, with any other value last.
var confidenceOrder = map[string]int{
	"confirmed": 0,
	"likely":    1,
	"tentative": 2,
}

// SeverityKnown reports whether s (already lowercased) is one of the five
// recognized severities.
func SeverityKnown(s string) bool {
	_, ok := severityRank[s]
	return ok
}

// indexOf maps each element of ss to its position in the slice.
func indexOf(ss []string) map[string]int {
	m := make(map[string]int, len(ss))
	for i, s := range ss {
		m[s] = i
	}
	return m
}

// Location is a single source position cited by a finding.
type Location struct {
	// File is the workspace-relative path of the cited file.
	File string `json:"file"`
	// StartLine is the 1-based first line of the region.
	StartLine int `json:"startLine"`
	// EndLine is the 1-based last line; omitted when it equals StartLine.
	EndLine int `json:"endLine,omitempty"`
}

// Finding is one security defect (or hardening note) in a review.
type Finding struct {
	// ID is the review-stable identifier (S001, S002, ...).
	ID string `json:"id"`
	// Title is the one-line headline.
	Title string `json:"title"`
	// Severity is one of critical|high|medium|low|info.
	Severity string `json:"severity"`
	// Confidence is one of confirmed|likely|tentative.
	Confidence string `json:"confidence"`
	// Surface names the affected mdsmith surface (cli, lsp, vscode, ...).
	Surface string `json:"surface"`
	// CWE is an optional comma-separated CWE identifier string.
	CWE string `json:"cwe"`
	// Location is the primary source position, if any.
	Location *Location `json:"location"`
	// RelatedLocations are additional positions in a multi-site chain.
	RelatedLocations []Location `json:"related_locations"`
	// Description explains the defect and the code path in plain language.
	Description string `json:"description"`
	// Impact states what an attacker achieves.
	Impact string `json:"impact"`
	// Repro is a minimal sketch proving the defect (not a weaponized PoC).
	Repro string `json:"repro"`
	// Remediation is the concrete fix.
	Remediation string `json:"remediation"`
}

// Target describes what was reviewed.
type Target struct {
	// Mode is "pr" or "audit".
	Mode string `json:"mode"`
	// Repo is the reviewed repository (e.g. jeduden/mdsmith).
	Repo string `json:"repo"`
	// Ref is the commit or PR head, for traceability.
	Ref string `json:"ref"`
	// Scope is free text describing the reviewed surface.
	Scope string `json:"scope"`
}

// Report is the root of a findings.json document.
type Report struct {
	// Target identifies what was reviewed.
	Target Target `json:"target"`
	// Coverage is an optional note on what was and was not reviewed.
	Coverage string `json:"coverage"`
	// Findings is the list of defects and hardening notes.
	Findings []Finding `json:"findings"`
}
